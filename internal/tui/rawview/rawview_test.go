package rawview

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/tui/view"
	"github.com/k22i/clump/internal/tui/view/viewtest"
)

// stub renders a hunk's header and body lines with two leading spaces, cut to
// w cells when w > 0. It stands in for diffview.RenderHunk in these tests.
func stub(h *gitdiff.Hunk, w int) []view.Line {
	lines := make([]view.Line, 0, len(h.Lines)+1)
	lines = append(lines, view.Line{
		Text:    cut("  "+h.Header, w),
		Plain:   cut("  "+h.Header, w),
		Kind:    view.KindHunk,
		HunkID:  h.ID,
		FileIdx: h.FileIdx,
		OldLine: h.OldStart,
		NewLine: h.NewStart,
	})
	for _, l := range h.Lines {
		text := cut("  "+l, w)
		kind := view.KindContext
		switch {
		case strings.HasPrefix(l, "+"):
			kind = view.KindAdd
		case strings.HasPrefix(l, "-"):
			kind = view.KindDel
		case strings.HasPrefix(l, "\\"):
			kind = view.KindNoNewline
		}
		lines = append(lines, view.Line{
			Text:    text,
			Plain:   text,
			Kind:    kind,
			HunkID:  h.ID,
			FileIdx: h.FileIdx,
		})
	}
	return lines
}

// cut is view.Truncate with the stub's "no limit when w <= 0" rule.
func cut(s string, w int) string {
	if w <= 0 {
		return s
	}
	return view.Truncate(s, w)
}

// rangeGrouping is the grouping used by most tests: two groups interleaved
// over the four hunks of lazygit/range-selection.txt.
func rangeGrouping() group.Grouping {
	return group.Grouping{Groups: []group.Group{
		{ID: "g1", Title: "Rename docs", Hunks: []string{"h1", "h3"}},
		{ID: "g2", Title: "Assets", Hunks: []string{"h2", "h4"}},
	}}
}

func TestGoldenRawRange(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/range-selection.txt")
	got := RenderAll(d, rangeGrouping(), view.PlainTheme(), 70, stub)
	viewtest.Golden(t, "rv-range", viewtest.JoinPlain(got))

	// The hunks alternate g1/g2/g1/g2, so every hunk starts a new boundary.
	var boundaries, labels []string
	for _, l := range got {
		switch l.Kind {
		case view.KindBoundary:
			boundaries = append(boundaries, l.Plain)
		case view.KindHunk:
			labels = append(labels, l.Plain[:LabelWidth])
		}
	}
	wantLabels := []string{"[ 1] ", "[ 2] ", "[ 1] ", "[ 2] "}
	if strings.Join(labels, "|") != strings.Join(wantLabels, "|") {
		t.Fatalf("hunk labels = %q, want %q", labels, wantLabels)
	}
	wantPrefixes := []string{"┄┄ 1. Rename docs ", "┄┄ 2. Assets ", "┄┄ 1. Rename docs ", "┄┄ 2. Assets "}
	if len(boundaries) != len(wantPrefixes) {
		t.Fatalf("got %d boundary lines, want %d: %q", len(boundaries), len(wantPrefixes), boundaries)
	}
	for i, b := range boundaries {
		if !strings.HasPrefix(b, wantPrefixes[i]) {
			t.Errorf("boundary %d = %q, want prefix %q", i, b, wantPrefixes[i])
		}
	}
}

