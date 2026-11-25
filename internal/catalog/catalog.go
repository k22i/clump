// Package catalog renders the hunk catalog the model groups over: one line
// per unit (`id | status | path | @@ header | +a -d`) and an annotated block
// per unit (`### hN path`). Runs of near-identical hunks across many files
// (translations, generated docs) collapse into one unit "cN" that the model
// assigns as a whole.
package catalog

import (
	"fmt"
	"path"
	"strings"

	"github.com/k22i/clump/internal/gitdiff"
)

// DefaultCollapseMin is the smallest run of near-identical hunks that
// collapses into one unit.
const DefaultCollapseMin = 6

// Options configures Units.
type Options struct {
	// CollapseMin: 0 means DefaultCollapseMin; < 0 disables collapsing.
	CollapseMin int
}

// Unit is one catalog entry: a single hunk ("h3") or a collapsed run ("c1").
type Unit struct {
	ID string
	// Hunks are the members in diff order. A single-hunk unit has one.
	Hunks []*gitdiff.Hunk
	// Pattern is the shared path pattern of a collapsed unit, e.g.
	// "docs/keybindings/*.md"; "" for a single hunk.
	Pattern string
	// Files is the number of distinct files in the unit.
	Files int
}

// Collapsed reports whether u stands for more than one hunk.
func (u Unit) Collapsed() bool { return len(u.Hunks) > 1 }

// HunkIDs returns the member hunk IDs in diff order.
func (u Unit) HunkIDs() []string {
	ids := make([]string, len(u.Hunks))
	for i, h := range u.Hunks {
		ids[i] = h.ID
	}
	return ids
}

// Units splits d into catalog units, in the diff order of each unit's first
// member. Two non-pseudo hunks are near-identical when their files share the
// directory, the extension and the status, and the hunks have the same +a
// and -d counts. A set of near-identical hunks collapses when it has at least
// CollapseMin members from at least CollapseMin distinct files. Collapsed
// units are numbered c1, c2, ... in order.
func Units(d *gitdiff.Diff, o Options) []Unit {
	min := o.CollapseMin
	if min == 0 {
		min = DefaultCollapseMin
	}
	type key struct {
		dir, ext string
		status   gitdiff.Status
		add, del int
	}
	sets := map[key][]*gitdiff.Hunk{}
	keyOf := func(h *gitdiff.Hunk) (key, bool) {
		if h.Pseudo || h.File == nil || min < 0 {
			return key{}, false
		}
		p := h.File.Path()
		return key{path.Dir(p), path.Ext(p), h.File.Status, h.Add, h.Del}, true
	}
	for _, h := range d.Hunks {
		if k, ok := keyOf(h); ok {
			sets[k] = append(sets[k], h)
		}
	}
	collapse := map[key]bool{}
	for k, hs := range sets {
		files := map[int]bool{}
		for _, h := range hs {
			files[h.FileIdx] = true
		}
		if len(hs) >= min && len(files) >= min {
			collapse[k] = true
		}
	}
	var out []Unit
	emitted := map[key]bool{}
	c := 0
	for _, h := range d.Hunks {
		k, ok := keyOf(h)
		if ok && collapse[k] {
			if emitted[k] {
				continue
			}
			emitted[k] = true
			c++
			hs := sets[k]
			files := map[int]bool{}
			for _, m := range hs {
				files[m.FileIdx] = true
			}
			pat := "*" + k.ext
			if k.dir != "." {
				pat = k.dir + "/" + pat
			}
			out = append(out, Unit{ID: fmt.Sprintf("c%d", c), Hunks: hs, Pattern: pat, Files: len(files)})
			continue
		}
		out = append(out, Unit{ID: h.ID, Hunks: []*gitdiff.Hunk{h}, Files: 1})
	}
	return out
}

// Aliases maps each collapsed unit ID to its member hunk IDs (for
// group.ValidateOpts.Aliases). Single-hunk units are not included.
func Aliases(units []Unit) map[string][]string {
	m := map[string][]string{}
	for _, u := range units {
		if u.Collapsed() {
			m[u.ID] = u.HunkIDs()
		}
	}
	return m
}

func sums(u Unit) (add, del int) {
	for _, h := range u.Hunks {
		add += h.Add
		del += h.Del
	}
	return add, del
}

// Line renders a unit's catalog line, never truncated:
//
//	h3 | M | src/a.go | @@ -1,4 +1,5 @@ func f() { | +2 -1
//	h4 | R | old.txt -> new.txt | (rename) | +0 -0
//	c1 | M | docs/keybindings/*.md (45 files) | 45 similar hunks, e.g. @@ -10,3 +10,4 @@ | +45 -45
//
// The +a -d of a collapsed unit are the sums over its members.
func Line(u Unit) string {
	h := u.Hunks[0]
	add, del := sums(u)
	if u.Collapsed() {
		return fmt.Sprintf("%s | %s | %s (%d files) | %d similar hunks, e.g. %s | +%d -%d",
			u.ID, h.File.Status, u.Pattern, u.Files, len(u.Hunks), h.Header, add, del)
	}
	return fmt.Sprintf("%s | %s | %s | %s | +%d -%d", u.ID, h.File.Status, h.File.DisplayPath(), h.Header, add, del)
}

// Block renders a unit's annotated hunk block with at most capLines body
// lines (capLines < 0 means no cap). A cut body ends with
// "[... N more lines]". A collapsed unit shows its first member as a sample:
//
//	### c1 docs/keybindings/*.md (45 similar hunks; sample: docs/keybindings/Keybindings_en.md)
//
// It returns the text (ending in "\n") and the number of body lines dropped.
func Block(u Unit, capLines int) (text string, dropped int) {
	h := u.Hunks[0]
	var b strings.Builder
	if u.Collapsed() {
		fmt.Fprintf(&b, "### %s %s (%d similar hunks; sample: %s)\n", u.ID, u.Pattern, len(u.Hunks), h.File.DisplayPath())
	} else {
		fmt.Fprintf(&b, "### %s %s\n", u.ID, h.File.DisplayPath())
	}
	b.WriteString(h.Header)
	b.WriteByte('\n')
	body := h.Lines
	if capLines >= 0 && len(body) > capLines {
		dropped = len(body) - capLines
		body = body[:capLines]
	}
	for _, l := range body {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if dropped > 0 {
		fmt.Fprintf(&b, "[... %d more lines]\n", dropped)
	}
	return b.String(), dropped
}

// FileBlock renders one file for a summary prompt: a header line
// "=== file <DisplayPath> (<status>, +a -d, N hunks)" followed by the Block
// of each of its hunks with capLines.
func FileBlock(d *gitdiff.Diff, fileIdx int, capLines int) (text string, dropped int) {
	f := d.Files[fileIdx]
	add, del := 0, 0
	for _, h := range f.Hunks {
		add += h.Add
		del += h.Del
	}
	var b strings.Builder
	fmt.Fprintf(&b, "=== file %s (%s, +%d -%d, %d hunks)\n", f.DisplayPath(), f.Status, add, del, len(f.Hunks))
	for _, h := range f.Hunks {
		t, dr := Block(Unit{ID: h.ID, Hunks: []*gitdiff.Hunk{h}, Files: 1}, capLines)
		b.WriteString(t)
		dropped += dr
	}
	return b.String(), dropped
}
