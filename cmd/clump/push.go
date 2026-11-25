package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/ipc"
)

// runPush is lazygit's diff filter: pass the diff through unchanged (or to
// --then), then forward it to a running watch. It always exits 0 and never
// prints anything of its own, so lazygit's panel is never disturbed.
func runPush(args []string, stdin *os.File, stdout io.Writer) int {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	then := fs.String("then", "", "")
	_ = fs.Parse(args) // unknown flags are ignored: push must never fail

	if isTerminal(stdin) {
		return 0 // lazygit's startup probe: nothing to filter
	}
	data, _ := io.ReadAll(stdin) // a short read still passes through what arrived

	// 1. The diff reaches lazygit first, untouched.
	if *then != "" {
		cmd := exec.Command("/bin/sh", "-c", *then)
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stdout, cmd.Stderr = stdout, os.Stderr
		if cmd.Run() != nil {
			// The renderer failed: fall back to the raw diff so the panel is
			// never empty.
			_, _ = stdout.Write(data)
		}
	} else {
		_, _ = stdout.Write(data)
	}
	if f, ok := stdout.(*os.File); ok {
		_ = f.Close() // lazygit renders as soon as the pipe closes
	}

	// 2. Fire and forget to watch.
	if len(data) == 0 {
		return 0
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0
	}
	root := ipc.FindRepoRoot(cwd)
	if root == "" {
		return 0
	}
	m := ipc.Msg{V: ipc.Version, SentNS: time.Now().UnixNano(), RepoRoot: root, GitArgv: ipc.ParentArgv(),
		RexBlock: os.Getenv("REX_BLOCK"), Diff: gitdiff.StripSGR(string(data))}
	_ = ipc.Send(ipc.SocketPath(root), m, ipc.DialTimeout, ipc.WriteTimeout)
	return 0
}
