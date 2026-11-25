package diffview_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui/diffview"
	"github.com/k22i/clump/internal/tui/view"
	"github.com/k22i/clump/internal/tui/view/viewtest"
)

// allIDs returns every hunk ID of d in diff order.
func allIDs(d *gitdiff.Diff) []string {
	ids := make([]string, 0, len(d.Hunks))
	for _, h := range d.Hunks {
		ids = append(ids, h.ID)
	}
	return ids
}

// countSGR counts the SGR sequences in s.
func countSGR(s string) int { return strings.Count(s, "\x1b[") }

func TestGoldenCommit(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/commit-67a7d22.txt")
	lines := diffview.RenderGroupLines(d, allIDs(d), view.PlainTheme(), 80, nil)
	viewtest.Golden(t, "dp-commit-67a7d22", viewtest.JoinPlain(lines))
}

func TestGoldenBinaryMode(t *testing.T) {
	d := viewtest.FixtureDiff(t, "git/binary-mode.show.txt")
	lines := diffview.RenderGroupLines(d, allIDs(d), view.PlainTheme(), 80, nil)
	if n := len(d.Files); n != 3 {
		t.Fatalf("fixture has %d files, want 3", n)
	}
	headers := 0
	for _, l := range lines {
		if l.Kind == view.KindFile {
			headers++
		}
	}
	if headers != 3 {
		t.Fatalf("got %d file headers, want 3", headers)
	}
	viewtest.Golden(t, "dp-binary-mode", viewtest.JoinPlain(lines))
}

func TestGoldenMarkedNarrow(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/commit-67a7d22.txt")
	lines := diffview.RenderGroupLines(d, allIDs(d), view.PlainTheme(), 30, map[string]bool{"h2": true})
	if len(lines) == 0 {
		t.Fatal("no lines rendered")
	}
	seen, truncated, marked := 0, 0, 0
	for _, l := range lines {
		if w := lipgloss.Width(l.Plain); w > 30 {
			t.Fatalf("line %q is %d cells wide, want at most 30", l.Plain, w)
		}
		if strings.HasSuffix(l.Plain, "…") {
			truncated++
		}
		if l.HunkID == "h2" {
			seen++
			if !strings.HasPrefix(l.Plain, "x ") {
				t.Fatalf("h2 line %q does not start with the marked gutter", l.Plain)
			}
			marked++
		} else if strings.HasPrefix(l.Plain, "x ") {
			t.Fatalf("unmarked line %q starts with the marked gutter", l.Plain)
		}
	}
	if seen == 0 || marked != seen {
		t.Fatalf("saw %d h2 lines, %d marked", seen, marked)
	}
	if truncated == 0 {
		t.Fatal("no line was truncated at width 30")
	}
	viewtest.Golden(t, "dp-marked-narrow", viewtest.JoinPlain(lines))
}

func TestGoldenNoNewline(t *testing.T) {
	d := viewtest.FixtureDiff(t, "edge/nonewline-both.diff")
	lines := diffview.RenderGroupLines(d, allIDs(d), view.PlainTheme(), 80, nil)
	markers := 0
	for _, l := range lines {
		if l.Kind == view.KindNoNewline {
			markers++
			if want := "  \\ No newline at end of file"; l.Plain != want {
				t.Fatalf("marker line is %q, want %q", l.Plain, want)
			}
		}
	}
	if markers != 2 {
		t.Fatalf("got %d no-newline markers, want 2", markers)
	}
	viewtest.Golden(t, "dp-nonewline", viewtest.JoinPlain(lines))
}

