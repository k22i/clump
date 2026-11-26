package ipc

import (
	"bufio"
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
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const Version = 1

const MaxMsgBytes = 64 << 20

const (
	DialTimeout  = 50 * time.Millisecond
	WriteTimeout = 100 * time.Millisecond
)

const readTimeout = 2 * time.Second

type Msg struct {
	V        int      `json:"v"`
	SentNS   int64    `json:"sent_ns"`
	RepoRoot string   `json:"repo_root"`
	GitArgv  []string `json:"git_argv,omitempty"`
	RexBlock string   `json:"rex_block,omitempty"`
	Diff     string   `json:"diff"`
}

var (
	ErrInUse    = errors.New("another clump watch is already running for this repo")
	ErrTooLarge = errors.New("ipc message too large")
)

func Encode(w io.Writer, m Msg) error {
	return json.NewEncoder(w).Encode(m)
}

func Decode(r io.Reader) (Msg, error) {
	var m Msg
	br := bufio.NewReader(io.LimitReader(r, MaxMsgBytes+1))
	line, err := br.ReadBytes('\n')
	switch {
	case len(line) == 0:
		if err == nil {
			return Msg{}, io.EOF
		}
		if errors.Is(err, io.EOF) {
			return Msg{}, io.EOF
		}
		return Msg{}, err
	case line[len(line)-1] == '\n':
		line = line[:len(line)-1]
	case len(line) >= MaxMsgBytes+1:
		return Msg{}, ErrTooLarge
	}
	if err := json.Unmarshal(line, &m); err != nil {
		return Msg{}, err
	}
	if m.V != Version {
		return Msg{}, fmt.Errorf("ipc: unsupported message version %d", m.V)
	}
	return m, nil
}

func SocketDir() string {
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); runtime != "" && filepath.IsAbs(runtime) {
		return filepath.Join(runtime, "clump")
	}
	return filepath.Join(os.TempDir(), "clump")
}

func SocketPath(repoRoot string) string {
	sum := sha256.Sum256([]byte(repoRoot))
	name := hex.EncodeToString(sum[:])[:16] + ".sock"
	p := filepath.Join(SocketDir(), name)
	if len(p) > MaxSocketPath {
		p = filepath.Join("/tmp", fmt.Sprintf("clump-%d", os.Getuid()), name)
	}
	return p
}

const MaxSocketPath = 103

func FindRepoRoot(dir string) string {
	for d := filepath.Clean(dir); ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

type Listener struct {
	path string

	ln        net.Listener
	sockInfo  os.FileInfo
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func Listen(path string) (*Listener, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ipc: create socket dir: %w", err)
	}
	if err := checkOwnDir(dir); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ipc: chmod socket dir: %w", err)
	}

	_, statErr := os.Lstat(path)
	switch {
	case statErr == nil:
		if c, err := net.DialTimeout("unix", path, DialTimeout); err == nil {
			c.Close()
			return nil, ErrInUse
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("ipc: remove stale socket: %w", err)
		}
	case !errors.Is(statErr, os.ErrNotExist):
		return nil, fmt.Errorf("ipc: stat socket: %w", statErr)
	}

	old := unix.Umask(0o177)
	ln, err := net.Listen("unix", path)
	unix.Umask(old)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen: %w", err)
	}
	if ul, ok := ln.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(false)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("ipc: chmod socket: %w", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("ipc: stat new socket: %w", err)
	}
	return &Listener{path: path, ln: ln, sockInfo: fi, done: make(chan struct{})}, nil
}

func (l *Listener) Path() string { return l.path }

func (l *Listener) Serve(ctx context.Context, handle func(Msg)) error {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			l.Close()
		case <-l.done:
		case <-stop:
		}
	}()

	var failures int
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			if l.closed() || ctx.Err() != nil {
				return nil
			}
			failures++
			if failures >= 100 {
				return fmt.Errorf("ipc: accept: %w", err)
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		failures = 0
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		m, err := Decode(conn)
		conn.Close()
		if err != nil {
			continue
		}
		handle(m)
	}
}

func (l *Listener) closed() bool {
	select {
	case <-l.done:
		return true
	default:
		return false
	}
}

func (l *Listener) Close() error {
	l.closeOnce.Do(func() {
		close(l.done)
		err := l.ln.Close()
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		if rmErr := l.removeSocket(); rmErr != nil && err == nil {
			err = rmErr
		}
		l.closeErr = err
	})
	return l.closeErr
}

func (l *Listener) removeSocket() error {
	fi, err := os.Stat(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("ipc: stat socket: %w", err)
	}
	if l.sockInfo == nil || !os.SameFile(l.sockInfo, fi) {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("ipc: remove socket: %w", err)
	}
	return nil
}

func Send(path string, m Msg, dialTimeout, writeTimeout time.Duration) error {
	conn, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		conn.Close()
		return err
	}
	writeErr := Encode(conn, m)
	if closeErr := conn.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr
}

func ParentArgv() []string {
	return parentArgv()
}

func checkOwnDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("ipc: socket dir: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("ipc: socket dir %s is not a directory", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("ipc: socket dir %s is owned by uid %d, not %d", dir, st.Uid, os.Getuid())
	}
	return nil
}
