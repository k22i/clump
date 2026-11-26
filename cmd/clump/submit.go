package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/forge"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
)

type submitOpts struct {
	repo, pr, event, body, bodyFile string
	yes, dryRun                     bool
}

func parseSubmit(args []string) (submitOpts, error) {
	var o submitOpts
	fs := flag.NewFlagSet("clump submit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.repo, "repo", "", "")
	fs.StringVar(&o.pr, "pr", "", "")
	fs.StringVar(&o.event, "event", "", "")
	fs.StringVar(&o.body, "body", "", "")
	fs.StringVar(&o.bodyFile, "body-file", "", "")
	fs.BoolVar(&o.yes, "yes", false, "")
	fs.BoolVar(&o.dryRun, "dry-run", false, "")
	fs.Usage = func() {}
	_ = fs.Parse(args)
	if fs.NArg() > 0 {
		return o, usageError{"submit takes flags only, not \"" + fs.Arg(0) + "\""}
	}
	if o.pr == "" {
		return o, usageError{"submit needs --pr SPEC (123, #123, OWNER/REPO#123 or a pull request URL)"}
	}
	if _, err := gitdiff.ParsePRSpec(o.pr); err != nil {
		return o, usageError{err.Error()}
	}
	if o.event == "" {
		return o, usageError{"submit needs --event comment, approve or request-changes"}
	}
	if _, err := forge.ParseEvent(o.event); err != nil {
		return o, usageError{err.Error()}
	}
	if o.body != "" && o.bodyFile != "" {
		return o, usageError{"--body and --body-file are exclusive"}
	}
	return o, nil
}

func runSubmit(args []string, stdin *os.File, stdout, stderr io.Writer) error {
	o, err := parseSubmit(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a, err := loadApp(false)
	if err != nil {
		return err
	}
	event, _ := forge.ParseEvent(o.event)
	body := o.body
	if o.bodyFile != "" {
		b, err := readBodyFile(o.bodyFile, stdin)
		if err != nil {
			return err
		}
		body = b
	}
	t := oneShotOpts{repo: o.repo, pr: o.pr}
	l, _, err := loadInput(ctx, t, stdin)
	if err != nil {
		return err
	}
	if len(l.Diff.Hunks) == 0 {
		return errors.New("the pull request diff is empty")
	}
	placed := placedComments(a, l)
	review := forge.Build(event, body, placed, l.Diff, groupTitles(l, a))
	if review.Counts.Total() == 0 && review.Body == "" {
		return errors.New("nothing to submit: no comments on this pull request (write them with C in the TUI or clump comments add)")
	}
	if o.dryRun {
		payload, err := review.Payload()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "--pr %s --event %s\n%s\n", o.pr, event, review.Summary())
		_, err = stdout.Write(append(payload, '\n'))
		return err
	}
	if !o.yes {
		what := fmt.Sprintf("Submit %s as a review of %s — %s? [y/N] ", event, prLabel(l.Ref, o.pr), review.Summary())
		if !confirm(what) {
			return errors.New("not submitted (no terminal to confirm on; pass --yes to skip the question)")
		}
	}
	if fileIsTerminal(stderr) {
		fmt.Fprintln(stderr, "clump: submitting "+event.String()+" review…")
	}
	res, err := forge.Submit(ctx, o.repo, o.pr, review)
	if err != nil {
		return err
	}
	if res.URL != "" {
		fmt.Fprintf(stdout, "submitted %s review: %s\n", event, res.URL)
	} else {
		fmt.Fprintf(stdout, "submitted %s review of %s\n", event, prLabel(l.Ref, o.pr))
	}
	return nil
}

func readBodyFile(path string, stdin *os.File) (string, error) {
	if path == "-" {
		if isTerminal(stdin) {
			return "", errors.New("--body-file -: stdin is a terminal")
		}
		b, err := io.ReadAll(stdin)
		return string(b), err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func placedComments(a *app, l *gitdiff.Loaded) []comment.Placed {
	g, _ := savedGrouping(a, l, a.cfg.Default)
	return placedCommentsForGrouping(a, l, g)
}

func placedCommentsForGrouping(a *app, l *gitdiff.Loaded, g group.Grouping) []comment.Placed {
	var groupIDs []string
	for _, grp := range g.Groups {
		groupIDs = append(groupIDs, grp.ID)
	}
	anchors := comment.Anchors(l.Diff, l.Diff.Hash(), groupIDs)
	keys := make([]string, 0, len(anchors))
	for _, an := range anchors {
		keys = append(keys, an.Key)
	}
	stored, err := a.store.Comments(keys)
	if err != nil {
		return nil
	}
	return comment.Place(comment.Resolve(anchors, stored), l.Diff, g)
}

func groupTitles(l *gitdiff.Loaded, a *app) func(id string) string {
	g, ok := savedGrouping(a, l, a.cfg.Default)
	if !ok {
		return func(id string) string { return "" }
	}
	return func(id string) string {
		for _, grp := range g.Groups {
			if grp.ID == id {
				return grp.Title
			}
		}
		return ""
	}
}

func prLabel(ref, spec string) string {
	if ref != "" && ref != "stdin" {
		return ref
	}
	return spec
}
