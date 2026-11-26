package main

import (
	"context"
	"io"
	"os"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui"
	"github.com/k22i/clump/internal/tui/view"
)

// theme picks the colour theme: CLUMP_THEME, else the terminal's answer to
// the colour-scheme query, else dark.
func theme() view.Theme {
	return view.NewTheme(detectDark())
}

func runTUI(ctx context.Context, a *app, o oneShotOpts, stdin *os.File, providerName string) error {
	repo, _ := resolveRepo(ctx, o.repo)
	topts := tui.Options{
		Engine: a.eng, Store: a.store, Cfg: a.cfg, Repo: repo, Provider: providerName, Yes: o.yes,
		Theme: theme(), Pick: o.pick, PR: o.pr, Status: statusFunc(),
	}
	if !o.pick {
		if len(o.args) == 0 && !o.branch && !isTerminal(stdin) {
			// Read the patch now: the TUI then reads keys from /dev/tty.
			b, err := io.ReadAll(stdin)
			if err != nil {
				return err
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
