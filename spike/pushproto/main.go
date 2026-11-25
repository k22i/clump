// pushproto is a throwaway prototype of `clump push` (a lazygit stdinFilter) and a
// minimal `watch` listener, to measure the latency budget and the IPC shape.
//
//	pushproto push    # stdin -> stdout unchanged; fire-and-forget copy to the socket
//	pushproto listen  # print one line per received message
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type Msg struct {
	V        int      `json:"v"`
	SentNS   int64    `json:"sent_ns"`
	RepoRoot string   `json:"repo_root"`
	GitArgv  []string `json:"git_argv,omitempty"` // parent git command line, if readable
	RexBlock string   `json:"rex_block,omitempty"`
	Diff     string   `json:"diff"`
}

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

func repoRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if p := filepath.Dir(d); p == d {
			return ""
		}
	}
}

func sockPath(root string) string {
	h := sha256.Sum256([]byte(root))
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "clump", hex.EncodeToString(h[:8])+".sock")
}

// parentArgv reads the parent's argv without cgo: kern.procargs2 on darwin, /proc on linux.
func parentArgv() []string {
	ppid := os.Getppid()
	switch runtime.GOOS {
	case "linux":
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", ppid))
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
	case "darwin":
		b, err := unix.SysctlRaw("kern.procargs2", ppid)
		if err != nil || len(b) < 4 {
			return nil
		}
		argc := int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24
		rest := b[4:]
		// skip exec path and padding NULs
		i := bytes.IndexByte(rest, 0)
		if i < 0 {
			return nil
		}
		rest = rest[i:]
		for len(rest) > 0 && rest[0] == 0 {
			rest = rest[1:]
		}
		var out []string
		for len(out) < argc && len(rest) > 0 {
			j := bytes.IndexByte(rest, 0)
			if j < 0 {
				out = append(out, string(rest))
				break
			}
			out = append(out, string(rest[:j]))
			rest = rest[j+1:]
		}
		return out
	}
	return nil
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func push() {
	start := time.Now()
	if isTTY(os.Stdin) { // lazygit's startup probe: no diff, nothing to forward
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	os.Stdout.Write(data) // pass through unchanged, first, so lazygit never waits on us
	os.Stdout.Close()
	cwd, _ := os.Getwd()
	root := repoRoot(cwd)
	note := func(s string) {
		if p := os.Getenv("PUSHPROTO_LOG"); p != "" {
			if f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				fmt.Fprintf(f, "%s total=%.2fms bytes=%d\n", s, float64(time.Since(start).Microseconds())/1000, len(data))
				f.Close()
			}
		}
	}
	if root == "" || len(data) == 0 {
		note("skip-no-root-or-empty")
		return
	}
	c, err := net.DialTimeout("unix", sockPath(root), 50*time.Millisecond)
	if err != nil {
		note("no-listener")
		return
	}
	c.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	m := Msg{V: 1, SentNS: time.Now().UnixNano(), RepoRoot: root, GitArgv: parentArgv(), RexBlock: os.Getenv("REX_BLOCK"),
		Diff: sgr.ReplaceAllString(string(data), "")}
	err = json.NewEncoder(c).Encode(m)
	c.Close()
	if err != nil {
		note("send-error")
		return
	}
	note("sent")
}

func listen(root string) {
	p := sockPath(root)
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.Chmod(filepath.Dir(p), 0o700)
	os.Remove(p)
	old := unix.Umask(0o177)
	l, err := net.Listen("unix", p)
	unix.Umask(old)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fi, _ := os.Stat(p)
	di, _ := os.Stat(filepath.Dir(p))
	fmt.Printf("listening %s sock=%v dir=%v\n", p, fi.Mode().Perm(), di.Mode().Perm())
	for {
		c, err := l.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			var m Msg
			if err := json.NewDecoder(c).Decode(&m); err != nil {
				fmt.Println("decode error:", err)
				return
			}
			first, _, _ := strings.Cut(m.Diff, "\n")
			kind := "?"
			for _, a := range m.GitArgv {
				if a == "show" || a == "diff" || a == "stash" || a == "log" {
					kind = a
					break
				}
			}
			fmt.Printf("recv lag=%.2fms kind=%s bytes=%d first=%q block=%s\n",
				float64(time.Now().UnixNano()-m.SentNS)/1e6, kind, len(m.Diff), first, m.RexBlock)
		}(c)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pushproto push | listen <repo-root>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "push":
		push()
	case "listen":
		listen(os.Args[2])
	}
}
