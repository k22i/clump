package main

// The comments subcommand: the agent-facing review-comment interface.
//
//	clump comments [REV | A..B | A...B | --branch | --pr SPEC] [--json]
//	clump comments add --file PATH [--line N --side old|new] \
//	                   [--rev REV | --branch | --pr SPEC] TEXT
//	clump comments add --group ID [--rev …] TEXT
//	clump comments delete --id ID
//
// The target is what the one-shot review takes: a rev, a range, --branch,
// --pr, or a patch on stdin (for the list). add and delete name their
// target in flags (`--rev`) so the free text is unambiguous. No provider is
// ever called: comments come from the store, and group comments are listed
// with the saved grouping (the override, else the cached result).

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
)

type commentsOpts struct {
	repo, pr, rev, file, side, groupID, id string
	branch, jsonOut                        bool
	line                                   int
	args                                   []string // free positionals
}

func parseCommentsFlags(args []string) (commentsOpts, error) {
	var o commentsOpts
	fs := flag.NewFlagSet("clump comments", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.repo, "repo", "", "")
	fs.StringVar(&o.pr, "pr", "", "")
	fs.StringVar(&o.rev, "rev", "", "")
	fs.BoolVar(&o.branch, "branch", false, "")
	fs.BoolVar(&o.jsonOut, "json", false, "")
	fs.StringVar(&o.file, "file", "", "")
	fs.IntVar(&o.line, "line", 0, "")
	fs.StringVar(&o.side, "side", "new", "")
	fs.StringVar(&o.groupID, "group", "", "")
	fs.StringVar(&o.id, "id", "", "")
	fs.Usage = func() {}
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return o, usageError{err.Error()}
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		o.args = append(o.args, args...) // positionals never return to the flagset
		break
	}
	if o.side != comment.SideOld && o.side != comment.SideNew {
		return o, usageError{"--side must be old or new"}
	}
	if o.line < 0 {
		return o, usageError{"--line must be a line number"}
	}
	if o.branch && (o.pr != "" || o.rev != "") {
		return o, usageError{"--branch takes no --pr or --rev"}
	}
	if o.pr != "" && o.rev != "" {
		return o, usageError{"--pr and --rev are exclusive"}
	}
	if len(o.args) > 1 {
		return o, usageError{"want at most one revision or range"}
	}
	return o, nil
}

// target is the equivalent one-shot target for loadInput.
func (o commentsOpts) target(stdin *os.File) (oneShotOpts, error) {
	t := oneShotOpts{repo: o.repo, pr: o.pr, branch: o.branch, jsonOut: o.jsonOut}
	switch {
	case o.rev != "":
		t.args = []string{o.rev}
	case len(o.args) > 0:
		t.args = []string{o.args[0]}
	default:
		// stdin falls through when stdout is not the terminal
		_ = stdin
	}
	return t, nil
}

// runComments is the `clump comments` entry: args[0] may be list|add|delete.
func runComments(args []string, stdin *os.File, stdout, stderr io.Writer) error {
	sub := "list"
	if len(args) > 0 {
		switch args[0] {
		case "list":
			sub, args = "list", args[1:]
		case "add":
			sub, args = "add", args[1:]
		case "delete", "rm":
			sub, args = "delete", args[1:]
		}
	}
	o, err := parseCommentsFlags(args)
	if err != nil {
		return err
	}
	switch sub {
	case "add":
		text := strings.Join(o.args, " ")
		if o.groupID == "" && o.file == "" {
			return usageError{"comments add wants --file PATH or --group ID"}
		}
		if o.groupID != "" && (o.file != "" || o.line != 0) {
			return usageError{"--group takes no --file or --line"}
		}
		return addComment(o, strings.TrimSpace(text), stdin, stdout)
	case "delete":
		if o.id == "" {
			return usageError{"comments delete wants --id ID"}
		}
		return deleteComment(o, o.id, stdout)
	default:
		return listComments(o, stdin, stdout)
	}
}

// commentDiff loads the target's diff.
func commentDiff(ctx context.Context, o commentsOpts, stdin *os.File) (*gitdiff.Loaded, error) {
	t, err := o.target(stdin)
	if err != nil {
		return nil, err
	}
	l, _, err := loadInput(ctx, t, stdin)
	if err != nil {
		return nil, err
	}
	if len(l.Diff.Hunks) == 0 {
		return nil, errors.New("the diff is empty")
	}
	return l, nil
}

// savedGrouping is the grouping the comments list places group comments
// under: the user's saved edits, else the cached result. It never calls a
// provider; ok is false when there is none.
func savedGrouping(a *app, l *gitdiff.Loaded, providerName string) (group.Grouping, bool) {
	ov, ok, err := a.store.LoadOverride(l.Diff.Hash())
	if err != nil || ok {
		return ov.Grouping, ok
	}
	out, ok, err := a.eng.Lookup(l.Diff, providerName)
	if err != nil || !ok {
		return group.Grouping{}, false
	}
	return out.Result.Grouping, true
}

