// lgcapture is a throwaway lazygit stdinFilter that records what lazygit pipes
// to it (stdin, argv, cwd, env, timing, signals) and passes stdin through.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type record struct {
	Seq       string            `json:"seq"`
	StartNS   int64             `json:"start_ns"`
	FirstByte int64             `json:"first_byte_ns,omitempty"`
	EOFNS     int64             `json:"eof_ns,omitempty"`
	EndNS     int64             `json:"end_ns,omitempty"`
	Signal    string            `json:"signal,omitempty"`
	WriteErr  string            `json:"write_err,omitempty"`
	PID       int               `json:"pid"`
	PPID      int               `json:"ppid"`
	Argv      []string          `json:"argv"`
	Cwd       string            `json:"cwd"`
	StdinTTY  bool              `json:"stdin_tty"`
	StdoutTTY bool              `json:"stdout_tty"`
	Bytes     int               `json:"bytes"`
	Env       map[string]string `json:"env"`
	Parent    string            `json:"parent_cmd"`
	Grand     string            `json:"grandparent_cmd"`
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func main() {
	dir := os.Getenv("LGCAPTURE_DIR")
	if dir == "" {
		dir = "/tmp/lgcapture"
	}
	os.MkdirAll(dir, 0o700)
	start := time.Now()
	seq := fmt.Sprintf("%d-%d", start.UnixNano(), os.Getpid())
	cwd, _ := os.Getwd()
	env := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GIT_") || strings.HasPrefix(k, "LAZYGIT") || strings.HasPrefix(k, "REX_") ||
			k == "LINES" || k == "COLUMNS" || k == "TERM" || k == "PAGER" || k == "LESS" || k == "COLORTERM" || k == "NO_COLOR" || k == "TERM_PROGRAM" {
			env[k] = v
		}
	}
	rec := record{Seq: seq, StartNS: start.UnixNano(), PID: os.Getpid(), PPID: os.Getppid(), Argv: os.Args, Cwd: cwd,
		StdinTTY: isTTY(os.Stdin), StdoutTTY: isTTY(os.Stdout), Env: env}
	pc := func(pid int) (string, int) {
		b, _ := exec.Command("ps", "-o", "ppid=,command=", "-p", fmt.Sprint(pid)).Output()
		t := strings.TrimSpace(string(b))
		pp, rest, _ := strings.Cut(t, " ")
		var n int
		fmt.Sscan(pp, &n)
		return strings.TrimSpace(rest), n
	}
	var gp int
	rec.Parent, gp = pc(os.Getppid())
	rec.Grand, _ = pc(gp)
	flush := func() {
		b, _ := json.Marshal(rec)
		f, err := os.OpenFile(filepath.Join(dir, "log.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			f.Write(append(b, '\n'))
			f.Close()
		}
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGPIPE)
	go func() {
		s := <-sigs
		rec.Signal = s.String()
		rec.EndNS = time.Now().UnixNano()
		flush()
		os.Exit(1)
	}()
	if d := os.Getenv("LGCAPTURE_DELAY_MS"); d != "" {
		var ms int
		fmt.Sscan(d, &ms)
		b, _ := json.Marshal(map[string]any{"started": seq})
		if f, err := os.OpenFile(filepath.Join(dir, "log.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			f.Write(append(b, 10))
			f.Close()
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	out, _ := os.Create(filepath.Join(dir, seq+".stdin"))
	r := bufio.NewReader(os.Stdin)
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if rec.FirstByte == 0 {
				rec.FirstByte = time.Now().UnixNano()
			}
			rec.Bytes += n
			out.Write(buf[:n])
			if _, werr := os.Stdout.Write(buf[:n]); werr != nil && rec.WriteErr == "" {
				rec.WriteErr = werr.Error()
			}
		}
		if err == io.EOF {
			rec.EOFNS = time.Now().UnixNano()
			break
		}
		if err != nil {
			rec.WriteErr = "read: " + err.Error()
			break
		}
	}
	out.Close()
	rec.EndNS = time.Now().UnixNano()
	flush()
}
