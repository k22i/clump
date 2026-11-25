package view_test

import (
	"testing"

	"github.com/k22i/clump/internal/tui/view"
	"github.com/k22i/clump/internal/tui/view/viewtest"
)

func TestRenderFileHeader(t *testing.T) {
	d := viewtest.FixtureDiff(t, "lazygit/commit-bdd913d.txt")
	l := view.RenderFileHeader(d.Files[0], 0, 1, 0, view.PlainTheme(), 50)
	if l.Plain != "── docs/old.txt -> docs/new.txt (R +1 −0) ────────" || l.Text != l.Plain || l.Kind != view.KindFile {
		t.Fatalf("got %q", l.Plain)
	}
	if got := view.RenderFileHeader(d.Files[0], 0, 1, 0, view.PlainTheme(), 20).Plain; got != "── docs/old.txt -> …" {
		t.Fatalf("narrow: %q", got)
	}
	if view.Truncate("héllo", 3) != "hé…" || view.Truncate("abc", 3) != "abc" || view.Truncate("abc", 0) != "abc" {
		t.Fatal("Truncate")
	}
}
