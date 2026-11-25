package helpview_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui/helpview"
	"github.com/k22i/clump/internal/tui/view"
	"github.com/k22i/clump/internal/tui/view/viewtest"
)

// sample is the help content the golden tests render.
var sample = []view.HelpSection{
	{Title: "Navigate", Entries: []view.HelpEntry{{Keys: "j/k", Desc: "next/previous group"}, {Keys: "enter", Desc: "focus the diff pane"}, {Keys: "]/[", Desc: "next/previous group from anywhere"}, {Keys: "a", Desc: "grouped / all hunks"}}},
	{Title: "Search", Entries: []view.HelpEntry{{Keys: "/", Desc: "search"}, {Keys: "n/N", Desc: "next/previous match"}}},
	{Title: "Edit", Entries: []view.HelpEntry{{Keys: "x", Desc: "mark hunk"}, {Keys: "M", Desc: "move marked hunks"}, {Keys: "R", Desc: "rename group"}, {Keys: "u/ctrl+r", Desc: "undo/redo"}}},
}

// splitLines splits rendered output into lines, treating "" as no lines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// checkWidth fails when any line of got is wider than width cells.
func checkWidth(t *testing.T, got string, width int) {
	t.Helper()
	for i, l := range splitLines(got) {
		if w := lipgloss.Width(l); w > width {
			t.Fatalf("line %d is %d cells wide, want at most %d: %q", i, w, width, l)
		}
	}
}

func TestGoldenHelpTall(t *testing.T) {
	got := helpview.Render(sample, 60, 40, view.PlainTheme())
	if n := len(splitLines(got)); n != 17 {
		t.Fatalf("got %d lines, want 17:\n%s", n, got)
	}
	checkWidth(t, got, 60)
	viewtest.Golden(t, "hp-tall", got)
}

func TestGoldenHelpTwoColumns(t *testing.T) {
	got := helpview.Render(sample, 90, 12, view.PlainTheme())
	if !strings.Contains(got, " │ ") {
		t.Fatalf("no column separator in:\n%s", got)
	}
	if n := len(splitLines(got)); n > 12 {
		t.Fatalf("got %d lines, want at most 12:\n%s", n, got)
	}
	checkWidth(t, got, 90)
	viewtest.Golden(t, "hp-two-columns", got)
}

func TestHelpTruncatedHeight(t *testing.T) {
	got := helpview.Render(sample, 50, 6, view.PlainTheme())
	lines := splitLines(got)
	if len(lines) != 6 {
		t.Fatalf("got %d lines, want 6:\n%s", len(lines), got)
	}
	if last := lines[len(lines)-1]; last != "… (more)" {
		t.Fatalf("last line %q, want %q", last, "… (more)")
	}
	checkWidth(t, got, 50)
}

func TestHelpNarrow(t *testing.T) {
	got := helpview.Render(sample, 20, 40, view.PlainTheme())
	checkWidth(t, got, 20)
	if !strings.Contains(got, "…") {
		t.Fatalf("narrow render is not truncated:\n%s", got)
	}
}

func TestHelpColoured(t *testing.T) {
	th := view.NewTheme(true)
	tests := []struct {
		name          string
		width, height int
	}{
		{"one-column", 60, 40},
		{"two-columns", 90, 12},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plain := helpview.Render(sample, tc.width, tc.height, view.PlainTheme())
			got := helpview.Render(sample, tc.width, tc.height, th)
			if !strings.Contains(got, "\x1b[") {
				t.Fatalf("coloured render has no SGR sequences:\n%q", got)
			}
			if stripped := gitdiff.StripSGR(got); stripped != plain {
				t.Fatalf("stripped render differs from the plain one:\n--- stripped ---\n%s\n--- plain ---\n%s", stripped, plain)
			}
			if !strings.Contains(got, th.Title.Render("Search")) {
				t.Fatalf("section title is not drawn with th.Title:\n%q", got)
			}
			if !strings.Contains(got, th.Key.Render("n/N")) {
				t.Fatalf("keys are not drawn with th.Key:\n%q", got)
			}
		})
	}
}

