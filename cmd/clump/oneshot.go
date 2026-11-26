package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/engine"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/report"
)

func configDefaultTOML() string   { return config.DefaultTOML }
func configPath() (string, error) { return config.Path() }

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func fileIsTerminal(w io.Writer) bool { f, ok := w.(*os.File); return ok && isTerminal(f) }

type oneShotOpts struct {
	repo, providerName, pr         string
	branch, jsonOut, plain, dryRun bool
	noCache, yes, pick             bool
	args                           []string
}

func parseOneShot(args []string) (oneShotOpts, error) {
	var o oneShotOpts
	fs := flag.NewFlagSet("clump", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.repo, "repo", "", "")
	fs.StringVar(&o.providerName, "provider", "", "")
	fs.StringVar(&o.pr, "pr", "", "")
	fs.BoolVar(&o.branch, "branch", false, "")
	fs.BoolVar(&o.jsonOut, "json", false, "")
	fs.BoolVar(&o.plain, "plain", false, "")
	fs.BoolVar(&o.dryRun, "dry-run", false, "")
	fs.BoolVar(&o.noCache, "no-cache", false, "")
	fs.BoolVar(&o.yes, "yes", false, "")
	fs.BoolVar(&o.pick, "pick", false, "")
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return o, usageError{err.Error()}
		}
		args = fs.Args()
		if len(args) > 0 {
			o.args = append(o.args, args[0])
			args = args[1:]
		}
	}
	if len(o.args) > 1 {
		return o, usageError{"want at most one revision or range"}
	}
	if o.branch && len(o.args) > 0 {
		return o, usageError{"--branch takes no revision"}
	}
	if o.jsonOut && o.plain {
		return o, usageError{"--json and --plain are exclusive"}
	}
	if o.pr != "" {
		if len(o.args) > 0 {
			return o, usageError{"--pr takes no revision"}
		}
		if o.branch {
			return o, usageError{"--branch and --pr are exclusive"}
		}
		if o.pick {
			return o, usageError{"--pick and --pr are exclusive"}
		}
		if _, err := gitdiff.ParsePRSpec(o.pr); err != nil {
			return o, usageError{err.Error()}
		}
	}
	return o, nil
}

type app struct {
	cfg   *config.Config
	store *cache.Store
	eng   *engine.Engine
}

func loadApp(noCache bool) (*app, error) {
	p, err := config.Path()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(p)
	if err != nil {
		return nil, err
	}
	reg, err := cfg.Registry()
	if err != nil {
		return nil, err
	}
	store, err := cache.Open(cache.Options{NoCache: noCache})
	if err != nil {
		return nil, err
	}
	return &app{cfg: cfg, store: store, eng: engine.New(cfg, reg, store)}, nil
}

func resolveRepo(ctx context.Context, dir string) (string, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = wd
	}
	return gitdiff.RepoRoot(ctx, dir)
}

func loadInput(ctx context.Context, o oneShotOpts, stdin *os.File) (*gitdiff.Loaded, string, error) {
	if o.pr != "" {
		repo, _ := resolveRepo(ctx, o.repo)
		l, err := gitdiff.LoadPR(ctx, repo, o.pr)
		return l, repo, err
	}
	if len(o.args) == 0 && !o.branch && !isTerminal(stdin) {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, "", err
		}
		l, err := gitdiff.LoadPatch(string(b))
		repo, _ := resolveRepo(ctx, o.repo)
		return l, repo, err
	}
	repo, err := resolveRepo(ctx, o.repo)
	if err != nil {
		return nil, "", err
	}
	if o.branch {
		l, err := gitdiff.LoadBranch(ctx, repo)
		return l, repo, err
	}
	spec := "HEAD"
	if len(o.args) == 1 {
		spec = o.args[0]
	}
	if base, head, ok := strings.Cut(spec, "..."); ok {
		if head == "" {
			head = "HEAD"
		}
		l, err := gitdiff.LoadRange(ctx, repo, base, head)
		return l, repo, err
	}
	if from, to, ok := strings.Cut(spec, ".."); ok {
		if to == "" {
			to = "HEAD"
		}
		l, err := gitdiff.LoadTwoDot(ctx, repo, from, to)
		return l, repo, err
	}
	l, err := gitdiff.LoadRev(ctx, repo, spec)
	return l, repo, err
}

