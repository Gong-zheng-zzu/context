package asdffile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ArchiveStore 冷数据归档存储：每个条目一个独立的 ASDF 文件。
//
// 实现 `internal/memory/tiered.Archiver` 接口，可直接接入冷热生命周期
// 管理器——冷数据降级时落盘为 ASDF，需要时再读回。
//
// 为什么一个条目一个文件，而不是全部塞进同一个大文件：
//   - Delete 是 O(1) 的文件删除，不必重写整个归档；
//   - 单条目损坏只影响它自己，不会波及整库；
//   - 可对每个条目单独做完整性校验。
//
// 代价：条目很多时会产生较多小文件。这对冷数据（低频访问）是可接受的取舍。
type ArchiveStore struct {
	dir string
	mu  sync.RWMutex
}

// NewArchiveStore 创建（必要时新建）归档目录。
func NewArchiveStore(dir string) (*ArchiveStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("asdffile: empty archive directory")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("asdffile: create archive dir %q: %w", dir, err)
	}
	return &ArchiveStore{dir: dir}, nil
}

// Archive 把一个条目写入归档。
//
// 同名条目会被整体覆盖：归档内容是冷数据的最终形态，覆盖语义比追加更
// 符合"重新归档"的直觉，也避免同一 id 出现多份互相矛盾的副本。
func (s *ArchiveStore) Archive(id string, payload []byte) error {
	if err := validateID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.pathFor(id)
	// 先写临时文件再重命名：避免进程中途崩溃留下半截归档文件，
	// 使 Restore 读到不完整的 payload。
	temp := path + ".tmp"

	header := NewHeader("")
	header.DataKind = "cold-archive"
	header.Permission = "archive"
	header.TimeStart = time.Now().UnixNano()
	header.GraphNodes = []GraphNodeAttr{{ID: id, Kind: "archive-entry"}}

	writer, err := Create(temp, header)
	if err != nil {
		return err
	}
	if err := writer.PutRaw("payload", payload); err != nil {
		writer.Close()
		os.Remove(temp)
		return err
	}
	if err := writer.Close(); err != nil {
		os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return fmt.Errorf("asdffile: rename archive %q: %w", id, err)
	}
	return nil
}

// Restore 读回一个归档条目。
func (s *ArchiveStore) Restore(id string) ([]byte, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}

	s.mu.RLock()
	path := s.pathFor(id)
	s.mu.RUnlock()

	reader, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.ReadBlock("payload")
}

// Delete 删除一个归档条目。条目不存在时视为成功（幂等）。
func (s *ArchiveStore) Delete(id string) error {
	if err := validateID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.pathFor(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("asdffile: delete archive %q: %w", id, err)
	}
	return nil
}

// Verify 校验单个归档条目的完整性。
func (s *ArchiveStore) Verify(id string) error {
	if err := validateID(id); err != nil {
		return err
	}

	s.mu.RLock()
	path := s.pathFor(id)
	s.mu.RUnlock()

	reader, err := Open(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	return reader.Verify()
}

// Stat 返回归档条目的大小与修改时间。
func (s *ArchiveStore) Stat(id string) (size int64, modTime time.Time, err error) {
	if err := validateID(id); err != nil {
		return 0, time.Time{}, err
	}

	s.mu.RLock()
	path := s.pathFor(id)
	s.mu.RUnlock()

	info, err := os.Stat(path)
	if err != nil {
		return 0, time.Time{}, err
	}
	return info.Size(), info.ModTime(), nil
}

// List 列出全部归档条目 id。
func (s *ArchiveStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("asdffile: list archives: %w", err)
	}
	const suffix = ".asdf"
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, suffix) {
			ids = append(ids, strings.TrimSuffix(name, suffix))
		}
	}
	return ids, nil
}

// pathFor 把条目 id 映射为文件路径。
func (s *ArchiveStore) pathFor(id string) string {
	return filepath.Join(s.dir, id+".asdf")
}

// validateID 校验条目 id，防止路径穿越。
//
// 归档 id 来自上层业务（可能是记忆条目 ID、用户可控字符串），
// 若直接拼进路径，"../" 之类的 id 就能读写目录之外的文件。
func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("asdffile: empty archive id")
	}
	if strings.ContainsAny(id, `/\`) || id == "." || id == ".." || strings.Contains(id, "..") {
		return fmt.Errorf("asdffile: invalid archive id %q: must not contain path separators or '..'", id)
	}
	for _, r := range id {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.'
		if !isAllowed {
			return fmt.Errorf("asdffile: invalid archive id %q: character %q not allowed", id, r)
		}
	}
	return nil
}
