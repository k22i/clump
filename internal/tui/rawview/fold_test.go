package rawview

import (
	"strings"
	"testing"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/tui/view"
	"github.com/k22i/clump/internal/tui/view/viewtest"
)

func TestRenderAllFolded(t *testing.T) {
	d, err := gitdiff.Parse("diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-x\n+y\n" +
		"diff --git a/b b/b\n--- a/b\n+++ b/b\n@@ -1 +1 @@\n-p\n+q\n")
	if err != nil {
		t.Fatal(err)
	}
	g := group.Grouping{Groups: []group.Group{{ID: "g1", Title: "Both", Hunks: []string{"h1", "h2"}}}}
	stub := func(h *gitdiff.Hunk, w int) []view.Line {
		return []view.Line{{Plain: h.Header, Text: h.Header, Kind: view.KindHunk, HunkID: h.ID}}
	}
	lines := RenderAllOpts(d, g, view.PlainTheme(), 40, stub, func(fi int, ids []string) view.FileState {
		return view.FileState{Folded: fi == 0}
	})
	got := viewtest.JoinPlain(lines)
	if strings.Count(got, "┄┄ 1. Both") != 1 || strings.Index(got, "┄┄ 1. Both") < strings.Index(got, "── b ") {
		t.Fatalf("got\n%s", got)
	}
	if !strings.Contains(got, "── a (M +1 −1) · 1 hunk folded ") || lines[0].Gutter != 0 || lines[len(lines)-1].Gutter != LabelWidth {
		t.Fatalf("got\n%s", got)
	}
}
