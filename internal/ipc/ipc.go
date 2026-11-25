// Package ipc is the protocol between `clump push` (run by lazygit as a diff
// filter) and `clump watch` (the pane that follows lazygit): one JSON line
// per message over a per-repo Unix socket that only the user can open.
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
	"time"

	"golang.org/x/sys/unix"
)

// Version is Msg.V.
const Version = 1

// MaxMsgBytes caps one encoded message (a diff can be large).
const MaxMsgBytes = 64 << 20

// Default timeouts for Send, as measured in the spike.
const (
	DialTimeout  = 50 * time.Millisecond
	WriteTimeout = 100 * time.Millisecond
)

// readTimeout bounds how long Serve waits for one connection's message.
const readTimeout = 2 * time.Second

// Msg is one selection change pushed from lazygit.
type Msg struct {
	V int `json:"v"`
	// SentNS is time.Now().UnixNano() when push sent it. Watch drops a
	// message older than the newest it has seen.
	SentNS   int64  `json:"sent_ns"`
	RepoRoot string `json:"repo_root"`
	// GitArgv is the parent git process's argv (what lazygit asked git
	// for), or nil if it could not be read.
	GitArgv []string `json:"git_argv,omitempty"`
	// RexBlock is $REX_BLOCK of the lazygit pane, if any.
	RexBlock string `json:"rex_block,omitempty"`
	// Diff is the filter's stdin with SGR sequences stripped.
	Diff string `json:"diff"`
}

// Errors.
var (
	// ErrInUse: another live listener owns the socket.
	ErrInUse = errors.New("another clump watch is already running for this repo")
	// ErrTooLarge: a message exceeded MaxMsgBytes.
	ErrTooLarge = errors.New("ipc message too large")
)

// Encode writes m as one JSON object followed by "\n".
func Encode(w io.Writer, m Msg) error {
	return json.NewEncoder(w).Encode(m)
}

// Decode reads one message written by Encode. It reads at most MaxMsgBytes
// (+1 to detect overflow) and returns ErrTooLarge beyond that. It rejects
// V != Version with an error naming the version.
func Decode(r io.Reader) (Msg, error) {
	var m Msg
	// LimitReader makes the read stop one byte past the cap, so a message at
	// the limit is still accepted and anything beyond it is detectable.
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
		// No newline before the cap: the message cannot be accepted.
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

// SocketDir is $XDG_RUNTIME_DIR/clump if XDG_RUNTIME_DIR is set and
// absolute, else os.TempDir()/clump.
func SocketDir() string {
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); runtime != "" && filepath.IsAbs(runtime) {
		return filepath.Join(runtime, "clump")
	}
	return filepath.Join(os.TempDir(), "clump")
}

// SocketPath is SocketDir()/<first 16 hex chars of sha256(repoRoot)>.sock.
func SocketPath(repoRoot string) string {
	sum := sha256.Sum256([]byte(repoRoot))
	name := hex.EncodeToString(sum[:])[:16] + ".sock"
	return filepath.Join(SocketDir(), name)
}

// FindRepoRoot walks up from dir to the first directory containing a
// ".git" entry (directory or file, so worktrees work) and returns it, or ""
// if there is none. It does not run git (push must stay fast).
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

// Listener accepts pushed messages.
type Listener struct {
	path string

	ln net.Listener
	// sockInfo is the socket file as it was right after Listen; Close only
	// removes the file while it is still this one (os.SameFile compares the
	// underlying device and inode).
	sockInfo os.FileInfo
	// done is closed by Close; it wakes Serve and its context watcher.
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// Listen creates the socket at path. The parent directory is created with
// mode 0700 (and chmod-ed to 0700 if it exists); the socket gets mode 0600.
// If path exists: when a dial to it succeeds within DialTimeout, Listen
// returns ErrInUse; otherwise the stale file is removed first.
func Listen(path string) (*Listener, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ipc: create socket dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ipc: chmod socket dir: %w", err)
	}

	_, statErr := os.Lstat(path)
	switch {
	case statErr == nil:
		// Something is there: a live listener answers a dial, a leftover
		// socket or file does not and is safe to replace.
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

	// The umask decides the socket's initial mode; set it so the socket is
	// never briefly world-writable. Chmod below makes it exact.
	old := unix.Umask(0o177)
	ln, err := net.Listen("unix", path)
	unix.Umask(old)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen: %w", err)
	}
	// Close must be the only thing that unlinks the socket, and only while
	// the file is still ours.
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

// Path returns the socket path.
func (l *Listener) Path() string { return l.path }

// Serve accepts connections until ctx is done or Close is called, decoding
// one Msg per connection (with a 2 s read deadline) and passing it to
// handle, sequentially in arrival order. Undecodable messages are dropped.
// It returns nil after Close or ctx cancellation.
func (l *Listener) Serve(ctx context.Context, handle func(Msg)) error {
	// ctx cancellation closes the listener, which unblocks Accept. The
	// goroutine always exits before Serve returns.
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
			// Transient (e.g. EMFILE): retry, but do not spin forever.
			failures++
			if failures >= 100 {
				return fmt.Errorf("ipc: accept: %w", err)
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		failures = 0
		// One connection is handled at a time, in accept order.
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		m, err := Decode(conn)
		conn.Close()
		if err != nil {
			continue // garbage or a timeout: drop it
		}
		handle(m)
	}
}

// closed reports whether Close has been called.
func (l *Listener) closed() bool {
	select {
	case <-l.done:
		return true
	default:
		return false
	}
}

// Close stops Serve and removes the socket file. Safe to call twice.
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

// removeSocket unlinks the socket path, but only while it is still the file
// this listener created: a newer watch may already have replaced it.
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

// Send dials path with dialTimeout, writes m with a write deadline of
// writeTimeout, and closes. It never blocks longer than about
// dialTimeout + writeTimeout.
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

// ParentArgv returns the argv of this process's parent: kern.procargs2 via
// sysctl on darwin, /proc/<ppid>/cmdline on linux, nil elsewhere or on any
// error.
func ParentArgv() []string {
	return parentArgv()
}
