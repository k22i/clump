package ipc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ipc")
	if err != nil {
		t.Fatalf("os.MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func receive(t *testing.T, ch <-chan Msg) Msg {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a message")
		return Msg{}
	}
}

func expectNoMessage(t *testing.T, ch <-chan Msg, d time.Duration) {
	t.Helper()
	select {
	case m := <-ch:
		t.Fatalf("unexpected message: %+v", m)
	case <-time.After(d):
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Msg
	}{
		{
			name: "full message",
			msg: Msg{
				V:        Version,
				SentNS:   1732000000000000000,
				RepoRoot: "/Users/me/src/clump",
				GitArgv:  []string{"git", "show", "--stat", "-p", "abc123", "--"},
				RexBlock: "block-3",
				Diff:     "diff --git a/main.go b/main.go\n+naïve — ünïcode ✓\n-\told\n",
			},
		},
		{
			name: "minimal message",
			msg:  Msg{V: Version},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Encode(&buf, tc.msg); err != nil {
				t.Fatalf("Encode: %v", err)
			}
			enc := buf.Bytes()
			if !bytes.HasSuffix(enc, []byte("\n")) {
				t.Fatalf("encoded form does not end in a newline: %q", enc)
			}
			if n := bytes.Count(enc, []byte("\n")); n != 1 {
				t.Fatalf("encoded form has %d newlines, want 1: %q", n, enc)
			}

			got, err := Decode(bytes.NewReader(enc))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !reflect.DeepEqual(got, tc.msg) {
				t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, tc.msg)
			}
		})
	}
}

type filler struct{ n int64 }

func (f *filler) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > f.n {
		p = p[:f.n]
	}
	for i := range p {
		p[i] = 'x'
	}
	f.n -= int64(len(p))
	return len(p), nil
}

func TestDecodeErrors(t *testing.T) {
	t.Run("wrong version", func(t *testing.T) {
		var buf bytes.Buffer
		if err := Encode(&buf, Msg{V: 2, Diff: "x"}); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if _, err := Decode(bytes.NewReader(buf.Bytes())); err == nil {
			t.Fatal("Decode accepted version 2")
		} else if !strings.Contains(err.Error(), "version 2") {
			t.Fatalf("error %q does not name the version", err)
		}
	})

	t.Run("garbage", func(t *testing.T) {
		if _, err := Decode(strings.NewReader("not json at all\n")); err == nil {
			t.Fatal("Decode accepted garbage")
		}
	})

	t.Run("empty", func(t *testing.T) {
		if _, err := Decode(strings.NewReader("")); !errors.Is(err, io.EOF) {
			t.Fatalf("Decode of an empty reader = %v, want io.EOF", err)
		}
	})

	t.Run("too large", func(t *testing.T) {
		src := &filler{n: MaxMsgBytes + 10}
		if _, err := Decode(src); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("Decode of an oversized message = %v, want ErrTooLarge", err)
		}
	})
}

func TestSocketPath(t *testing.T) {
	t.Run("absolute XDG_RUNTIME_DIR", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/501")
		sum := sha256.Sum256([]byte("/repo/root"))
		want := "/run/user/501/clump/" + hex.EncodeToString(sum[:])[:16] + ".sock"
		if got := SocketPath("/repo/root"); got != want {
			t.Fatalf("SocketPath = %q, want %q", got, want)
		}
	})

	t.Run("relative XDG_RUNTIME_DIR falls back to TempDir", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "relative/run")
		got := SocketPath("/repo/root")
		wantDir := filepath.Join(os.TempDir(), "clump")
		if filepath.Dir(got) != wantDir {
			t.Fatalf("SocketPath = %q, want a path in %q", got, wantDir)
		}
	})

	t.Run("stable and distinct", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "")
		a1 := SocketPath("/repo/a")
		a2 := SocketPath("/repo/a")
		b := SocketPath("/repo/b")
		if a1 != a2 {
			t.Fatalf("same root gave %q and %q", a1, a2)
		}
		if a1 == b {
			t.Fatalf("different roots share the socket path %q", a1)
		}
	})

	t.Run("name is the sha256 prefix", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/501")
		root := "/repo/root"
		base := filepath.Base(SocketPath(root))
		if len(base) != len("0123456789abcdef.sock") {
			t.Fatalf("socket name %q has the wrong length", base)
		}
		if !strings.HasSuffix(base, ".sock") {
			t.Fatalf("socket name %q does not end in .sock", base)
		}
		sum := sha256.Sum256([]byte(root))
		if want := hex.EncodeToString(sum[:])[:16] + ".sock"; base != want {
			t.Fatalf("socket name = %q, want %q", base, want)
		}
		for _, r := range strings.TrimSuffix(base, ".sock") {
			if !strings.ContainsRune("0123456789abcdef", r) {
				t.Fatalf("socket name %q is not lowercase hex", base)
			}
		}
	})
}

