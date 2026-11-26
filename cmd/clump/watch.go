package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/k22i/clump/internal/ipc"
	"github.com/k22i/clump/internal/tui"
)

func runWatch(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repoFlag := fs.String("repo", "", "")
	providerFlag := fs.String("provider", "", "")
	logFile := fs.String("log", "", "")
	if err := fs.Parse(args); err != nil {
		return usageError{"watch: " + err.Error()}
	}
	if fs.NArg() > 0 {
		return usageError{"watch takes no arguments"}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	repo, err := resolveRepo(ctx, *repoFlag)
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	a, err := loadApp(false)
	if err != nil {
		return err
	}
	root := ipc.FindRepoRoot(repo)
	if root == "" {
		root = repo
	}
	l, err := ipc.Listen(ipc.SocketPath(root))
	if errors.Is(err, ipc.ErrInUse) {
		return fmt.Errorf("watch: %w (%s)", err, root)
	}
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	defer l.Close()
	var logw io.Writer
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("watch: %w", err)
		}
		defer f.Close()
		logw = f
	}
	providerName := *providerFlag
	if providerName == "" {
		providerName = "cheap"
	}
	_, err = tui.Run(ctx, tui.Options{
		Engine: a.eng, Store: a.store, Cfg: a.cfg, Repo: root, Provider: providerName, Theme: theme(), Status: statusFunc(),
		Watch: &tui.WatchOptions{Repo: root, Listener: l, AutoGroup: a.cfg.AutoGroup, FollowWorktree: a.cfg.FollowWorktree, Log: logw},
	})
	return err
}