func TestSubsetAndOrder(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/range-selection.txt")
	lines := diffview.RenderGroupLines(d, []string{"h4", "h1", "zz"}, view.PlainTheme(), 80, nil)

	var headers []view.Line
	for _, l := range lines {
		if l.Kind == view.KindFile {
			headers = append(headers, l)
		}
	}
	if len(headers) != 2 {
		t.Fatalf("got %d file headers, want 2: %v", len(headers), headers)
	}
	if !strings.Contains(headers[0].Plain, "docs/old.txt -> docs/new.txt") {
		t.Fatalf("first header is %q, want the docs file", headers[0].Plain)
	}
	// h1 is the only shown hunk of that file: +1 −0.
	if !strings.Contains(headers[0].Plain, "(R +1 −0)") {
		t.Fatalf("first header is %q, want the h1 sums", headers[0].Plain)
	}
	if !strings.Contains(headers[1].Plain, "src/main.go") {
		t.Fatalf("second header is %q, want src/main.go", headers[1].Plain)
	}
	if !strings.Contains(headers[1].Plain, "(T +0 −0)") {
		t.Fatalf("second header is %q, want the pseudo-hunk sums", headers[1].Plain)
	}

	var ids []string
	for _, l := range lines {
		if l.Kind == view.KindHunk {
			ids = append(ids, l.HunkID)
		}
	}
	if want := []string{"h1", "h4"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("rendered hunks %v, want %v", ids, want)
	}
}

// nums is the (Kind, OldLine, NewLine) triple of a rendered line.
type nums struct {
	kind     view.LineKind
	old, new int
}

// triples returns the numbers of every line.
func triples(lines []view.Line) []nums {
	out := make([]nums, 0, len(lines))
	for _, l := range lines {
		out = append(out, nums{kind: l.Kind, old: l.OldLine, new: l.NewLine})
	}
	return out
}

