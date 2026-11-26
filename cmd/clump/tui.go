package main

import (
	"context"
	"io"
	"os"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/handoff"
	"github.com/k22i/clump/internal/tui"
	"github.com/k22i/clump/internal/tui/view"
)

func theme() view.Theme {
	return view.NewTheme(detectDark())
}

func runTUI(ctx context.Context, a *app, o oneShotOpts, stdin *os.File, providerName string) (*handoff.Request, error) {
	repo, _ := resolveRepo(ctx, o.repo)
	topts := tui.Options{
		Engine: a.eng, Store: a.store, Cfg: a.cfg, Repo: repo, Provider: providerName, Yes: o.yes,
		Theme: theme(), Pick: o.pick, PR: o.pr, Status: statusFunc(),
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
