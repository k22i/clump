package tui

import (
	"strings"
	"testing"

	"github.com/k22i/clump/internal/report"
	"github.com/k22i/clump/internal/tui/view"
)

func reportMarkdown(h *harness) string { return report.Markdown(h.m.reportDoc()) }

// lineOf finds the first line of the given kind and returns its index.
func lineOf(h *harness, kind view.LineKind) int {
	for i, l := range h.m.ensureLines() {
		if l.Kind == kind {
			return i
		}
	}
	return -1
}

func TestCommentOnLine(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("enter") // focus the diff pane
	if i := lineOf(h, view.KindAdd); i < 0 {
		t.Fatal("no + line on screen")
	} else {
		h.m.setDcur(i)
	}
	h.keys("C")
	h.typeText("magic number needs a name")
	h.keys("enter")
	if h.m.ov != ovNone || !strings.Contains(h.m.flash, "comment added") {
		t.Fatalf("flash %q ov %v", h.m.flash, h.m.ov)
	}
	lines := h.m.ensureLines()
	idx := lineOf(h, view.KindComment)
	if idx < 0 || !strings.Contains(lines[idx].Plain, "✎ magic number needs a name") {
		t.Fatalf("no comment row: %d", idx)
	}
	if lines[idx].Kind != view.KindComment || lines[idx].Gutter == 0 {
		t.Fatalf("comment row: %+v", lines[idx])
	}
	// The comment lands under the line it is about.
	var at int
	for i, l := range lines {
		if l.Kind == view.KindComment {
			at = i
			break
		}
	}
	if at < 2 || lines[at-1].Kind == view.KindFile {
		t.Fatalf("comment not under its line: %d %v", at, lines[at-1].Kind)
	}
	// Comments are content-anchored: a second session sees them.
	h2 := newHarness(t, h.store, nil)
	h2.load(testPatch)
	h2.keys("enter")
	if lineOf(h2, view.KindComment) < 0 {
		t.Fatal("comment not persisted")
	}

	// D under the comment row deletes exactly that comment.
	h.m.setDcur(idx)
	h.keys("D")
	if !strings.Contains(h.m.flash, "comment deleted") {
		t.Fatalf("flash %q", h.m.flash)
	}
	h3 := newHarness(t, h.store, nil)
	h3.load(testPatch)
	h3.keys("enter")
	if lineOf(h3, view.KindComment) >= 0 {
		t.Fatal("comment survived deletion")
	}
}

func TestCommentOnFileHeader(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("enter")
	h.m.setDcur(h.m.headerLine(0)) // src/a.go
	h.keys("C")
	if !strings.HasPrefix(h.m.input.title.prompt, "Comment on src/a.go") {
		t.Fatalf("prompt %q", h.m.input.title.prompt)
	}
	h.typeText("needs tests")
	h.keys("enter")
	// The header is reviewed? No, but the comment shows even folded.
	lines := h.m.ensureLines()
	found := false
	for i, l := range lines {
		if l.Kind == view.KindComment && strings.Contains(l.Plain, "needs tests") {
			found = true
			if lines[i-1].Kind != view.KindFile {
				t.Fatalf("file comment not under its header: %s", lines[i-1].Plain)
			}
			h.m.setDcur(i)
		}
	}
	if !found {
		t.Fatal("file comment missing")
	}
	h.keys("D")
	if lineOf(h, view.KindComment) >= 0 {
		t.Fatal("file comment not deleted")
	}
}

func TestCommentOnGroupFromList(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("C")
	if !strings.HasPrefix(h.m.input.title.prompt, "Comment on ") {
		t.Fatalf("prompt %q", h.m.input.title.prompt)
	}
	h.typeText("this whole logical change should be split")
	h.keys("enter")
	// The comment weaves into the group block in grouped mode.
	s := h.screen()
	if !strings.Contains(s, "✎ this whole logical change should be split") {
		t.Fatalf("group comment not on screen:\n%s", s)
	}
	// And into the report, placed under its group.
	doc := h.m.reportDoc()
	if len(doc.Comments) != 1 {
		t.Fatalf("report comments: %+v", doc.Comments)
	}
	c := doc.Comments[0]
	if c.Kind.String() != "group" || c.GroupID != h.grouping().Groups[0].ID {
		t.Fatalf("placed comment: %+v", c)
	}
	md := reportMarkdown(h)
	if !strings.Contains(md, "Review comments") ||
		!strings.Contains(md, "1. this group — this whole logical change should be split") {
		t.Fatalf("markdown:\n%s", md)
	}
}

func TestCommentPersistenceAcrossRegroup(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("enter")
	if i := lineOf(h, view.KindAdd); i < 0 {
		t.Fatal("no + line")
	} else {
		h.m.setDcur(i)
	}
	h.keys("C")
	h.typeText("line comment")
	h.keys("enter")
	if lineOf(h, view.KindComment) < 0 {
		t.Fatal("comment not shown")
	}
	// A regroup changes the group IDs but the line comment survives.
	h.keys("esc") // back to the list
	h.keys("r")   // regroup, bypassing the cache
	if lineOf(h, view.KindComment) < 0 && h.m.focus == focusList {
		h.keys("enter")
	}
	if i := lineOf(h, view.KindComment); i < 0 {
		t.Fatal("line comment lost after regroup")
	}
}

func TestEmptyCommentIgnored(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("C")
	h.typeText("   ")
	h.keys("enter")
	if !strings.Contains(h.m.flash, "empty comment") || len(h.m.comments) != 0 {
		t.Fatalf("flash %q comments %v", h.m.flash, h.m.comments)
	}
}