func runOneShot(args []string, stdin *os.File, stdout, stderr io.Writer) error {
	o, err := parseOneShot(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a, err := loadApp(o.noCache)
	if err != nil {
		return err
	}
	providerName := o.providerName
	if providerName == "" {
		providerName = a.cfg.Default
	}
	interactive := !o.jsonOut && !o.plain && !o.dryRun && fileIsTerminal(stdout)
	if interactive || o.pick {
		return runTUI(ctx, a, o, stdin, providerName)
	}
	l, repo, err := loadInput(ctx, o, stdin)
	if err != nil {
		return err
	}
	if len(l.Diff.Hunks) == 0 {
		return errors.New("the diff is empty")
	}
	a.eng.MarkGenerated(ctx, l.Diff, repo)
	in := engine.Input{Diff: l.Diff, Context: l.Context}
	if o.dryRun {
		j, err := a.eng.Prepare(in, providerName)
		if err != nil {
			return err
		}
		_, err = io.WriteString(stdout, a.eng.DryRun(j))
		return err
	}
	doc, err := groupForOutput(ctx, a, l, in, providerName, o, stderr)
	if err != nil {
		return err
	}
	if o.jsonOut {
		b, err := report.JSON(doc)
		if err != nil {
			return err
		}
		_, err = stdout.Write(b)
		return err
	}
	_, err = io.WriteString(stdout, report.Markdown(doc))
	return err
}

func groupForOutput(ctx context.Context, a *app, l *gitdiff.Loaded, in engine.Input, providerName string, o oneShotOpts, stderr io.Writer) (report.Doc, error) {
	d := l.Diff
	doc := report.Doc{Title: l.Title, Ref: l.Ref, Diff: d, DiffHash: d.Hash()}
	if ov, ok, err := a.store.LoadOverride(doc.DiffHash); err != nil {
		return doc, err
	} else if ok {
		doc.Snapshots = ov.Snapshots
		if len(ov.Grouping.Groups) > 0 && group.Check(d, ov.Grouping) == nil {
			doc.Grouping, doc.Edited = ov.Grouping, true
			doc.Provider = "edited"
			return doc, nil
		} else if len(ov.Grouping.Groups) > 0 {
			doc.Notices = append(doc.Notices, "ignored an override that no longer matches this diff")
		}
	}
	if out, ok, err := a.eng.Lookup(d, providerName); err != nil {
		return doc, err
	} else if ok {
		doc.Grouping, doc.Notices, doc.Fallback = out.Result.Grouping, append(doc.Notices, out.Result.Notices...), out.Result.Fallback
		doc.Provider, doc.Cached = out.Provider, true
		return doc, nil
	}
	j, err := a.eng.Prepare(in, providerName)
	if err != nil {
		return doc, err
	}
	if j.Est.OverCeiling && !o.yes {
		if !confirm(fmt.Sprintf("Send %s to %s (above the ceiling of %d tokens)? [y/N] ", j.Est, j.Provider, j.Est.Ceiling)) {
			return doc, fmt.Errorf("not sent: %s is above the ceiling of %d tokens (use --yes to send anyway)", j.Est, j.Est.Ceiling)
		}
	}
	progress := func(p engine.Progress) {
		if fileIsTerminal(stderr) {
			fmt.Fprintf(stderr, "clump: %s with %s (%d/%d)…\n", p.What, j.Provider, p.Step, p.Steps)
		}
	}
	out, err := a.eng.Run(ctx, j, progress)
	if err != nil {
		return doc, err
	}
	doc.Grouping, doc.Notices, doc.Fallback, doc.Provider = out.Result.Grouping, append(doc.Notices, out.Result.Notices...), out.Result.Fallback, out.Provider
	return doc, nil
}

func confirm(question string) bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer tty.Close()
	fmt.Fprint(tty, question)
	line, _ := bufio.NewReader(tty).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
