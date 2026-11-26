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

func runPush(args []string, stdin *os.File, stdout io.Writer) int {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	then := fs.String("then", "", "")
	_ = fs.Parse(args)

	if isTerminal(stdin) {
		return 0
	}
	data, _ := io.ReadAll(stdin)

	if *then != "" {
		cmd := exec.Command("/bin/sh", "-c", *then)
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stdout, cmd.Stderr = stdout, os.Stderr
		if cmd.Run() != nil {
			_, _ = stdout.Write(data)
		}
	} else {
		_, _ = stdout.Write(data)
	}
	if f, ok := stdout.(*os.File); ok {
		_ = f.Close()
	}

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
