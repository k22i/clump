package tui

import (
	"strings"
	"testing"

	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/tui/view"
)

func TestReviewGroupsFromTheList(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("c")
	if !h.m.allReviewed(h.grouping().Groups[0].Hunks) || h.m.cursor != 1 {
		t.Fatalf("after c: cursor %d", h.m.cursor)
	}
	s := h.screen()
	if !strings.Contains(s, "1/2 reviewed") || !strings.Contains(s, "3h ✓") || !strings.Contains(h.m.flash, "next: Changes in (root)") {
		t.Fatalf("screen:\n%s\nflash %q", s, h.m.flash)
	}
	// Marks are kept by content: a new session on the same diff sees them.
	h2 := newHarness(t, h.store, nil)
	h2.load(testPatch)
	if !h2.m.allReviewed(h2.grouping().Groups[0].Hunks) || h2.m.allReviewed(h2.grouping().Groups[1].Hunks) {
		t.Fatal("reviewed marks not persisted")
	}
	h2.keys("j", "c")
	if !strings.Contains(h2.m.flash, "all 2 groups reviewed") {
		t.Fatalf("flash %q", h2.m.flash)
	}
	// c on a reviewed group unmarks it.
	h2.keys("c")
	if h2.m.allReviewed(h2.grouping().Groups[1].Hunks) || !strings.Contains(h2.m.flash, "unreviewed") {
		t.Fatalf("unmark: flash %q", h2.m.flash)
	}
}

func TestReviewFilesInTheDiffPane(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("enter")
	h.m.setDcur(h.m.headerLine(0)) // src/a.go
	h.keys("c")
	lines := h.m.ensureLines()
	a := lines[h.m.headerLine(0)]
	if !a.Folded || !strings.Contains(a.Plain, "── ✓ src/a.go (M +2 −0) · 2 hunks folded") {
		t.Fatalf("a.go header %q", a.Plain)
	}
	if h.m.dcur != h.m.headerLine(1) || h.m.cursor != 0 {
		t.Fatalf("cursor not on the next file: dcur %d", h.m.dcur)
	}
	// z unfolds the reviewed file, z again folds it.
	h.m.setDcur(h.m.headerLine(0))
	h.keys("z")
	if h.m.ensureLines()[h.m.headerLine(0)].Folded {
		t.Fatal("z did not unfold")
	}
	h.keys("z")
	if !h.m.ensureLines()[h.m.headerLine(0)].Folded {
		t.Fatal("z did not fold")
	}
	// Checking the last file completes the group and moves on.
	h.m.setDcur(h.m.headerLine(1) + 2) // inside src/b.go
	h.keys("c")
	if h.m.cursor != 1 || h.m.focus != focusDiff {
		t.Fatalf("after the last file: cursor %d", h.m.cursor)
	}
}

func TestFoldAllAndLineNumbers(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	if s := h.screen(); !strings.Contains(s, "   1  1  package a") || !strings.Contains(s, "      2 +import \"fmt\"") {
		t.Fatalf("no line numbers:\n%s", s)
	}
	h.keys("#")
	if s := h.screen(); strings.Contains(s, "   1  1  package a") || !strings.Contains(s, "   package a") {
		t.Fatalf("line numbers still on:\n%s", s)
	}
	h.keys("Z")
	for _, l := range h.m.ensureLines() {
		if l.Kind == view.KindFile && !l.Folded {
			t.Fatalf("not folded: %q", l.Plain)
		}
		if l.Kind == view.KindAdd {
			t.Fatalf("hunk line shown: %q", l.Plain)
		}
	}
	h.keys("Z")
	for _, l := range h.m.ensureLines() {
		if l.Kind == view.KindFile && l.Folded {
			t.Fatalf("still folded: %q", l.Plain)
		}
	}
	// Search skips the line-number gutter.
	h.keys("#", "/")
	h.typeText("1")
	h.keys("enter")
	for _, i := range h.m.matches {
		if l := h.m.ensureLines()[i]; l.Kind != view.KindHunk && !strings.Contains(l.Plain[l.Gutter:], "1") {
			t.Fatalf("matched the gutter: %q", l.Plain)
		}
	}
}

const generatedPatch = testPatch + `diff --git a/go.sum b/go.sum
--- a/go.sum
+++ b/go.sum
@@ -1 +1,2 @@
 example.com/a v1.0.0 h1:abc=
+example.com/b v1.2.0 h1:SECRETSUMNOTSENT=
`

func TestGeneratedFilesGroup(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(generatedPatch)
	h.check()
	gs := h.grouping().Groups
	last := gs[len(gs)-1]
	if last.ID != group.GeneratedID || strings.Join(last.Hunks, ",") != "h5" {
		t.Fatalf("groups %+v", gs)
	}
	for _, p := range h.fake.Prompts() {
		if strings.Contains(p, "SECRETSUMNOTSENT") {
			t.Fatal("generated hunk sent to the model")
		}
	}
	// Not part of the review progress; shown folded with its reason.
	if _, total := h.m.reviewProgress(); total != len(gs)-1 {
		t.Fatalf("total %d", total)
	}
	h.keys("end")
	s := h.screen()
	if !strings.Contains(s, "go.sum (M +1 −0) · 1 hunk folded · generated (pattern go.sum)") || strings.Contains(s, "SECRETSUMNOTSENT") {
		t.Fatalf("screen:\n%s", s)
	}
	h.keys("z")
	if !strings.Contains(h.screen(), "SECRETSUMNOTSENT") {
		t.Fatal("z from the list did not unfold")
	}
}

func TestToggleSideBySide(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	for _, l := range h.m.ensureLines() {
		if strings.Contains(l.Plain, "│") {
			t.Fatalf("split rendered before s: %q", l.Plain)
		}
	}
	h.keys("s")
	if !h.m.split {
		t.Fatalf("s: split=%v flash %q", h.m.split, h.m.flash)
	}
	if !strings.Contains(h.m.flash, "side by side on") {
		t.Fatalf("flash %q", h.m.flash)
	}
	var split int
	for _, l := range h.m.ensureLines() {
		if strings.Contains(l.Plain, "│") {
			split++
		}
	}
	if split == 0 {
		t.Fatal("s drew no side-by-side line")
	}
	h.keys("s")
	if h.m.split || !strings.Contains(h.m.flash, "side by side off") {
		t.Fatalf("s again: split=%v flash %q", h.m.split, h.m.flash)
	}

	// Config side_by_side starts the TUI split.
	h2 := newHarness(t, nil, func(c *config.Config) { c.SideBySide = true })
	h2.load(testPatch)
	if !h2.m.split {
		t.Fatal("config side_by_side = true ignored")
	}
}
