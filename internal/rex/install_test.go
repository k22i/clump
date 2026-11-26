package rex

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var installTestModule = []byte("-- clump for Rex (test module)\nreturn {}\n")

type installPaths struct {
	cfg string
	mod string
}

func newInstallPaths(t *testing.T) installPaths {
	t.Helper()
	dir := t.TempDir()
	return installPaths{
		cfg: filepath.Join(dir, "config", "rex", "init.lua"),
		mod: filepath.Join(dir, "data", "clump", "rex", "init.lua"),
	}
}

func (p installPaths) opts() InstallOpts {
	return InstallOpts{ConfigPath: p.cfg, ModulePath: p.mod, Module: installTestModule}
}

func runInstall(t *testing.T, o InstallOpts) (string, error) {
	t.Helper()
	var out bytes.Buffer
	if o.In == nil {
		o.In = strings.NewReader("")
	}
	o.Out = &out
	err := Install(o)
	return out.String(), err
}

func installMustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func installMustPerm(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func installMustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat %s: err = %v; want a not-exist error", path, err)
	}
}

func installBackups(t *testing.T, configPath string) []string {
	t.Helper()
	b, err := filepath.Glob(configPath + ".clump-backup-*")
	if err != nil {
		t.Fatalf("glob installBackups of %s: %v", configPath, err)
	}
	return b
}

