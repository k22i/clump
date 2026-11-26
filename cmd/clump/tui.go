package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/handoff"
	"github.com/k22i/clump/internal/tui"
	"github.com/k22i/clump/internal/tui/view"
)

func theme(stderr io.Writer) view.Theme {
	dark, err := detectDark()
	if err != nil {
		fmt.Fprintf(stderr, "clump: %s\n", err)
	}
	return view.NewTheme(dark)
}

func runTUI(ctx context.Context, a *app, o oneShotOpts, stdin *os.File, stderr io.Writer, providerName string) (*handoff.Request, error) {
	repo, _ := resolveRepo(ctx, o.repo)
	topts := tui.Options{
		Engine: a.eng, Store: a.store, Cfg: a.cfg, Repo: repo, Provider: providerName, Yes: o.yes,
		Theme: theme(stderr), Pick: o.pick, PR: o.pr, Status: statusFunc(),
	}
	if !o.pick {
		if len(o.args) == 0 && !o.branch && !isTerminal(stdin) {
			b, err := io.ReadAll(stdin)
			if err != nil {
				return nil, err
			}
			text := string(b)
			topts.Load = func(context.Context) (*gitdiff.Loaded, error) { return gitdiff.LoadPatch(text) }
		} else {
			topts.Load = func(ctx context.Context) (*gitdiff.Loaded, error) {
				l, _, err := loadInput(ctx, o, stdin)
				return l, err
			}
		}
	}
	return tui.Run(ctx, topts)
}
