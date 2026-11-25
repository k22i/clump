// Package report renders a grouped diff as Markdown (for `y` and --plain, a
// PR-description-style summary) and as JSON (for --json, scripts and
// agents). Both reflect the effective grouping, edits included.
package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
)

// Doc is one grouped diff as the UI and the outputs see it.
type Doc struct {
	Title    string
	Ref      string
	Diff     *gitdiff.Diff
	Grouping group.Grouping
	Notices  []string
	Provider string
	DiffHash string
	Cached   bool
	Edited   bool
	Fallback bool
	// Snapshots are kept edit snapshots (from the override file).
	Snapshots []cache.Snapshot
}

// GroupStats returns the +/- totals, file count and hunk count of a group.
func GroupStats(d *gitdiff.Diff, g group.Group) (add, del, files int) {
	seen := map[int]bool{}
	for _, id := range g.Hunks {
		if h := d.Hunk(id); h != nil {
			add += h.Add
			del += h.Del
			seen[h.FileIdx] = true
		}
	}
	return add, del, len(seen)
}

// fileRows lists a group's files in diff order with their hunk counts.
func fileRows(d *gitdiff.Diff, g group.Group) []string {
	type row struct {
		n, add, del int
	}
	rows := map[int]*row{}
	var order []int
	for _, id := range g.Hunks {
		h := d.Hunk(id)
		if h == nil {
			continue
		}
		r, ok := rows[h.FileIdx]
		if !ok {
			r = &row{}
			rows[h.FileIdx] = r
			order = append(order, h.FileIdx)
		}
		r.n++
		r.add += h.Add
		r.del += h.Del
	}
	sortInts(order)
	var out []string
	for _, fi := range order {
		r := rows[fi]
		f := d.Files[fi]
		total := len(f.Hunks)
		part := ""
		if r.n < total {
			part = fmt.Sprintf(", %d of %d hunks", r.n, total)
		}
		out = append(out, fmt.Sprintf("- `%s` (+%d −%d%s)", f.DisplayPath(), r.add, r.del, part))
	}
	return out
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// Markdown renders doc as a Markdown summary suitable for a PR description.
func Markdown(doc Doc) string {
	var b strings.Builder
	d := doc.Diff
	add, del := d.Stats()
	title := doc.Title
	if title == "" {
		title = "Changes"
	}
	if doc.Ref != "" && doc.Ref != "stdin" {
		fmt.Fprintf(&b, "# %s (%s)\n\n", title, doc.Ref)
	} else {
		fmt.Fprintf(&b, "# %s\n\n", title)
	}
	meta := []string{
		plural(len(doc.Grouping.Groups), "group"), plural(len(d.Files), "file"),
		fmt.Sprintf("+%d −%d", add, del),
	}
	if doc.Provider != "" {
		meta = append(meta, "grouped by "+doc.Provider)
	}
	if doc.Edited {
		meta = append(meta, "edited")
	}
	b.WriteString(strings.Join(meta, " · "))
	b.WriteString("\n")
	for i, g := range doc.Grouping.Groups {
		ga, gd, _ := GroupStats(d, g)
		fmt.Fprintf(&b, "\n## %d. %s (+%d −%d)\n\n", i+1, g.Title, ga, gd)
		if desc := strings.TrimSpace(g.Description); desc != "" {
			b.WriteString(desc)
			b.WriteString("\n\n")
		}
		if len(g.Hunks) == 0 {
			b.WriteString("_(empty)_\n")
			continue
		}
		for _, r := range fileRows(d, g) {
			b.WriteString(r)
			b.WriteByte('\n')
		}
	}
	if len(doc.Notices) > 0 {
		b.WriteString("\n")
		for _, n := range doc.Notices {
			fmt.Fprintf(&b, "> %s\n", n)
		}
	}
	return b.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// JSONHunk is one hunk in the --json output.
type JSONHunk struct {
	ID      string         `json:"id"`
	File    string         `json:"file"`
	OldFile string         `json:"old_file,omitempty"`
	Status  gitdiff.Status `json:"status"`
	Header  string         `json:"header"`
	Add     int            `json:"add"`
	Del     int            `json:"del"`
	Pseudo  bool           `json:"pseudo,omitempty"`
}

// JSONGroup is one group in the --json output.
type JSONGroup struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Add         int        `json:"add"`
	Del         int        `json:"del"`
	Hunks       []JSONHunk `json:"hunks"`
}

// JSONDoc is the --json output.
type JSONDoc struct {
	Version   int         `json:"version"`
	Title     string      `json:"title"`
	Ref       string      `json:"ref"`
	DiffHash  string      `json:"diff_hash"`
	Provider  string      `json:"provider"`
	Cached    bool        `json:"cached"`
	Edited    bool        `json:"edited"`
	Fallback  bool        `json:"fallback"`
	Notices   []string    `json:"notices"`
	Files     int         `json:"files"`
	HunkCount int         `json:"hunk_count"`
	Add       int         `json:"add"`
	Del       int         `json:"del"`
	Groups    []JSONGroup `json:"groups"`
	Snapshots []JSONSnap  `json:"snapshots,omitempty"`
}

// JSONSnap is a kept snapshot in the --json output.
type JSONSnap struct {
	Name   string        `json:"name"`
	At     time.Time     `json:"at"`
	Groups []group.Group `json:"groups"`
}

// JSON renders doc as indented JSON ending in a newline.
func JSON(doc Doc) ([]byte, error) {
	d := doc.Diff
	add, del := d.Stats()
	out := JSONDoc{Version: 1, Title: doc.Title, Ref: doc.Ref, DiffHash: doc.DiffHash, Provider: doc.Provider,
		Cached: doc.Cached, Edited: doc.Edited, Fallback: doc.Fallback, Notices: doc.Notices,
		Files: len(d.Files), HunkCount: len(d.Hunks), Add: add, Del: del, Groups: []JSONGroup{}}
	if out.Notices == nil {
		out.Notices = []string{}
	}
	for _, g := range doc.Grouping.Groups {
		ga, gd, _ := GroupStats(d, g)
		jg := JSONGroup{ID: g.ID, Title: g.Title, Description: g.Description, Add: ga, Del: gd, Hunks: []JSONHunk{}}
		for _, id := range g.Hunks {
			h := d.Hunk(id)
			if h == nil {
				continue
			}
			jh := JSONHunk{ID: h.ID, File: h.File.Path(), Status: h.File.Status, Header: h.Header, Add: h.Add, Del: h.Del, Pseudo: h.Pseudo}
			if h.File.OldPath != "" && h.File.OldPath != h.File.Path() {
				jh.OldFile = h.File.OldPath
			}
			jg.Hunks = append(jg.Hunks, jh)
		}
		out.Groups = append(out.Groups, jg)
	}
	for _, s := range doc.Snapshots {
		out.Snapshots = append(out.Snapshots, JSONSnap{Name: s.Name, At: s.At, Groups: s.Grouping.Groups})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
