package cache

import (
	"io/fs"
	"os"
	"path/filepath"
)

// osFS is the FS backed by the os package.
type osFS struct{}

// ReadFile reads the named file.
func (osFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

// WriteFileAtomic writes data to a temporary file next to name and renames it
// into place, so a reader never sees a partial file.
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
	// remove discards the temporary file on any failure below.
	remove := func(cause error) error {
		// f is closed on the paths that reach here except the close failure
		// itself, where a second Close merely reports the same closed file.
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

// Remove deletes the named file.
func (osFS) Remove(name string) error { return os.Remove(name) }

// Stat returns the FileInfo for the named file.
func (osFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
