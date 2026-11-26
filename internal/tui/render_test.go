package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/tui/view"
)

func TestFooterHintsDropWholeAndKeepTheirStyles(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	th := view.NewTheme(true)
	h.m.th = th

	first := th.Style(th.Key, "?") + " " + th.Style(th.Dim, "keys")
	full := h.m.renderHints("", th.Dim)
	if !strings.HasPrefix(full, first) {
		t.Fatalf("wide footer = %q, want it to start with %q", full, first)
	}
	if last := th.Style(th.Key, "q") + " " + th.Style(th.Dim, "quit"); !strings.Contains(full, last) {
		t.Fatalf("wide footer = %q, want the last hint %q", full, last)
	}

	h.m.width = 24
	narrow := h.m.renderHints("", th.Dim)
	sep := th.Style(th.Dim, " · ")
	if got, want := narrow, first+sep+th.Style(th.Key, "/")+" "+th.Style(th.Dim, "search"); got != want {
		t.Fatalf("footer at width 24 = %q, want %q", got, want)
	}
	if got := lipgloss.Width(narrow); got != 17 {
		t.Fatalf("footer at width 24 is %d cells, want 17", got)
	}
	for _, fragment := range []string{"revi", "quit"} {
		if strings.Contains(narrow, fragment) {
			t.Fatalf("footer at width 24 = %q, want no hint cut down to %q", narrow, fragment)
		}
	}
	if strings.Contains(narrow, th.Style(th.Title, "?")) || strings.Contains(narrow, th.Style(th.Title, "keys")) {
		t.Fatalf("footer at width 24 = %q, want the hints in their own styles and nothing in the title style", narrow)
	}
}

func TestFooterLeadIsTruncatedBeforeItIsStyled(t *testing.T) {
	h := newHarness(t, nil, nil)
	th := view.NewTheme(true)
	h.m.th = th
	h.m.width = 3
	got := h.m.renderHints("VISUAL", th.Title)
	if want := th.Style(th.Title, "VI…"); got != want {
		t.Fatalf("renderHints(VISUAL) at width 3 = %q, want %q", got, want)
	}
	if n := strings.Count(got, "\x1b"); n != 2 {
		t.Fatalf("renderHints(VISUAL) at width 3 = %q (%d escapes), want one styled run and no cut sequence", got, n)
	}
}
