package main

import (
	"context"
	"errors"
	"io"
	"os"
)

// Entry points built in later phases.

func runPush(args []string, stdin *os.File, stdout io.Writer) int { return 0 }

func runWatch(args []string, stderr io.Writer) error { return errors.New("watch: not built yet") }

func runRex(args []string, stdin *os.File, stdout io.Writer) error {
	return errors.New("rex: not built yet")
}

func runLazygit(args []string, stdin *os.File, stdout io.Writer) error {
	return errors.New("lazygit: not built yet")
}

func runTUI(ctx context.Context, a *app, o oneShotOpts, stdin *os.File, providerName string) error {
	return errors.New("the TUI is not built yet; use --plain, --json or --dry-run")
}
