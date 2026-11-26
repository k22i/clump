package cache

import (
	"errors"
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
	unlink := func(cause error) error {
		if err := os.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return errors.Join(cause, err)
		}
		return cause
	}
	abandon := func(cause error) error {
		return unlink(errors.Join(cause, f.Close()))
	}
	if _, err := f.Write(data); err != nil {
		return abandon(err)
	}
	if err := f.Sync(); err != nil {
		return abandon(err)
	}
	if err := f.Close(); err != nil {
		return unlink(err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return unlink(err)
	}
	if err := os.Rename(tmp, name); err != nil {
		return unlink(err)
	}
	return nil
}

func (osFS) Remove(name string) error { return os.Remove(name) }

func (osFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
