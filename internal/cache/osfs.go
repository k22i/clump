package cache

import (
	"io/fs"
	"os"
	"path/filepath"
)

type osFS struct{}

func (osFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

func (osFS) WriteFileAtomic(name string, data []byte) error {
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	remove := func(cause error) error {
		_ = f.Close()
		_ = os.Remove(tmp)
		return cause
	}
	if _, err := f.Write(data); err != nil {
		return remove(err)
	}
	if err := f.Sync(); err != nil {
		return remove(err)
	}
	if err := f.Close(); err != nil {
		return remove(err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return remove(err)
	}
	if err := os.Rename(tmp, name); err != nil {
		return remove(err)
	}
	return nil
}

func (osFS) Remove(name string) error { return os.Remove(name) }

func (osFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
