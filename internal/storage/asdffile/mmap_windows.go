//go:build windows

package asdffile

import "os"

// Windows uses an owned byte buffer instead of converting the uintptr returned
// by MapViewOfFile into a Go slice. That conversion is easy to misuse and is
// rejected by go vet's unsafeptr check. The reader already has a ReadAt
// fallback, so a copy is a safe and behaviourally equivalent implementation on
// the development platform. Linux keeps the zero-copy mmap implementation in
// mmap_unix.go.
type mappedFile struct {
	data []byte
}

func mapFile(file *os.File, size int) (*mappedFile, error) {
	if size <= 0 {
		return &mappedFile{}, nil
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		return nil, err
	}
	if len(data) != size {
		return nil, &os.PathError{Op: "read", Path: file.Name(), Err: os.ErrInvalid}
	}
	return &mappedFile{data: data}, nil
}

func (m *mappedFile) Bytes() []byte { return m.data }

// Active reports whether a mapped view is active. The Windows implementation
// deliberately returns false because it owns a regular buffer.
func (m *mappedFile) Active() bool { return false }

func (m *mappedFile) unmap() error {
	m.data = nil
	return nil
}
