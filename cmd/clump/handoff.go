package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/handoff"
)

var syscallExec = syscall.Exec

var handoffTerminal = isTerminal

type handoffOpts struct {
	repo, pr, agent, where string
	branch, yes, dryRun    bool
	args                   []string
}

func parseHandoffFlags(args []string) (handoffOpts, error) {
	var o handoffOpts
	fs := flag.NewFlagSet("clump handoff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.repo, "repo", "", "")
	fs.StringVar(&o.pr, "pr", "", "")
	fs.BoolVar(&o.branch, "branch", false, "")
	fs.StringVar(&o.agent, "agent", "", "")
	fs.StringVar(&o.where, "where", "", "")
	fs.BoolVar(&o.yes, "yes", false, "")
	fs.BoolVar(&o.dryRun, "dry-run", false, "")
	fs.Usage = func() {}
	for {
		if err := fs.Parse(args); err != nil {
			return o, usageError{err.Error()}
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		o.args = append(o.args, args[0])
		args = args[1:]
	}
	if len(o.args) > 1 {
		return o, usageError{"handoff wants at most one revision or range"}
	}
	if o.branch && (o.pr != "" || len(o.args) > 0) {
		return o, usageError{"--branch takes no --pr or revision"}
	}
	if o.pr != "" && len(o.args) > 0 {
		return o, usageError{"--pr takes no revision"}
	}
	if o.where != "" && !handoff.ValidWhere(o.where) {
		return o, usageError{"--where must be " + handoff.WhereReplace + ", " + handoff.WherePane + " or " + handoff.WherePrint}
	}
	return o, nil
}

func (o handoffOpts) diffOpts() commentsOpts {
	c := commentsOpts{repo: o.repo, pr: o.pr, branch: o.branch}
	if len(o.args) > 0 {
		c.args = []string{o.args[0]}
	}
	return c
}

func runHandoff(args []string, stdin *os.File, stdout, stderr io.Writer) error {
	o, err := parseHandoffFlags(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a, err := loadApp(false)
	if err != nil {
		return err
	}
	l, err := commentDiff(ctx, o.diffOpts(), stdin)
	if err != nil {
		return err
	}
	placed := placedComments(a, l)
	if len(placed) == 0 {
		return errors.New("nothing to hand off: no comments on this diff (write them with C in the TUI or clump comments add)")
	}
	agent, err := a.cfg.ResolveHandoffAgent(o.agent, exec.LookPath)
	if err != nil {
		return err
	}
	where := o.where
	if where == "" {
		where = agent.Where
	}
	prompt := handoff.Prompt(handoff.Review{Title: l.Title, Ref: l.Ref, Comments: placed, GroupTitle: groupTitles(l, a)})
	repo, err := resolveRepo(ctx, o.repo)
	if err != nil {
		return err
	}
	req := handoffRequest(agent, where, prompt, repo, placed)
	if o.dryRun {
		fmt.Fprintf(stdout, "handoff to %s (%s)\n\n$ %s\n\nprompt:\n\n%s\n", agent.Name, where, req.Command(), prompt)
		return nil
	}
	if where != handoff.WherePrint && !o.yes {
		if !confirm(fmt.Sprintf("Hand off %s to %s (%s)? [y/N] ", commentSummary(placed), agent.Name, where)) {
			return errors.New("not handed off (no terminal to confirm on; pass --yes to skip the question)")
		}
	}
	return deliverRequest(a, req, stdin, stdout)
}

func handoffRequest(agent handoff.Agent, where, prompt, repo string, placed []comment.Placed) *handoff.Request {
	return &handoff.Request{
		Agent: agent.Name, Where: where, Argv: agent.ArgvWith(prompt),
		Prompt: prompt, Dir: repo, Anchors: handoff.AnchorKeys(placed),
	}
}

func commentSummary(placed []comment.Placed) string {
	return fmt.Sprintf("%d comment(s)", len(placed))
}

func deliverRequest(a *app, req *handoff.Request, stdin *os.File, stdout io.Writer) error {
	stamp := func() error {
		if len(req.Anchors) == 0 {
			return nil
		}
		_, err := a.store.StampHandoff(req.Anchors, time.Now())
		return err
	}
	switch req.Where {
	case handoff.WherePrint:
		if _, err := io.WriteString(stdout, req.Prompt+"\n"); err != nil {
			return err
		}
		return stamp()
	case handoff.WhereReplace:
		if req.Path == "" {
			path, err := exec.LookPath(req.Argv[0])
			if err != nil {
				printHandoffFallback(stdout, req, err.Error())
				return fmt.Errorf("handoff: %s: the prompt and command were printed on stdout", err)
			}
			req.Path = path
		}
		if !handoffTerminal(stdin) {
			return errors.New("handoff: replace needs a terminal on stdin for the agent's UI; use --where pane or print in scripts")
		}
		if err := stamp(); err != nil {
			return err
		}
		if err := syscallExec(req.Path, req.Argv, os.Environ()); err != nil {
			printHandoffFallback(stdout, req, err.Error())
			return fmt.Errorf("handoff: %s: the prompt and command were printed on stdout", err)
		}
		return nil
	case handoff.WherePane:
		if _, err := exec.LookPath(req.Argv[0]); err != nil {
			printHandoffFallback(stdout, req, err.Error())
			return fmt.Errorf("handoff: %s: the prompt and command were printed on stdout", err)
		}
		paneArgv, multiplexer, ok := handoff.PaneArgv(paneEnvironment(), req.Dir, handoff.LabelFor(req.Agent), req.Argv)
		if !ok {
			reason := "no Rex or tmux session to open a pane in"
			printHandoffFallback(stdout, req, reason)
			return fmt.Errorf("handoff: %s: the prompt and command were printed on stdout", reason)
		}
		var errb bytes.Buffer
		cmd := exec.Command(paneArgv[0], paneArgv[1:]...)
		cmd.Dir = req.Dir
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			reason := paneArgv[0] + ": " + strings.TrimSpace(errb.String())
			printHandoffFallback(stdout, req, reason)
			return fmt.Errorf("handoff: %s: the prompt and command were printed on stdout", reason)
		}
		if err := stamp(); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "handed off to %s in a new pane (%s)\n", req.Agent, multiplexer)
		return nil
	}
	return errors.New("handoff: unknown where " + req.Where)
}

func paneEnvironment() handoff.Environment {
	return handoff.Environment{RexSession: os.Getenv("REX_SESSION"), RexBlock: os.Getenv("REX_BLOCK"), Tmux: os.Getenv("TMUX")}
}

func printHandoffFallback(w io.Writer, req *handoff.Request, reason string) {
	fmt.Fprintf(w, "clump: could not hand off to %s (%s)\n\n$ %s\n\nprompt:\n\n%s\n",
		req.Agent, reason, req.Command(), req.Prompt)
}