func TestConfigPath(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{"xdg absolute", xdg, filepath.Join(xdg, "rex", "init.lua")},
		{"xdg relative", filepath.Join("relative", "config"), filepath.Join(home, ".config", "rex", "init.lua")},
		{"xdg unset", "", filepath.Join(home, ".config", "rex", "init.lua")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", tt.xdg)
			got, err := ConfigPath()
			if err != nil {
				t.Fatalf("ConfigPath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ConfigPath() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestModulePath(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{"xdg absolute", xdg, filepath.Join(xdg, "clump", "rex", "init.lua")},
		{"xdg relative", filepath.Join("relative", "data"), filepath.Join(home, ".local", "share", "clump", "rex", "init.lua")},
		{"xdg unset", "", filepath.Join(home, ".local", "share", "clump", "rex", "init.lua")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_DATA_HOME", tt.xdg)
			got, err := ModulePath()
			if err != nil {
				t.Fatalf("ModulePath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ModulePath() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestDofileLine(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "plain",
			path: "/home/u/.local/share/clump/rex/init.lua",
			want: `dofile("/home/u/.local/share/clump/rex/init.lua") -- clump: actions clump_pick, clump_watch, clump_watch_close`,
		},
		{
			name: "quote and backslash",
			path: `/tmp/od"d/c:\rex/init.lua`,
			want: `dofile("/tmp/od\"d/c:\\rex/init.lua") -- clump: actions clump_pick, clump_watch, clump_watch_close`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DofileLine(tt.path); got != tt.want {
				t.Fatalf("DofileLine(%q) = %q; want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestInstallPrintOnly(t *testing.T) {
	p := newInstallPaths(t)
	out, err := runInstall(t, p.opts())
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got := installMustRead(t, p.mod); !bytes.Equal(got, installTestModule) {
		t.Fatalf("module = %q; want %q", got, installTestModule)
	}
	if got := installMustPerm(t, p.mod); got != 0o644 {
		t.Fatalf("module mode = %o; want 0644", got)
	}
	if want := "wrote the clump Rex module to " + p.mod; !strings.Contains(out, want) {
		t.Fatalf("output = %q; want it to contain %q", out, want)
	}
	if want := DofileLine(p.mod); !strings.Contains(out, want) {
		t.Fatalf("output = %q; want it to contain %q", out, want)
	}
	if want := `(run "clump rex install --write" to append it)`; !strings.Contains(out, want) {
		t.Fatalf("output = %q; want it to contain %q", out, want)
	}
	if strings.Contains(out, "Append it to") {
		t.Fatalf("output = %q; want no prompt without Write", out)
	}
	installMustNotExist(t, p.cfg)
}

func TestInstallModuleUpToDate(t *testing.T) {
	p := newInstallPaths(t)
	if _, err := runInstall(t, p.opts()); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	old := time.Unix(1700000000, 0)
	if err := os.Chtimes(p.mod, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	out, err := runInstall(t, p.opts())
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if want := "the clump Rex module at " + p.mod + " is up to date"; !strings.Contains(out, want) {
		t.Fatalf("output = %q; want it to contain %q", out, want)
	}
	info, err := os.Stat(p.mod)
	if err != nil {
		t.Fatalf("stat module: %v", err)
	}
	if got := info.ModTime(); !got.Equal(old) {
		t.Fatalf("module mtime = %v; want it unchanged at %v", got, old)
	}
	if got := installMustRead(t, p.mod); !bytes.Equal(got, installTestModule) {
		t.Fatalf("module = %q; want %q", got, installTestModule)
	}
}

func TestInstallWriteYes(t *testing.T) {
	p := newInstallPaths(t)
	const original = "-- mine\nrex.x = 1"
	if err := os.MkdirAll(filepath.Dir(p.cfg), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p.cfg, []byte(original), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.Chmod(p.cfg, 0o600); err != nil {
		t.Fatalf("chmod config: %v", err)
	}

	o := p.opts()
	o.Write = true
	o.In = strings.NewReader("y\n")
	out, err := runInstall(t, o)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	line := DofileLine(p.mod)
	if got, want := string(installMustRead(t, p.cfg)), original+"\n"+line+"\n"; got != want {
		t.Fatalf("config = %q; want %q", got, want)
	}
	if got := installMustPerm(t, p.cfg); got != 0o600 {
		t.Fatalf("config mode = %o; want 0600", got)
	}

	b := installBackups(t, p.cfg)
	if len(b) != 1 {
		t.Fatalf("installBackups = %v; want exactly one", b)
	}
	if got := string(installMustRead(t, b[0])); got != original {
		t.Fatalf("backup = %q; want %q", got, original)
	}
	if got := installMustPerm(t, b[0]); got != 0o600 {
		t.Fatalf("backup mode = %o; want 0600", got)
	}
	if want := "appended to " + p.cfg + " (backup: " + b[0] + ")"; !strings.Contains(out, want) {
		t.Fatalf("output = %q; want it to contain %q", out, want)
	}
}

func TestInstallWriteNewConfig(t *testing.T) {
	p := newInstallPaths(t)
	o := p.opts()
	o.Write = true
	o.In = strings.NewReader("yes\n")
	out, err := runInstall(t, o)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	line := DofileLine(p.mod)
	if got, want := string(installMustRead(t, p.cfg)), line+"\n"; got != want {
		t.Fatalf("config = %q; want %q", got, want)
	}
	if got := installMustPerm(t, p.cfg); got != 0o644 {
		t.Fatalf("config mode = %o; want 0644", got)
	}
	if b := installBackups(t, p.cfg); len(b) != 0 {
		t.Fatalf("installBackups = %v; want none", b)
	}
	if want := "appended to " + p.cfg; !strings.Contains(out, want) {
		t.Fatalf("output = %q; want it to contain %q", out, want)
	}
	if strings.Contains(out, "backup:") {
		t.Fatalf("output = %q; want no backup mention", out)
	}
}

func TestInstallWriteNo(t *testing.T) {
	const original = "-- mine\n"
	for _, tt := range []struct {
		name   string
		answer string
	}{
		{"n", "n\n"},
		{"eof", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := newInstallPaths(t)
			if err := os.MkdirAll(filepath.Dir(p.cfg), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(p.cfg, []byte(original), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			o := p.opts()
			o.Write = true
			o.In = strings.NewReader(tt.answer)
			out, err := runInstall(t, o)
			if err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			if !strings.Contains(out, "not written") {
				t.Fatalf("output = %q; want it to contain %q", out, "not written")
			}
			if got := string(installMustRead(t, p.cfg)); got != original {
				t.Fatalf("config = %q; want it untouched at %q", got, original)
			}
			if b := installBackups(t, p.cfg); len(b) != 0 {
				t.Fatalf("installBackups = %v; want none", b)
			}
		})
	}
}

type installFailReader struct{ t *testing.T }

func (r installFailReader) Read([]byte) (int, error) {
	r.t.Errorf("Install read the confirmation input; want no prompt")
	return 0, io.EOF
}

func TestInstallAlreadyLoaded(t *testing.T) {
	p := newInstallPaths(t)
	original := "-- mine\n" + DofileLine(p.mod) + "\n"
	if err := os.MkdirAll(filepath.Dir(p.cfg), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p.cfg, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	o := p.opts()
	o.Write = true
	o.In = installFailReader{t: t}
	out, err := runInstall(t, o)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if want := p.cfg + " already loads it"; !strings.Contains(out, want) {
		t.Fatalf("output = %q; want it to contain %q", out, want)
	}
	if strings.Contains(out, "Add this line to") {
		t.Fatalf("output = %q; want no instructions when already loaded", out)
	}
	if got := string(installMustRead(t, p.cfg)); got != original {
		t.Fatalf("config = %q; want it untouched at %q", got, original)
	}
	if b := installBackups(t, p.cfg); len(b) != 0 {
		t.Fatalf("installBackups = %v; want none", b)
	}
}

func TestInstallEmptyModule(t *testing.T) {
	p := newInstallPaths(t)
	o := p.opts()
	o.Module = nil
	_, err := runInstall(t, o)
	if err == nil {
		t.Fatal("Install() with no module returned no error")
	}
	if want := "rex install: empty module"; err.Error() != want {
		t.Fatalf("error = %q; want %q", err, want)
	}
	installMustNotExist(t, p.mod)
	installMustNotExist(t, p.cfg)
}
