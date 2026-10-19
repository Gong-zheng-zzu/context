//go:build !windows

package asdffile

import (
	"os"
	"syscall"
)

// mappedFile 在 Unix 系平台上通过 syscall.Mmap 实现。
type mappedFile struct {
	data []byte
}

// mapFile 把文件以只读、共享方式映射到进程地址空间。
//
// size 为 0（空文件）时不建立映射：mmap 长度为 0 是非法参数。
func mapFile(file *os.File, size int) (*mappedFile, error) {
	if size <= 0 {
		return &mappedFile{}, nil
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return &mappedFile{data: data}, nil
}

// Bytes 返回映射区域的切片（零拷贝）。
func (m *mappedFile) Bytes() []byte { return m.data }

// Active 报告映射是否真实建立。
func (m *mappedFile) Active() bool { return len(m.data) > 0 }

// unmap 解除映射。
func (m *mappedFile) unmap() error {
	if len(m.data) == 0 {
		return nil
	}
	err := syscall.Munmap(m.data)
	m.data = nil
	return err
}
