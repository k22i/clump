// Package comment is clump's review-comment model: what a comment can
// attach to (an anchor), how an anchor is keyed for storage, and how
// stored comments are resolved back to human-readable targets for the
// report outputs and the comments CLI.
//
// Anchors follow the reviewed-mark philosophy: they are keyed by content.
// A line comment is keyed by its hunk's ReviewKey, side and line number, so
// it follows the change across regrouping, edits, provider switches and the
// commit-versus-range view, and disappears when the hunk's text changes. A
// file comment is keyed by the file's path and the content of its whole set
// of hunks. A group comment is keyed by the diff and the group's ID, so it
// stays while that grouping (cached or the user's saved edits) is the one
// on screen; a regroup that renumbers groups drops it from view.
package comment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
)

// Kind says what a comment is attached to.
type Kind uint8

// Comment kinds.
const (
	KindLine  Kind = iota // one diff line (a side and a line number)
	KindFile              // a file's whole change in this diff
	KindGroup             // a group of the diff's grouping
)

// String renders the kind for display and JSON.
func (k Kind) String() string {
	switch k {
	case KindLine:
		return "line"
	case KindFile:
		return "file"
	default:
		return "group"
	}
}

// MarshalText implements encoding.TextMarshaler.
func (k Kind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (k *Kind) UnmarshalText(b []byte) error {
	switch string(b) {
	case "line":
		*k = KindLine
	case "file":
		*k = KindFile
	case "group":
		*k = KindGroup
	default:
		return fmt.Errorf("unknown comment kind %q", string(b))
	}
	return nil
}

// Sides a line comment can sit on.
const (
	SideOld = "old"
	SideNew = "new"
)

// Comment is one stored review comment.
type Comment struct {
	ID   string    `json:"id"`   // short hash identifying it within its anchor
	Text string    `json:"text"` // the note itself, one line
	At   time.Time `json:"at"`   // when it was written
}

// Anchor is one place a comment can attach to, carrying the coordinates a
// display needs: HunkID/Path/Side/Line for line anchors, Path for file
// anchors, GroupID for group anchors.
type Anchor struct {
	Kind    Kind
	Key     string
	HunkID  string
	Path    string
	Side    string
	Line    int
	GroupID string
}

// LineKey is the storage key of a comment on line (side SideNew or SideOld)
// of the hunk with the given ReviewKey.
func LineKey(reviewKey, side string, line int) string {
	return hashFields("line", reviewKey, side, strconv.Itoa(line))
}

// FileKey is the storage key of a comment on the file path's whole change:
// its path plus its hunks' ReviewKeys in file order, so it survives
// regrouping and resets when the file's changes do.
func FileKey(path string, reviewKeys []string) string {
	return hashFields("file", path, strings.Join(reviewKeys, "\x00"))
}

// GroupKey is the storage key of a comment on group groupID of the diff
// with the given diff hash. Overrides and cached groupings reuse the same
// IDs, so such a comment comes back with its group; a regroup that
// renumbers groups leaves it stored but out of view.
func GroupKey(diffHash, groupID string) string {
	return hashFields("group", diffHash, groupID)
}

func hashFields(fields ...string) string {
	h := sha256.New()
	for i, f := range fields {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Anchors lists every anchor of a diff: each body line of each hunk in diff
// order, then each file, then each group ID given (in that order). Group
// anchors exist only while a grouping names the group.
func Anchors(d *gitdiff.Diff, diffHash string, groupIDs []string) []Anchor {
	if d == nil {
		return nil
	}
	var out []Anchor
	for _, h := range d.Hunks {
		out = append(out, lineAnchors(h)...)
	}
	for _, f := range d.Files {
		rkeys := make([]string, 0, len(f.Hunks))
		for _, h := range f.Hunks {
			rkeys = append(rkeys, h.ReviewKey())
		}
		path := f.DisplayPath()
		out = append(out, Anchor{Kind: KindFile, Key: FileKey(path, rkeys), Path: path})
	}
	for _, id := range groupIDs {
		out = append(out, Anchor{Kind: KindGroup, Key: GroupKey(diffHash, id), GroupID: id})
	}
	return out
}

// lineAnchors walks h's body and names its commentable lines.
func lineAnchors(h *gitdiff.Hunk) []Anchor {
	if h.Pseudo || len(h.Lines) == 0 {
		return nil
	}
	rkey := h.ReviewKey()
	path := ""
	if h.File != nil {
		path = h.File.DisplayPath()
	}
	oldNo, newNo := h.OldStart, h.NewStart
	out := make([]Anchor, 0, len(h.Lines))
	for _, raw := range h.Lines {
		if raw == "" {
			continue
		}
		switch raw[0] {
		case '-':
			if oldNo > 0 {
				out = append(out, Anchor{Kind: KindLine, Key: LineKey(rkey, SideOld, oldNo), HunkID: h.ID,
					Path: path, Side: SideOld, Line: oldNo})
			}
			oldNo++
		case '+':
			if newNo > 0 {
				out = append(out, Anchor{Kind: KindLine, Key: LineKey(rkey, SideNew, newNo), HunkID: h.ID,
					Path: path, Side: SideNew, Line: newNo})
			}
			newNo++
		case ' ':
			side, n := SideNew, newNo
			if newNo == 0 {
				side, n = SideOld, oldNo
			}
			if n > 0 {
				out = append(out, Anchor{Kind: KindLine, Key: LineKey(rkey, side, n), HunkID: h.ID,
					Path: path, Side: side, Line: n})
			}
			oldNo++
			newNo++
		}
	}
	return out
}

// Resolved is an anchor with the comments stored on it.
type Resolved struct {
	Anchor
	Comments []Comment
}

// Resolve zips anchors with the comments loaded for their keys; anchors
// without comments are dropped. Stable in anchor order.
func Resolve(anchors []Anchor, comments map[string][]Comment) []Resolved {
	var out []Resolved
	for _, a := range anchors {
		if cs := comments[a.Key]; len(cs) > 0 {
			out = append(out, Resolved{Anchor: a, Comments: cs})
		}
	}
	return out
}

// Placed is a display-ready comment: where it shows (group, path, side,
// line) and what it says.
type Placed struct {
	Kind    Kind      `json:"kind"`
	GroupID string    `json:"group,omitempty"`
	Path    string    `json:"path,omitempty"`
	Side    string    `json:"side,omitempty"`
	Line    int       `json:"line,omitempty"`
	Text    string    `json:"text"`
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Anchor  string    `json:"anchor"`
}

// Place maps resolved comments onto the groups of g for the given diff. A
// line comment goes to the group holding its hunk, a file comment to the
// group of the file's earliest hunk (a file block can straddle groups), a
// group comment to its own group. A target that is in no group keeps
// GroupID "".
func Place(res []Resolved, d *gitdiff.Diff, g group.Grouping) []Placed {
	hunkGroup := make(map[string]string, len(g.Groups))
	for _, grp := range g.Groups {
		for _, id := range grp.Hunks {
			hunkGroup[id] = grp.ID
		}
	}
	fileGroup := make(map[string]string)
	for _, f := range d.Files {
		for _, h := range f.Hunks {
			if id, ok := hunkGroup[h.ID]; ok {
				fileGroup[f.DisplayPath()] = id
				break
			}
		}
	}
	out := make([]Placed, 0, len(res))
	for _, r := range res {
		for _, c := range r.Comments {
			p := Placed{Kind: r.Kind, Path: r.Path, Side: r.Side, Line: r.Line, GroupID: r.GroupID,
				Text: c.Text, ID: c.ID, At: c.At, Anchor: r.Key}
			if r.Kind == KindLine {
				p.GroupID = hunkGroup[r.HunkID]
			} else if r.Kind == KindFile {
				p.GroupID = fileGroup[r.Path]
			}
			out = append(out, p)
		}
	}
	return out
}

// FindAnchor resolves a comment target in d for the comments CLI: path
// alone is a file comment; path with side and a line number is a comment on
// that line. The file may be named by either of its paths or its display
// form ("old -> new").
func FindAnchor(d *gitdiff.Diff, path, side string, line int) (*Anchor, error) {
	if d == nil {
		return nil, fmt.Errorf("no diff to comment on")
	}
	for _, f := range d.Files {
		if f.DisplayPath() != path && f.Path() != path &&
			!(f.OldPath != "" && f.OldPath == path) && !(f.NewPath != "" && f.NewPath == path) {
			continue
		}
		rkeys := make([]string, 0, len(f.Hunks))
		for _, h := range f.Hunks {
			rkeys = append(rkeys, h.ReviewKey())
		}
		display := f.DisplayPath()
		if line == 0 {
			return &Anchor{Kind: KindFile, Key: FileKey(display, rkeys), Path: display}, nil
		}
		if side != SideOld && side != SideNew {
			side = SideNew
		}
		for _, h := range f.Hunks {
			if a := hunkLineAnchor(h, side, line); a != nil {
				return a, nil
			}
		}
		return nil, fmt.Errorf("%s:%d (%s side) is not in the diff", path, line, side)
	}
	return nil, fmt.Errorf("file %q is not in the diff", path)
}

// hunkLineAnchor returns the line anchor when line names one of h's body
// lines on side. Only removed lines anchor on the old side; a context line
// anchors on the new side (its old side when the diff has none, as in a
// deleted file), exactly as in the anchors the TUI reads.
func hunkLineAnchor(h *gitdiff.Hunk, side string, line int) *Anchor {
	if h.Pseudo || len(h.Lines) == 0 {
		return nil
	}
	oldNo, newNo := h.OldStart, h.NewStart
	rkey, path := h.ReviewKey(), ""
	if h.File != nil {
		path = h.File.DisplayPath()
	}
	found := false
	for _, raw := range h.Lines {
		if raw == "" {
			continue
		}
		s, n := "", 0
		switch raw[0] {
		case '-':
			s, n = SideOld, oldNo
			oldNo++
		case '+':
			s, n = SideNew, newNo
			newNo++
		case ' ':
			s, n = SideNew, newNo
			if newNo == 0 {
				s, n = SideOld, oldNo
			}
			oldNo++
			newNo++
		default:
			continue // "\ No newline…"
		}
		if !found && s == side && n == line {
			found = true
			return &Anchor{Kind: KindLine, Key: LineKey(rkey, s, n), HunkID: h.ID,
				Path: path, Side: s, Line: n}
		}
	}
	return nil
}

// LineTarget picks a line anchor's side and number for a rendered diff line:
// a removed line is anchored by its old number, every other line by its new
// number (its old number when there is none, as in a deleted file).
func LineTarget(isDel bool, oldLine, newLine int) (side string, n int) {
	if isDel || newLine == 0 {
		return SideOld, oldLine
	}
	return SideNew, newLine
}