func TestGoldenRawSameGroup(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/commit-67a7d22.txt")
	g := group.Grouping{Groups: []group.Group{
		{ID: "g1", Title: "Everything", Hunks: []string{"h1", "h2"}},
	}}
	got := RenderAll(d, g, view.PlainTheme(), 60, stub)
	viewtest.Golden(t, "rv-onegroup", viewtest.JoinPlain(got))

	n := 0
	for _, l := range got {
		if l.Kind == view.KindBoundary {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("got %d boundary lines for one group, want 1", n)
	}
}

func TestRawNoGroup(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/commit-67a7d22.txt")
	g := group.Grouping{Groups: []group.Group{
		{ID: "g1", Title: "Readme only", Hunks: []string{"h1"}},
	}}
	got := RenderAll(d, g, view.PlainTheme(), 60, stub)

	var h2Label, h2Boundary string
	for _, l := range got {
		if l.Kind == view.KindBoundary && strings.Contains(l.Plain, noGroupTitle) {
			h2Boundary = l.Plain
		}
		if l.Kind == view.KindHunk && l.HunkID == "h2" {
			h2Label = l.Plain[:LabelWidth]
			if h2Label != "[ ?] " {
				t.Fatalf("h2 label = %q, want %q", h2Label, "[ ?] ")
			}
		}
	}
	if h2Label != "[ ?] " {
		t.Fatalf("no h2 hunk line found in %q", viewtest.JoinPlain(got))
	}
	if !strings.HasPrefix(h2Boundary, "┄┄ ?. (no group) ") {
		t.Fatalf("h2 boundary = %q, want prefix %q", h2Boundary, "┄┄ ?. (no group) ")
	}
	// h1 keeps its own label and boundary.
	for _, l := range got {
		if l.Kind == view.KindHunk && l.HunkID == "h1" && l.Plain[:LabelWidth] != "[ 1] " {
			t.Fatalf("h1 label = %q, want %q", l.Plain[:LabelWidth], "[ 1] ")
		}
	}
}

func TestRawFields(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/range-selection.txt")
	var widths []int
	rec := func(h *gitdiff.Hunk, w int) []view.Line {
		widths = append(widths, w)
		return stub(h, w)
	}
	const width = 70
	got := RenderAll(d, rangeGrouping(), view.PlainTheme(), width, rec)

	for _, w := range widths {
		if w != width-LabelWidth {
			t.Fatalf("hunk renderer called with width %d, want %d", w, width-LabelWidth)
		}
	}
	if len(widths) != len(d.Hunks) {
		t.Fatalf("hunk renderer called %d times, want %d", len(widths), len(d.Hunks))
	}
	for i, l := range got {
		if c := lipgloss.Width(l.Plain); c > width {
			t.Errorf("line %d is %d cells wide, want at most %d: %q", i, c, width, l.Plain)
		}
		if l.Text != l.Plain {
			t.Errorf("line %d Text != Plain under the plain theme: %q vs %q", i, l.Text, l.Plain)
		}
		switch l.Kind {
		case view.KindBoundary, view.KindBlank:
			if l.FileIdx != -1 {
				t.Errorf("line %d (%v) FileIdx = %d, want -1", i, l.Kind, l.FileIdx)
			}
			if l.HunkID != "" {
				t.Errorf("line %d (%v) HunkID = %q, want empty", i, l.Kind, l.HunkID)
			}
		case view.KindHunk, view.KindAdd, view.KindDel, view.KindContext, view.KindNoNewline:
			if l.HunkID == "" {
				t.Errorf("line %d lost its HunkID: %q", i, l.Plain)
			}
			if l.FileIdx < 0 {
				t.Errorf("line %d has FileIdx %d, want the hunk's file", i, l.FileIdx)
			}
		}
	}
	// Hunk lines keep the file index the renderer reported.
	seen := map[string]int{}
	for _, l := range got {
		if l.HunkID != "" {
			seen[l.HunkID] = l.FileIdx
		}
	}
	for _, h := range d.Hunks {
		if seen[h.ID] != h.FileIdx {
			t.Errorf("hunk %s FileIdx = %d, want %d", h.ID, seen[h.ID], h.FileIdx)
		}
	}
}

func TestRawColoured(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/range-selection.txt")
	plain := RenderAll(d, rangeGrouping(), view.PlainTheme(), 70, stub)
	coloured := RenderAll(d, rangeGrouping(), view.NewTheme(true), 70, stub)
	if len(plain) != len(coloured) {
		t.Fatalf("coloured render has %d lines, plain has %d", len(coloured), len(plain))
	}
	for i := range plain {
		if got := gitdiff.StripSGR(coloured[i].Text); got != plain[i].Plain {
			t.Errorf("line %d: StripSGR(Text) = %q, want %q", i, got, plain[i].Plain)
		}
		if coloured[i].Kind != plain[i].Kind {
			t.Errorf("line %d: coloured Kind %v, plain Kind %v", i, coloured[i].Kind, plain[i].Kind)
		}
	}
	// The label really is styled: at least one line carries an escape.
	styled := false
	for _, l := range coloured {
		if l.Kind == view.KindHunk && strings.Contains(l.Text, "\x1b[") {
			styled = true
		}
	}
	if !styled {
		t.Error("no coloured hunk label found; GroupLabel styling is missing")
	}
}

func TestRawEmpty(t *testing.T) {
	cases := []struct {
		name string
		diff *gitdiff.Diff
	}{
		{"no files", &gitdiff.Diff{}},
		{"files without hunks", &gitdiff.Diff{Files: []*gitdiff.File{{NewPath: "a", Status: gitdiff.Modified}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RenderAll(c.diff, group.Grouping{}, view.PlainTheme(), 40, stub)
			if got == nil {
				t.Fatal("RenderAll returned nil, want an empty non-nil slice")
			}
			if len(got) != 0 {
				t.Fatalf("got %d lines, want 0: %q", len(got), viewtest.JoinPlain(got))
			}
		})
	}
}
