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

type Kind uint8

const (
	KindLine Kind = iota
	KindFile
	KindGroup
)

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

func (k Kind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

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

const (
	SideOld = "old"
	SideNew = "new"
)

type Comment struct {
	ID   string    `json:"id"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

type Anchor struct {
	Kind    Kind
	Key     string
	HunkID  string
	Path    string
	Side    string
	Line    int
	GroupID string
}

func LineKey(reviewKey, side string, line int) string {
	return hashFields("line", reviewKey, side, strconv.Itoa(line))
}

func FileKey(path string, reviewKeys []string) string {
	return hashFields("file", path, strings.Join(reviewKeys, "\x00"))
}

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

type Resolved struct {
	Anchor
	Comments []Comment
}

func Resolve(anchors []Anchor, comments map[string][]Comment) []Resolved {
	var out []Resolved
	for _, a := range anchors {
		if cs := comments[a.Key]; len(cs) > 0 {
			out = append(out, Resolved{Anchor: a, Comments: cs})
		}
	}
	return out
}

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
			continue
		}
		if !found && s == side && n == line {
			found = true
			return &Anchor{Kind: KindLine, Key: LineKey(rkey, s, n), HunkID: h.ID,
				Path: path, Side: s, Line: n}
		}
	}
	return nil
}

func LineTarget(isDel bool, oldLine, newLine int) (side string, n int) {
	if isDel || newLine == 0 {
		return SideOld, oldLine
	}
	return SideNew, newLine
}