func TestFindRepoRoot(t *testing.T) {
	t.Run(".git directory", func(t *testing.T) {
		a := t.TempDir()
		deep := filepath.Join(a, "b", "c")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(a, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := FindRepoRoot(deep); got != a {
			t.Fatalf("FindRepoRoot(%q) = %q, want %q", deep, got, a)
		}
	})

	t.Run(".git file (worktree)", func(t *testing.T) {
		a := t.TempDir()
		deep := filepath.Join(a, "sub")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(a, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := FindRepoRoot(deep); got != a {
			t.Fatalf("FindRepoRoot(%q) = %q, want %q", deep, got, a)
		}
	})

	t.Run("no .git", func(t *testing.T) {
		if got := FindRepoRoot("/"); got != "" {
			t.Fatalf("FindRepoRoot(%q) = %q, want \"\"", "/", got)
		}
		dir := t.TempDir()
		got := FindRepoRoot(dir)
		if got == "" {
			return
		}
		if _, err := os.Lstat(filepath.Join(got, ".git")); err != nil {
			t.Fatalf("FindRepoRoot(%q) = %q, which has no .git", dir, got)
		}
	})
}

func TestListenPermissions(t *testing.T) {
	path := shortSocketPath(t)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %#o, want 0600", got)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat socket dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Fatalf("socket dir mode = %#o, want 0700", got)
	}
}

func TestListenStaleSocket(t *testing.T) {
	path := shortSocketPath(t)
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen over a stale file: %v", err)
	}
	defer l.Close()

	fi, err := os.Stat(l.Path())
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("path mode = %v, want a socket", fi.Mode())
	}
}

func TestListenInUse(t *testing.T) {
	path := shortSocketPath(t)
	first, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer first.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Msg, 4)
	go func() { _ = first.Serve(ctx, func(m Msg) { got <- m }) }()

	second, err := Listen(path)
	if !errors.Is(err, ErrInUse) {
		t.Fatalf("second Listen = (%v, %v), want ErrInUse", second, err)
	}
	if second != nil {
		second.Close()
	}

	want := Msg{V: Version, SentNS: 7, RepoRoot: "/r", Diff: "after in-use\n"}
	if err := Send(path, want, DialTimeout, time.Second); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msg := receive(t, got); !reflect.DeepEqual(msg, want) {
		t.Fatalf("got %+v, want %+v", msg, want)
	}
}

func TestServeReceivesInOrder(t *testing.T) {
	path := shortSocketPath(t)
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var inFlight atomic.Int32
	var overlaps atomic.Int32
	got := make(chan Msg, 64)
	served := make(chan error, 1)
	go func() {
		served <- l.Serve(ctx, func(m Msg) {
			if inFlight.Add(1) != 1 {
				overlaps.Add(1)
			}
			got <- m
			inFlight.Add(-1)
		})
	}()

	const n = 20
	for i := 0; i < n; i++ {
		m := Msg{V: Version, SentNS: int64(i), RepoRoot: "/r", Diff: fmt.Sprintf("diff %d\n", i)}
		if err := Send(path, m, DialTimeout, time.Second); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		m := receive(t, got)
		if m.SentNS != int64(i) {
			t.Fatalf("message %d has SentNS %d, messages arrived out of order", i, m.SentNS)
		}
		if want := fmt.Sprintf("diff %d\n", i); m.Diff != want {
			t.Fatalf("message %d has diff %q, want %q", i, m.Diff, want)
		}
		if o := overlaps.Load(); o != 0 {
			t.Fatalf("%d overlapping handle calls", o)
		}
	}
	expectNoMessage(t, got, 100*time.Millisecond)

	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after cancellation")
	}
}

