// Package ipc is the protocol between `clump push` (run by lazygit as a diff
// filter) and `clump watch` (the pane that follows lazygit): one JSON line
// per message over a per-repo Unix socket that only the user can open.
package ipc

import (
	"context"
	"errors"
	"io"
	"time"
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
	panic("ipc: Encode not implemented")
}

// Decode reads one message written by Encode. It reads at most MaxMsgBytes
// (+1 to detect overflow) and returns ErrTooLarge beyond that. It rejects
// V != Version with an error naming the version.
func Decode(r io.Reader) (Msg, error) {
	panic("ipc: Decode not implemented")
}

// SocketDir is $XDG_RUNTIME_DIR/clump if XDG_RUNTIME_DIR is set and
// absolute, else os.TempDir()/clump.
func SocketDir() string {
	panic("ipc: SocketDir not implemented")
}

// SocketPath is SocketDir()/<first 16 hex chars of sha256(repoRoot)>.sock.
func SocketPath(repoRoot string) string {
	panic("ipc: SocketPath not implemented")
}

// FindRepoRoot walks up from dir to the first directory containing a
// ".git" entry (directory or file, so worktrees work) and returns it, or ""
// if there is none. It does not run git (push must stay fast).
func FindRepoRoot(dir string) string {
	panic("ipc: FindRepoRoot not implemented")
}

// Listener accepts pushed messages.
type Listener struct {
	path string
	impl listenerImpl
}

type listenerImpl interface {
	Close() error
}

// Listen creates the socket at path. The parent directory is created with
// mode 0700 (and chmod-ed to 0700 if it exists); the socket gets mode 0600.
// If path exists: when a dial to it succeeds within DialTimeout, Listen
// returns ErrInUse; otherwise the stale file is removed first.
func Listen(path string) (*Listener, error) {
	panic("ipc: Listen not implemented")
}

// Path returns the socket path.
func (l *Listener) Path() string { return l.path }

// Serve accepts connections until ctx is done or Close is called, decoding
// one Msg per connection (with a 2 s read deadline) and passing it to
// handle, sequentially in arrival order. Undecodable messages are dropped.
// It returns nil after Close or ctx cancellation.
func (l *Listener) Serve(ctx context.Context, handle func(Msg)) error {
	panic("ipc: Listener.Serve not implemented")
}

// Close stops Serve and removes the socket file. Safe to call twice.
func (l *Listener) Close() error {
	panic("ipc: Listener.Close not implemented")
}

// Send dials path with dialTimeout, writes m with a write deadline of
// writeTimeout, and closes. It never blocks longer than about
// dialTimeout + writeTimeout.
func Send(path string, m Msg, dialTimeout, writeTimeout time.Duration) error {
	panic("ipc: Send not implemented")
}

// ParentArgv returns the argv of this process's parent: kern.procargs2 via
// sysctl on darwin, /proc/<ppid>/cmdline on linux, nil elsewhere or on any
// error.
func ParentArgv() []string {
	return parentArgv()
}
