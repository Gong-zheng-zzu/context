package asdffile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestArchiveStoreRoundTrip 归档—读回往返。
func TestArchiveStoreRoundTrip(t *testing.T) {
	store, err := NewArchiveStore(filepath.Join(t.TempDir(), "archive"))
	if err != nil {
		t.Fatalf("NewArchiveStore: %v", err)
	}

	payload := []byte("这是需要归档的冷数据内容")
	if err := store.Archive("mem-001", payload); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	got, err := store.Restore("mem-001")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("Restore = %q, want %q", got, payload)
	}

	if err := store.Verify("mem-001"); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

// TestArchiveStoreOverwrites 重新归档同一 id 应整体覆盖，而不是追加。
func TestArchiveStoreOverwrites(t *testing.T) {
	store, err := NewArchiveStore(filepath.Join(t.TempDir(), "archive"))
	if err != nil {
		t.Fatalf("NewArchiveStore: %v", err)
	}

	if err := store.Archive("mem-002", []byte("第一版")); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if err := store.Archive("mem-002", []byte("第二版")); err != nil {
		t.Fatalf("Archive overwrite: %v", err)
	}

	got, err := store.Restore("mem-002")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if string(got) != "第二版" {
		t.Errorf("Restore = %q, want 第二版", got)
	}
}

// TestArchiveStoreDelete 删除后不可读；删除不存在的条目应幂等成功。
func TestArchiveStoreDelete(t *testing.T) {
	store, err := NewArchiveStore(filepath.Join(t.TempDir(), "archive"))
	if err != nil {
		t.Fatalf("NewArchiveStore: %v", err)
	}

	if err := store.Archive("mem-003", []byte("x")); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if err := store.Delete("mem-003"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Restore("mem-003"); err == nil {
		t.Error("Restore succeeded after Delete")
	}
	// 幂等。
	if err := store.Delete("mem-003"); err != nil {
		t.Errorf("Delete of missing entry: %v", err)
	}
	if err := store.Delete("never-existed"); err != nil {
		t.Errorf("Delete of never-existing entry: %v", err)
	}
}

// TestArchiveStoreListAndStat 列表与元信息查询。
func TestArchiveStoreListAndStat(t *testing.T) {
	store, err := NewArchiveStore(filepath.Join(t.TempDir(), "archive"))
	if err != nil {
		t.Fatalf("NewArchiveStore: %v", err)
	}

	for _, id := range []string{"a", "b", "c"} {
		if err := store.Archive(id, []byte("payload-"+id)); err != nil {
			t.Fatalf("Archive %s: %v", id, err)
		}
	}

	ids, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("List = %v, want 3 entries", ids)
	}

	size, modTime, err := store.Stat("a")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if size <= 0 {
		t.Errorf("Stat size = %d, want > 0", size)
	}
	if time.Since(modTime) > time.Hour {
		t.Errorf("modTime = %v, want recent", modTime)
	}
}

// TestArchiveStoreRejectsPathTraversal 路径穿越必须被拒绝。
//
// 归档 id 可能来自业务层，若不加约束，"../secret" 之类的 id
// 会让归档操作读写到目录之外的文件——这是真实的越权风险。
func TestArchiveStoreRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	store, err := NewArchiveStore(filepath.Join(dir, "archive"))
	if err != nil {
		t.Fatalf("NewArchiveStore: %v", err)
	}

	// 在归档目录之外放一个敏感文件。
	outside := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(outside, []byte("敏感"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	for _, badID := range []string{
		"../secret",
		"..",
		"a/b",
		`a\b`,
		"",
		"sp ace",
	} {
		if err := store.Archive(badID, []byte("x")); err == nil {
			t.Errorf("Archive accepted dangerous id %q", badID)
		}
		if _, err := store.Restore(badID); err == nil {
			t.Errorf("Restore accepted dangerous id %q", badID)
		}
		if err := store.Delete(badID); err == nil {
			t.Errorf("Delete accepted dangerous id %q", badID)
		}
	}

	// 敏感文件必须原封不动。
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "敏感" {
		t.Errorf("outside file modified: %q", data)
	}
}

// TestArchiveStoreSurvivesReopen 归档目录重新打开后数据仍在（持久化验证）。
func TestArchiveStoreSurvivesReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "archive")

	store, err := NewArchiveStore(dir)
	if err != nil {
		t.Fatalf("NewArchiveStore: %v", err)
	}
	if err := store.Archive("persist", []byte("跨进程数据")); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// 模拟进程重启：新建一个指向同一目录的 store。
	reopened, err := NewArchiveStore(dir)
	if err != nil {
		t.Fatalf("NewArchiveStore (reopen): %v", err)
	}
	got, err := reopened.Restore("persist")
	if err != nil {
		t.Fatalf("Restore after reopen: %v", err)
	}
	if string(got) != "跨进程数据" {
		t.Errorf("Restore = %q, want 跨进程数据", got)
	}
}

// TestArchiveStoreImplementsArchiver 编译期断言：满足 tiered.Archiver 接口。
//
// 这里用匿名接口而非导入 tiered 包，避免存储层反向依赖记忆层
// （依赖方向必须是 tiered → asdffile）。
type archiverInterface interface {
	Archive(id string, payload []byte) error
	Restore(id string) ([]byte, error)
	Delete(id string) error
}

var _ archiverInterface = (*ArchiveStore)(nil)
