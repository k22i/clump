package catalog

import (
	"fmt"
	"path"
	"strings"

	"github.com/k22i/clump/internal/gitdiff"
)

const DefaultCollapseMin = 6

type Options struct {
	CollapseMin int
}

type Unit struct {
	ID      string
	Hunks   []*gitdiff.Hunk
	Pattern string
	Files   int
}

func (u Unit) Collapsed() bool { return len(u.Hunks) > 1 }

func (u Unit) HunkIDs() []string {
	ids := make([]string, len(u.Hunks))
	for i, h := range u.Hunks {
		ids[i] = h.ID
	}
	return ids
}

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
		if skipped(h) {
			continue
		}
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
		if skipped(h) {
			continue
		}
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

func skipped(h *gitdiff.Hunk) bool { return h.File != nil && h.File.Generated != "" }

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

func Line(u Unit) string {
	h := u.Hunks[0]
	add, del := sums(u)
	if u.Collapsed() {
		return fmt.Sprintf("%s | %s | %s (%d files) | %d similar hunks, e.g. %s | +%d -%d",
			u.ID, h.File.Status, u.Pattern, u.Files, len(u.Hunks), h.Header, add, del)
	}
	return fmt.Sprintf("%s | %s | %s | %s | +%d -%d", u.ID, h.File.Status, h.File.DisplayPath(), h.Header, add, del)
}

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
