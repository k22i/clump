package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSocketPathFallsBackWhenTooLong(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/"+strings.Repeat("d", 120))
	p := SocketPath("/repo")
	if len(p) > MaxSocketPath || !strings.HasPrefix(p, filepath.Join("/tmp", "clump-")) || !strings.HasSuffix(p, ".sock") {
		t.Fatalf("SocketPath = %q (%d bytes)", p, len(p))
	}
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/501")
	if p := SocketPath("/repo"); !strings.HasPrefix(p, "/run/user/501/clump/") {
		t.Fatalf("short path changed: %q", p)
	}
}

func TestListenRefusesSymlinkDir(t *testing.T) {
	base, err := os.MkdirTemp("", "ipcl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(filepath.Join(link, "x.sock")); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("err = %v", err)
	}
}