func TestLineNumbers(t *testing.T) {
	hand := &gitdiff.Hunk{
		ID:       "h1",
		Header:   "@@ -10,3 +20,4 @@",
		OldStart: 10, NewStart: 20,
		Lines: []string{" a", "-b", "+c", "+d", "", "\\ No newline at end of file"},
	}
	tests := []struct {
		name  string
		lines []view.Line
		want  []nums
	}{
		{
			name: "omitted counts",
			lines: diffview.RenderGroupLines(
				viewtest.FixtureDiff(t, "edge/omitted-counts.diff"),
				[]string{"h1", "h2"}, view.PlainTheme(), 80, nil),
			want: []nums{
				{view.KindFile, 0, 0},
				{view.KindHunk, 1, 1},
				{view.KindContext, 1, 1},
				{view.KindAdd, 0, 2},
				{view.KindHunk, 10, 12},
				{view.KindAdd, 0, 12},
			},
		},
		{
			name:  "hand built",
			lines: diffview.RenderHunk(hand, view.PlainTheme(), diffview.HunkOpts{Width: 80}),
			want: []nums{
				{view.KindHunk, 10, 20},
				{view.KindContext, 10, 20},
				{view.KindDel, 11, 0},
				{view.KindAdd, 0, 21},
				{view.KindAdd, 0, 22},
				{view.KindContext, 12, 23},
				{view.KindNoNewline, 0, 0},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := triples(tc.lines); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTabExpansion(t *testing.T) {
	h := &gitdiff.Hunk{
		ID:     "h1",
		Header: "@@ -1 +1,2 @@",
		Lines:  []string{"+\tx\ty", "+ab\tc"},
	}
	lines := diffview.RenderHunk(h, view.PlainTheme(), diffview.HunkOpts{Width: 80})
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"tab at column 0", lines[1].Plain, "  +    x   y"},
		{"tab after two columns", lines[2].Plain, "  +ab  c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestPlainThemeTextEqualsPlain(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/commit-67a7d22.txt")
	lines := diffview.RenderGroupLines(d, allIDs(d), view.PlainTheme(), 80, nil)
	if len(lines) == 0 {
		t.Fatal("no lines rendered")
	}
	for i, l := range lines {
		if l.Text != l.Plain {
			t.Fatalf("line %d: Text %q != Plain %q", i, l.Text, l.Plain)
		}
	}
}

func TestColouredStripsToPlain(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/commit-67a7d22.txt")
	for _, dark := range []bool{true, false} {
		for _, width := range []int{80, 30} {
			name := fmt.Sprintf("dark=%v/width=%d", dark, width)
			t.Run(name, func(t *testing.T) {
				th := view.NewTheme(dark)
				lines := diffview.RenderGroupLines(d, allIDs(d), th, width, nil)
				if len(lines) == 0 {
					t.Fatal("no lines rendered")
				}
				goStyled := 0
				for i, l := range lines {
					if got := gitdiff.StripSGR(l.Text); got != l.Plain {
						t.Fatalf("line %d: StripSGR(Text) %q != Plain %q", i, got, l.Plain)
					}
					if !isBody(l.Kind) || l.FileIdx < 0 || l.FileIdx >= len(d.Files) {
						continue
					}
					if !strings.HasSuffix(d.Files[l.FileIdx].Path(), ".go") {
						continue
					}
					if countSGR(l.Text) > 0 {
						goStyled++
					}
				}
				if goStyled == 0 {
					t.Fatal("no Go body line was styled")
				}
			})
		}
	}
}

// isBody reports whether kind is a hunk body line kind.
func isBody(kind view.LineKind) bool {
	switch kind {
	case view.KindAdd, view.KindDel, view.KindContext:
		return true
	}
	return false
}

func TestNoLexer(t *testing.T) {
	h := &gitdiff.Hunk{
		ID:       "h1",
		Header:   "@@ -1 +1 @@",
		OldStart: 1, NewStart: 1,
		Lines: []string{"-x := 1", "+y := 2"},
	}
	lines := diffview.RenderHunk(h, view.NewTheme(true), diffview.HunkOpts{Width: 40, Path: "file.unknownext"})
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	for i, l := range lines {
		if got := gitdiff.StripSGR(l.Text); got != l.Plain {
			t.Fatalf("line %d: StripSGR(Text) %q != Plain %q", i, got, l.Plain)
		}
	}
	// The body keeps the theme's +/- colours even without a lexer.
	if countSGR(lines[1].Text) == 0 || countSGR(lines[2].Text) == 0 {
		t.Fatal("body lines lost their theme colours")
	}
}

func TestWordDiff(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		wantOld []diffview.Span
		wantNew []diffview.Span
		wantOK  bool
	}{
		{
			name:   "identical",
			old:    "total := a + b",
			new:    "total := a + b",
			wantOK: true,
		},
		{
			name:    "one changed token",
			old:     "foo(a, b)",
			new:     "foo(a, c)",
			wantOld: []diffview.Span{{Start: 7, End: 8}},
			wantNew: []diffview.Span{{Start: 7, End: 8}},
			wantOK:  true,
		},
		{
			name:    "tail rewritten",
			old:     "return x",
			new:     "return y + 1",
			wantOld: []diffview.Span{{Start: 7, End: 8}},
			wantNew: []diffview.Span{{Start: 7, End: 12}},
			wantOK:  true,
		},
		{
			name:    "word inserted in the middle",
			old:     "the quick fox",
			new:     "the quick brown fox",
			wantNew: []diffview.Span{{Start: 10, End: 16}},
			wantOK:  true,
		},
		{
			name:   "unrelated lines",
			old:    "alpha beta",
			new:    "gamma delta",
			wantOK: false,
		},
		{
			name:   "too long",
			old:    strings.Repeat("x", 1500),
			new:    strings.Repeat("y", 1500),
			wantOK: false,
		},
		{
			name:   "too many tokens",
			old:    strings.Repeat("a ", 101) + "b",
			new:    strings.Repeat("a ", 101) + "c",
			wantOK: false,
		},
		{
			name:    "changed rune is one token",
			old:     "x ÿ y",
			new:     "x z y",
			wantOld: []diffview.Span{{Start: 2, End: 4}},
			wantNew: []diffview.Span{{Start: 2, End: 3}},
			wantOK:  true,
		},
		{
			name:    "offsets after multi-byte runes",
			old:     "aïe b",
			new:     "aïe c",
			wantOld: []diffview.Span{{Start: 5, End: 6}},
			wantNew: []diffview.Span{{Start: 5, End: 6}},
			wantOK:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldSpans, newSpans, ok := diffview.WordDiff(tc.old, tc.new)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (spans %v / %v)", ok, tc.wantOK, oldSpans, newSpans)
			}
			if !reflect.DeepEqual(oldSpans, tc.wantOld) {
				t.Fatalf("old spans %v, want %v", oldSpans, tc.wantOld)
			}
			if !reflect.DeepEqual(newSpans, tc.wantNew) {
				t.Fatalf("new spans %v, want %v", newSpans, tc.wantNew)
			}
		})
	}
}