// addComment resolves the anchor and stores the comment.
func addComment(o commentsOpts, text string, stdin *os.File, stdout io.Writer) error {
	if text == "" {
		return usageError{"comments add wants a comment text"}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a, err := loadApp(false)
	if err != nil {
		return err
	}
	l, err := commentDiff(ctx, o, stdin)
	if err != nil {
		return err
	}
	var anchor *comment.Anchor
	if o.groupID != "" {
		g, ok := savedGrouping(a, l, a.cfg.Default)
		if !ok {
			return errors.New("no saved grouping for this diff; comment on a file or line instead")
		}
		found := false
		for _, grp := range g.Groups {
			if grp.ID == o.groupID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("no group %s in the saved grouping", o.groupID)
		}
		anchor = &comment.Anchor{Kind: comment.KindGroup, Key: comment.GroupKey(l.Diff.Hash(), o.groupID), GroupID: o.groupID}
	} else {
		anchor, err = comment.FindAnchor(l.Diff, o.file, o.side, o.line)
		if err != nil {
			return err
		}
	}
	c, err := a.store.AddComment(anchor.Key, text, time.Now())
	if err != nil {
		return err
	}
	placed := comment.Placed{Kind: anchor.Kind, GroupID: anchor.GroupID, Path: anchor.Path,
		Side: anchor.Side, Line: anchor.Line, Text: c.Text, ID: c.ID, At: c.At, Anchor: anchor.Key}
	if o.jsonOut {
		b, err := json.MarshalIndent(struct {
			Version int            `json:"version"`
			Comment comment.Placed `json:"comment"`
		}{Version: 1, Comment: placed}, "", "  ")
		if err != nil {
			return err
		}
		_, err = stdout.Write(append(b, '\n'))
		return err
	}
	fmt.Fprintf(stdout, "added comment %s: %s\n", c.ID, commentTargetLabel(placed))
	return nil
}

// deleteComment removes a comment by id; its anchor is found in the store.
func deleteComment(o commentsOpts, id string, stdout io.Writer) error {
	a, err := loadApp(false)
	if err != nil {
		return err
	}
	all, err := a.store.AllComments(time.Now())
	if err != nil {
		return err
	}
	for key, cs := range all {
		for _, c := range cs {
			if c.ID != id {
				continue
			}
			if err := a.store.DeleteComment(key, id); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "deleted comment %s\n", id)
			return nil
		}
	}
	return fmt.Errorf("no comment %s", id)
}

// listComments prints a diff's comments, numbered, or as JSON.
func listComments(o commentsOpts, stdin *os.File, stdout io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a, err := loadApp(false)
	if err != nil {
		return err
	}
	l, err := commentDiff(ctx, o, stdin)
	if err != nil {
		return err
	}
	var g group.Grouping
	if grp, ok := savedGrouping(a, l, a.cfg.Default); ok {
		g = grp
	}
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
		return err
	}
	placed := comment.Place(comment.Resolve(anchors, stored), l.Diff, g)
	if o.jsonOut {
		out, err := json.MarshalIndent(struct {
			Version  int              `json:"version"`
			Title    string           `json:"title"`
			Ref      string           `json:"ref"`
			DiffHash string           `json:"diff_hash"`
			Comments []comment.Placed `json:"comments"`
		}{Version: 1, Title: l.Title, Ref: l.Ref, DiffHash: l.Diff.Hash(), Comments: placed}, "", "  ")
		if err != nil {
			return err
		}
		_, err = stdout.Write(append(out, '\n'))
		return err
	}
	head := "Comments"
	if l.Title != "" {
		head += " on " + l.Title
		if l.Ref != "" && l.Ref != "stdin" {
			head += " (" + l.Ref + ")"
		}
	}
	fmt.Fprintln(stdout, head)
	if len(placed) == 0 {
		fmt.Fprintln(stdout, "(none)")
		return nil
	}
	for i, c := range placed {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, commentTargetLabel(c))
	}
	return nil
}

// commentTargetLabel names where a comment sits, like the report does.
func commentTargetLabel(c comment.Placed) string {
	var at string
	switch c.Kind {
	case comment.KindLine:
		at = fmt.Sprintf("`%s:%d` (%s side)", c.Path, c.Line, c.Side)
	case comment.KindFile:
		at = fmt.Sprintf("`%s` (file)", c.Path)
	case comment.KindGroup:
		at = "group " + c.GroupID
	default:
		at = "the review"
	}
	return at + " — " + c.Text
}
