package rex

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// dofileLineSuffix names the actions DofileLine advertises in a comment, so a
// reader of Rex's init.lua can find them.
const dofileLineSuffix = " -- clump: actions clump_pick, clump_watch, clump_watch_close"

// ConfigPath is Rex's init file: $XDG_CONFIG_HOME/rex/init.lua, else
// ~/.config/rex/init.lua.
func ConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "rex", "init.lua"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rex", "init.lua"), nil
}

// ModulePath is where `clump rex install` writes clump's Lua module:
// $XDG_DATA_HOME/clump/rex/init.lua, else ~/.local/share/clump/rex/init.lua.
func ModulePath() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "clump", "rex", "init.lua"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "clump", "rex", "init.lua"), nil
}

// DofileLine is the line to add to Rex's init.lua:
//
//	dofile("<modulePath>") -- clump: actions clump_pick, clump_watch, clump_watch_close
//
// modulePath is written as a Lua double-quoted string (\ and " escaped).
func DofileLine(modulePath string) string {
	var b strings.Builder
	b.WriteString(`dofile("`)
	for i := 0; i < len(modulePath); i++ {
		switch c := modulePath[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString(`")`)
	b.WriteString(dofileLineSuffix)
	return b.String()
}

// InstallOpts configures Install.
type InstallOpts struct {
	ConfigPath string // Rex init.lua
	ModulePath string // where the module goes
	Module     []byte // the module's contents (embedded rex/init.lua)
	Write      bool   // append the dofile line after confirmation
	In         io.Reader
	Out        io.Writer
}

// Install always writes Module to ModulePath (clump's own file, mode 0644,
// parent dirs 0755), then prints DofileLine. With Write it also asks
// "Append it to <ConfigPath>? [y/N] " and on "y"/"yes" copies
// ConfigPath to ConfigPath + ".clump-backup-<YYYYMMDD-HHMMSS>" (if it exists)
// and appends the line (with a leading newline if the file does not end in
// one). If ConfigPath already contains the module path it says so and
// changes nothing.
func Install(o InstallOpts) error {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	if len(o.Module) == 0 {
		return errors.New("rex install: empty module")
	}
	if err := installWriteModule(out, o.ModulePath, o.Module); err != nil {
		return err
	}

	content, exists, err := installReadFile(o.ConfigPath)
	if err != nil {
		return err
	}
	if strings.Contains(content, o.ModulePath) {
		_, err := fmt.Fprintf(out, "%s already loads it\n", o.ConfigPath)
		return err
	}

	line := DofileLine(o.ModulePath)
	if _, err := fmt.Fprintf(out, "Add this line to %s:\n\n%s\n\n", o.ConfigPath, line); err != nil {
		return err
	}
	if !o.Write {
		_, err := fmt.Fprintf(out, "(run %q to append it)\n", "clump rex install --write")
		return err
	}

	if _, err := fmt.Fprintf(out, "Append it to %s? [y/N] ", o.ConfigPath); err != nil {
		return err
	}
	in := o.In
	if in == nil {
		in = strings.NewReader("")
	}
	answer, err := installReadLine(in)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if !installIsYes(answer) {
		_, err := fmt.Fprintln(out, "not written")
		return err
	}

	perm := fs.FileMode(0o644)
	backup := ""
	if exists {
		info, err := os.Stat(o.ConfigPath)
		if err != nil {
			return err
		}
		perm = info.Mode().Perm()
		backup = o.ConfigPath + ".clump-backup-" + time.Now().Format("20060102-150405")
		if err := installCopyFile(o.ConfigPath, backup, perm); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(o.ConfigPath), 0o755); err != nil {
		return err
	}
	next := content
	if next != "" && !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	next += line + "\n"
	if err := os.WriteFile(o.ConfigPath, []byte(next), perm); err != nil {
		return err
	}
	if err := os.Chmod(o.ConfigPath, perm); err != nil {
		return err
	}
	if backup != "" {
		_, err = fmt.Fprintf(out, "appended to %s (backup: %s)\n", o.ConfigPath, backup)
	} else {
		_, err = fmt.Fprintf(out, "appended to %s\n", o.ConfigPath)
	}
	return err
}

// installWriteModule writes module to path with mode 0644, creating parent
// directories with 0755, and says what it did. It writes a temporary file in
// the same directory and renames it, so a crash never leaves a half-written
// module. When the file already holds the same bytes it writes nothing and
// leaves its modification time alone.
func installWriteModule(out io.Writer, path string, module []byte) error {
	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, module):
		_, err := fmt.Fprintf(out, "the clump Rex module at %s is up to date\n", path)
		return err
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".clump-rex-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// After a successful rename tmp no longer exists, so this removes the
	// temporary file on every earlier failure path and nothing otherwise.
	defer os.Remove(tmp)
	if _, err := f.Write(module); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "wrote the clump Rex module to %s\n", path)
	return err
}

// installReadFile reads path; a missing file is empty content, not an error.
func installReadFile(path string) (content string, exists bool, err error) {
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		return string(b), true, nil
	case errors.Is(err, fs.ErrNotExist):
		return "", false, nil
	default:
		return "", false, err
	}
}

// installReadLine reads one line from r without its newline.
func installReadLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, err
}

// installIsYes reports whether answer is an explicit yes.
func installIsYes(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

// installCopyFile copies src to dst with the given permissions.
func installCopyFile(src, dst string, perm fs.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, b, perm); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}
