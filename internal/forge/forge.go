// Package forge posts a review to a git hosting service. clump's comments
// attach to a diff by content (internal/comment); a forge turns them into a
// hosted review: line and file comments become inline review comments, group
// comments — a clump concept no forge has — become the review body, and the
// whole review is posted in one request.
//
// GitHub is the forge: everything goes through the gh CLI, so clump stores
// no tokens and writes as whatever account gh is logged into, exactly like
// --pr reads. The REST call is `gh api --input -` against
// repos/{owner}/{repo}/pulls/N/reviews with event COMMENT, APPROVE or
// REQUEST_CHANGES (a pending review needs GraphQL and is out of scope).
package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/gitdiff"
)

// Event is a GitHub pull request review event.
type Event string

// The review events. Leave and Draft do not exist here: Draft needs a
// pending review (GraphQL), and the REST call alone cannot make one.
const (
	EventComment       Event = "COMMENT"
	EventApprove       Event = "APPROVE"
	EventRequestChange Event = "REQUEST_CHANGES"
)

// ParseEvent parses an --event / picker value.
func ParseEvent(s string) (Event, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "comment":
		return EventComment, nil
	case "approve":
		return EventApprove, nil
	case "request-changes", "request_changes", "request changes":
		return EventRequestChange, nil
	default:
		return "", fmt.Errorf("unknown review event %q: use comment, approve or request-changes", s)
	}
}

// String is the lowercase name the CLI and TUI show.
func (e Event) String() string {
	switch e {
	case EventApprove:
		return "approve"
	case EventRequestChange:
		return "request changes"
	default:
		return "comment"
	}
}

// Inline is one inline review comment as the GitHub API takes it: a file
// (subject_type file), or a file and a side and a line (subject_type line).
// Line comments draw on the diff at path:side:line; file comments attach to
// the file header.
type Inline struct {
	Path        string `json:"path"`
	SubjectType string `json:"subject_type,omitempty"` // "" (line) or "file"
	Side        string `json:"side,omitempty"`         // LEFT or RIGHT
	Line        int    `json:"line,omitempty"`
	Body        string `json:"body"`
}

// Kind counts the comments of the review by what they attach to.
type Counts struct {
	Line, File, Group int
}

// Total is every comment, inline and in the body.
func (c Counts) Total() int { return c.Line + c.File + c.Group }

// Review is the review to post: an event, a body and inline comments.
type Review struct {
	Event    Event
	Body     string
	Comments []Inline
	Counts   Counts
}

// Build turns the diff's placed comments into a review. Line comments become
// inline comments on their side and line (LEFT is the old path of a rename,
// as the PR base shows it), file comments become file-level comments, and
// group comments — a clump-only concept — gather into the body after base.
// groupTitle may be nil; then group IDs stand in for titles.
func Build(event Event, base string, placed []comment.Placed, d *gitdiff.Diff, groupTitle func(id string) string) Review {
	r := Review{Event: event, Body: strings.TrimSpace(base)}
	path, old := filePaths(d)
	var groups []string
	for _, p := range placed {
		switch p.Kind {
		case comment.KindLine:
			in := Inline{Body: p.Text}
			if p.Side == comment.SideNew {
				in.Path, in.Side = path[p.Path], "RIGHT"
			} else {
				in.Path, in.Side = old[p.Path], "LEFT"
			}
			in.Line = p.Line
			r.Comments = append(r.Comments, in)
			r.Counts.Line++
		case comment.KindFile:
			r.Comments = append(r.Comments, Inline{Path: path[p.Path], SubjectType: "file", Body: p.Text})
			r.Counts.File++
		case comment.KindGroup:
			text := p.Text
			if groupTitle != nil {
				if t := groupTitle(p.GroupID); t != "" {
					text = t + ": " + text
				}
			}
			groups = append(groups, text)
			r.Counts.Group++
		}
	}
	if len(groups) > 0 {
		section := "## Group comments\n"
		for i, g := range groups {
			section += strconv.Itoa(i+1) + ". " + g + "\n"
		}
		if r.Body != "" {
			r.Body += "\n\n"
		}
		r.Body += section
	}
	r.Body = strings.TrimSpace(r.Body)
	return r
}

// filePaths indexes the diff's files by their display and current paths (both
// name a file in the PR diff UI) and by their old path (the location of the
// base blob for comments on the LEFT side).
func filePaths(d *gitdiff.Diff) (path, old map[string]string) {
	path, old = map[string]string{}, map[string]string{}
	if d == nil {
		return path, old
	}
	for _, f := range d.Files {
		path[f.DisplayPath()] = f.Path()
		path[f.Path()] = f.Path()
		if f.OldPath != "" {
			old[f.DisplayPath()] = f.OldPath
			old[f.Path()] = f.OldPath
		} else {
			old[f.DisplayPath()] = f.Path()
			old[f.Path()] = f.Path()
		}
	}
	return path, old
}

// Payload is the JSON body of the API call.
func (r Review) Payload() ([]byte, error) {
	return json.Marshal(reviewJSON{
		Event:    string(r.Event),
		Body:     r.Body,
		Comments: r.Comments,
	})
}

type reviewJSON struct {
	Event    string   `json:"event"`
	Body     string   `json:"body,omitempty"`
	Comments []Inline `json:"comments,omitempty"`
}

// Result reports the posted review.
type Result struct {
	URL    string // the review's html_url, "" when the API gave none
	Number int    // the review id
}

// Submit posts r as one review of the pull request named by spec (the same
// --pr forms LoadPR reads), through the gh CLI in repo ("" is the current
// directory). The whole review — every inline comment — is one request.
func Submit(ctx context.Context, repo, spec string, r Review) (Result, error) {
	p, err := gitdiff.ParsePRSpec(spec)
	if err != nil {
		return Result{}, err
	}
	endpoint := "repos/{owner}/{repo}/pulls/" + p.Number + "/reviews"
	if p.Owner != "" {
		endpoint = "repos/" + p.Owner + "/" + p.Repo + "/pulls/" + p.Number + "/reviews"
	}
	body, err := r.Payload()
	if err != nil {
		return Result{}, err
	}
	out, err := gitdiff.Gh(ctx, repo, string(body), "api", "--method", "POST", "--input", "-", endpoint)
	if err != nil {
		return Result{}, fmt.Errorf("submit review: %w", err)
	}
	var res struct {
		HTMLURL string `json:"html_url"`
		ID      int    `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		// The review is posted; only the report of it failed to parse.
		return Result{}, fmt.Errorf("review posted but not reported: %w", err)
	}
	return Result{URL: res.HTMLURL, Number: res.ID}, nil
}

// Summary is one human line about what the review contains, for confirm
// prompts and reports.
func (r Review) Summary() string {
	var parts []string
	if r.Counts.Line > 0 {
		parts = append(parts, plural(r.Counts.Line, "line comment"))
	}
	if r.Counts.File > 0 {
		parts = append(parts, plural(r.Counts.File, "file comment"))
	}
	if r.Counts.Group > 0 {
		parts = append(parts, "and "+strconv.Itoa(r.Counts.Group)+" group comment(s) in the body")
	}
	if len(parts) == 0 {
		if r.Body != "" {
			return "a review body, no comments"
		}
		return "no comments"
	}
	return strings.Join(parts, ", ")
}

func plural(n int, word string) string {
	if n == 1 {
		return strconv.Itoa(n) + " " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
