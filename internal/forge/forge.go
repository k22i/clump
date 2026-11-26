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

type Event string

const (
	EventComment       Event = "COMMENT"
	EventApprove       Event = "APPROVE"
	EventRequestChange Event = "REQUEST_CHANGES"
)

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

type Inline struct {
	Path        string `json:"path"`
	SubjectType string `json:"subject_type,omitempty"`
	Side        string `json:"side,omitempty"`
	Line        int    `json:"line,omitempty"`
	Body        string `json:"body"`
}

type Counts struct {
	Line, File, Group int
}

func (c Counts) Total() int { return c.Line + c.File + c.Group }

type Review struct {
	Event    Event
	Body     string
	Comments []Inline
	Counts   Counts
}

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

type Result struct {
	URL    string
	Number int
}

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
		return Result{}, fmt.Errorf("review posted but not reported: %w", err)
	}
	return Result{URL: res.HTMLURL, Number: res.ID}, nil
}

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