func TestHelpZero(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{
		{"zero-height", 60, 0},
		{"zero-width", 0, 40},
		{"negative", -1, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := helpview.Render(sample, tc.width, tc.height, view.PlainTheme()); got != "" {
				t.Fatalf("got %q, want %q", got, "")
			}
		})
	}
}

func TestHelpNoSections(t *testing.T) {
	got := helpview.Render(nil, 40, 10, view.PlainTheme())
	want := "clump keys (? or esc to close)\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFindMatches(t *testing.T) {
	lines := []view.Line{{Plain: "Foo bar"}, {Plain: "foo"}, {Plain: "FOO"}, {Plain: "baz"}}
	tests := []struct {
		name  string
		query string
		want  []int
	}{
		{"insensitive", "foo", []int{0, 1, 2}},
		{"sensitive", "Foo", []int{0}},
		{"empty", "", nil},
		{"no-match", "zzz", nil},
		{"upper-only", "FOO", []int{2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := helpview.FindMatches(lines, tc.query)
			if len(got) != len(tc.want) {
				t.Fatalf("FindMatches(%q) = %v, want %v", tc.query, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("FindMatches(%q) = %v, want %v", tc.query, got, tc.want)
				}
			}
		})
	}
}

func TestNextMatch(t *testing.T) {
	matches := []int{2, 5, 9}
	tests := []struct {
		name    string
		matches []int
		from    int
		forward bool
		want    int
	}{
		{"forward-first", matches, 0, true, 2},
		{"forward-next", matches, 2, true, 5},
		{"forward-wrap", matches, 9, true, 2},
		{"forward-past-end", matches, 100, true, 2},
		{"backward-prev", matches, 5, false, 2},
		{"backward-wrap", matches, 2, false, 9},
		{"backward-past-end", matches, 100, false, 9},
		{"backward-before-start", matches, -1, false, 9},
		{"empty-forward", nil, 0, true, -1},
		{"empty-backward", nil, 0, false, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := helpview.NextMatch(tc.matches, tc.from, tc.forward); got != tc.want {
				t.Fatalf("NextMatch(%v, %d, %v) = %d, want %d", tc.matches, tc.from, tc.forward, got, tc.want)
			}
		})
	}
}

func TestHighlight(t *testing.T) {
	const plain = "a foo and FOO"
	th := view.NewTheme(true)
	got := helpview.Highlight(view.Line{Plain: plain}, "foo", th)

	if stripped := gitdiff.StripSGR(got); stripped != plain {
		t.Fatalf("stripped = %q, want %q", stripped, plain)
	}
	lower := th.Match.Render("foo")
	upper := th.Match.Render("FOO")
	if strings.Count(got, lower) != 1 || strings.Count(got, upper) != 1 {
		t.Fatalf("want exactly one %q run and one %q run in %q", lower, upper, got)
	}
	if strings.Count(got, th.Match.Render("a foo and")) != 0 {
		t.Fatalf("non-matching text is styled: %q", got)
	}

	t.Run("case-sensitive-no-match", func(t *testing.T) {
		if out := helpview.Highlight(view.Line{Plain: plain}, "Foo", th); out != plain {
			t.Fatalf("got %q, want %q", out, plain)
		}
	})

	t.Run("plain-theme", func(t *testing.T) {
		if out := helpview.Highlight(view.Line{Plain: plain}, "foo", view.PlainTheme()); out != plain {
			t.Fatalf("got %q, want %q", out, plain)
		}
	})

	t.Run("empty-query", func(t *testing.T) {
		if out := helpview.Highlight(view.Line{Plain: plain}, "", th); out != plain {
			t.Fatalf("got %q, want %q", out, plain)
		}
	})

	t.Run("exact-case", func(t *testing.T) {
		out := helpview.Highlight(view.Line{Plain: plain}, "FOO", th)
		if strings.Count(out, upper) != 1 || strings.Count(out, lower) != 0 {
			t.Fatalf("got %q, want only %q styled", out, upper)
		}
	})
}

func TestHighlightOverlap(t *testing.T) {
	const plain = "aaaa"
	th := view.NewTheme(true)
	got := helpview.Highlight(view.Line{Plain: plain}, "aa", th)
	want := th.Match.Render("aa") + th.Match.Render("aa")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