func TestWordHighlightApplied(t *testing.T) {
	paired := &gitdiff.Hunk{
		ID:       "h1",
		Header:   "@@ -1 +1 @@",
		OldStart: 1, NewStart: 1,
		Lines: []string{"-total := a + b", "+total := a + c"},
	}
	alone := &gitdiff.Hunk{
		ID:       "h1",
		Header:   "@@ -1 +1 @@",
		OldStart: 1, NewStart: 1,
		Lines: []string{"+total := a + c"},
	}
	th := view.NewTheme(true)
	lines := diffview.RenderHunk(paired, th, diffview.HunkOpts{Width: 80})
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	single := diffview.RenderHunk(alone, th, diffview.HunkOpts{Width: 80})
	if len(single) != 2 {
		t.Fatalf("got %d lines, want 2", len(single))
	}
	add, unpaired := lines[2], single[1]
	if add.Plain != unpaired.Plain {
		t.Fatalf("plain text differs: %q vs %q", add.Plain, unpaired.Plain)
	}
	if countSGR(add.Text) <= countSGR(unpaired.Text) {
		t.Fatalf("paired add line has %d SGR sequences, unpaired %d: no word highlight",
			countSGR(add.Text), countSGR(unpaired.Text))
	}
	if got := gitdiff.StripSGR(add.Text); got != add.Plain {
		t.Fatalf("StripSGR(Text) %q != Plain %q", got, add.Plain)
	}
}

func TestEmpty(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/commit-67a7d22.txt")
	for _, tc := range []struct {
		name string
		ids  []string
	}{
		{"no ids", nil},
		{"only unknown ids", []string{"zz", "nope"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := diffview.RenderGroupLines(d, tc.ids, view.PlainTheme(), 80, nil)
			if lines == nil {
				t.Fatal("got nil, want an empty slice")
			}
			if len(lines) != 0 {
				t.Fatalf("got %d lines, want 0", len(lines))
			}
		})
	}
}

func TestPerf(t *testing.T) {
	const n = 3000
	body := make([]string, 0, n)
	for i := 0; i < n; i++ {
		body = append(body, fmt.Sprintf("+\tif x%d := compute(%d); x%d > 0 {", i, i, i))
	}
	h := &gitdiff.Hunk{
		ID:       "h1",
		Header:   fmt.Sprintf("@@ -1,%d +1,%d @@", n, n),
		OldStart: 1, NewStart: 1,
		Lines: body,
	}
	// 1.5s is the budget for an uninstrumented build. Chroma's regexp engine
	// runs roughly 25x slower under the race detector, so the -race run gets
	// a looser ceiling that still catches real regressions.
	budget := 1500 * time.Millisecond
	if raceEnabled {
		budget = 12 * time.Second
	}
	start := time.Now()
	lines := diffview.RenderHunk(h, view.NewTheme(true), diffview.HunkOpts{Width: 120, Path: "big.go"})
	if took := time.Since(start); took > budget {
		t.Fatalf("rendering %d lines took %v, want under %v", n, took, budget)
	}
	if len(lines) != n+1 {
		t.Fatalf("got %d lines, want %d", len(lines), n+1)
	}
	if lines[1].Plain == "" || countSGR(lines[1].Text) == 0 {
		t.Fatal("first body line was not styled")
	}
}