func TestServeDropsGarbage(t *testing.T) {
	path := shortSocketPath(t)
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan Msg, 4)
	go func() { _ = l.Serve(ctx, func(m Msg) { got <- m }) }()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := conn.Write([]byte("nonsense\n")); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close garbage conn: %v", err)
	}

	want := Msg{V: Version, SentNS: 1, RepoRoot: "/r", Diff: "valid\n"}
	if err := Send(path, want, DialTimeout, time.Second); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msg := receive(t, got); !reflect.DeepEqual(msg, want) {
		t.Fatalf("got %+v, want %+v", msg, want)
	}
	expectNoMessage(t, got, 200*time.Millisecond)
}

func TestServeStopsOnCtx(t *testing.T) {
	path := shortSocketPath(t)
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- l.Serve(ctx, func(Msg) {}) }()

	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return within 1s of cancellation")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		now := runtime.NumGoroutine()
		if now <= before {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines after Serve returned, started with %d (leak)", now, before)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestCloseRemovesOnlyOwnSocket(t *testing.T) {
	t.Run("removes its socket and is idempotent", func(t *testing.T) {
		path := shortSocketPath(t)
		l, err := Listen(path)
		if err != nil {
			t.Fatalf("Listen: %v", err)
		}
		if err := l.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("socket still present after Close: %v", err)
		}
		if err := l.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
	})

	t.Run("leaves a newer socket alone", func(t *testing.T) {
		path := shortSocketPath(t)
		old, err := Listen(path)
		if err != nil {
			t.Fatalf("Listen: %v", err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove socket by hand: %v", err)
		}
		newer, err := Listen(path)
		if err != nil {
			t.Fatalf("Listen (newer): %v", err)
		}
		defer newer.Close()

		if err := old.Close(); err != nil {
			t.Fatalf("old Close: %v", err)
		}
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("older listener deleted the newer socket: %v", err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat newer socket: %v", err)
		}
		if fi.Mode()&os.ModeSocket == 0 {
			t.Fatalf("path mode = %v, want the newer socket", fi.Mode())
		}
	})
}

func TestSendNoListener(t *testing.T) {
	msg := Msg{V: Version, Diff: "x\n"}
	const limit = 100 * time.Millisecond

	t.Run("no socket file", func(t *testing.T) {
		path := shortSocketPath(t)
		start := time.Now()
		if err := Send(path, msg, DialTimeout, WriteTimeout); err == nil {
			t.Fatal("Send to a missing socket succeeded")
		}
		if d := time.Since(start); d > limit {
			t.Fatalf("Send took %v, want under %v", d, limit)
		}
	})

	t.Run("stale socket file", func(t *testing.T) {
		path := shortSocketPath(t)
		ul, err := net.Listen("unix", path)
		if err != nil {
			t.Fatalf("net.Listen: %v", err)
		}
		ul.(*net.UnixListener).SetUnlinkOnClose(false)
		if err := ul.Close(); err != nil {
			t.Fatalf("close listener: %v", err)
		}

		start := time.Now()
		err = Send(path, msg, DialTimeout, WriteTimeout)
		if err == nil {
			t.Fatal("Send to a stale socket succeeded")
		}
		if !errors.Is(err, syscall.ECONNREFUSED) {
			t.Fatalf("Send error = %v, want ECONNREFUSED", err)
		}
		if d := time.Since(start); d > limit {
			t.Fatalf("Send took %v, want under %v", d, limit)
		}
	})
}

func TestSendWriteDeadline(t *testing.T) {
	path := shortSocketPath(t)
	ul, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := ul.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		ul.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})

	msg := Msg{V: Version, RepoRoot: "/r", Diff: strings.Repeat("x", 32<<20)}
	start := time.Now()
	err = Send(path, msg, DialTimeout, WriteTimeout)
	if err == nil {
		t.Fatal("Send of a 32 MiB message into a non-reading listener succeeded")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Send took %v, want the write deadline to fire well under 1s", d)
	}
}

func TestEncodeIsOneJSONObject(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, Msg{V: Version, SentNS: 42, Diff: "a\nb\n"}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(bytes.TrimSuffix(buf.Bytes(), []byte("\n")), &raw); err != nil {
		t.Fatalf("encoded message is not one JSON object: %v", err)
	}
	for _, key := range []string{"v", "sent_ns", "repo_root", "diff"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("encoded message has no %q field: %v", key, raw)
		}
	}
}
