package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func wheelMsg(button tea.MouseButton, x, y int) tea.MouseMsg {
	return tea.MouseWheelMsg(tea.Mouse{X: x, Y: y, Button: button})
}

func clickMsg(button tea.MouseButton, x, y int) tea.MouseMsg {
	return tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: button})
}

func shortHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, nil, nil)
	h.send(tea.WindowSizeMsg{Width: 120, Height: 12})
	h.load(testPatch)
	h.keys("a")
	if lines := h.m.ensureLines(); len(lines) <= h.m.bodyHeight() {
		t.Fatalf("%d lines fit in body height %d: nothing to scroll", len(lines), h.m.bodyHeight())
	}
	return h
}

func TestWheelScrollsTheWindow(t *testing.T) {
	h := shortHarness(t)
	h.m.setDcur(3)
	if h.m.scroll != 0 || h.m.dcur != 3 {
		t.Fatalf("start scroll %d dcur %d", h.m.scroll, h.m.dcur)
	}
	x, y := h.m.diffRect().x+2, h.m.diffRect().y+3
	h.send(wheelMsg(tea.MouseWheelDown, x, y))
	if h.m.scroll != 3 || h.m.dcur != 3 {
		t.Fatalf("after one notch: scroll %d dcur %d", h.m.scroll, h.m.dcur)
	}
	h.send(wheelMsg(tea.MouseWheelDown, x, y))
	if h.m.scroll != 6 || h.m.dcur != 6 {
		t.Fatalf("after two notches: scroll %d dcur %d", h.m.scroll, h.m.dcur)
	}
	h.keys("j", "j")
	if h.m.dcur != 8 || h.m.scroll != 6 {
		t.Fatalf("after j j: scroll %d dcur %d", h.m.scroll, h.m.dcur)
	}
	h.send(wheelMsg(tea.MouseWheelUp, x, y))
	if h.m.scroll != 3 || h.m.dcur != 8 {
		t.Fatalf("after wheel up: scroll %d dcur %d", h.m.scroll, h.m.dcur)
	}
	h.keys("end")
	last := len(h.m.ensureLines()) - 1
	if h.m.dcur != last {
		t.Fatalf("dcur %d, want %d", h.m.dcur, last)
	}
	h.send(wheelMsg(tea.MouseWheelUp, x, y))
	if want := h.m.scroll + h.m.bodyHeight() - 1; h.m.dcur != want {
		t.Fatalf("after wheel up at the end: dcur %d, want the bottom edge %d (scroll %d)", h.m.dcur, want, h.m.scroll)
	}
	for range 50 {
		h.send(wheelMsg(tea.MouseWheelUp, x, y))
	}
	if h.m.scroll != 0 || h.m.dcur != h.m.bodyHeight()-1 {
		t.Fatalf("scrolled above the first line: scroll %d dcur %d", h.m.scroll, h.m.dcur)
	}
}

func TestWheelStopsAtTheLastPage(t *testing.T) {
	h := shortHarness(t)
	h.m.setDcur(0)
	x, y := h.m.diffRect().x+2, h.m.diffRect().y+3
	for range 50 {
		h.send(wheelMsg(tea.MouseWheelDown, x, y))
	}
	if want := len(h.m.ensureLines()) - h.m.bodyHeight(); h.m.scroll != want {
		t.Fatalf("scroll %d, want the last page %d", h.m.scroll, want)
	}
	s := h.screen()
	lines := strings.Split(s, "\n")
	if len(lines) != 12 {
		t.Fatalf("%d screen lines", len(lines))
	}
	last := h.m.ensureLines()[len(h.m.ensureLines())-1].Plain
	if !strings.Contains(s, last) {
		t.Fatalf("last line %q not on screen:\n%s", last, s)
	}
}

func TestWheelOverTheListMovesTheGroupCursor(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	if len(h.m.groups()) < 2 {
		t.Fatalf("%d groups", len(h.m.groups()))
	}
	h.send(wheelMsg(tea.MouseWheelDown, 3, 3))
	if h.m.cursor != 1 {
		t.Fatalf("cursor %d after wheel down over the list", h.m.cursor)
	}
	h.send(wheelMsg(tea.MouseWheelUp, 3, 3))
	if h.m.cursor != 0 {
		t.Fatalf("cursor %d after wheel up over the list", h.m.cursor)
	}
	h.send(wheelMsg(tea.MouseWheelDown, h.m.diffRect().x+2, 3))
	if h.m.cursor != 0 {
		t.Fatalf("cursor %d: the diff wheel moved the list", h.m.cursor)
	}
}

func TestClickPutsTheCursorOnTheLine(t *testing.T) {
	h := shortHarness(t)
	h.m.setDcur(0)
	r := h.m.diffRect()
	h.send(clickMsg(tea.MouseLeft, r.x+4, r.y+4))
	if h.m.dcur != 4 || h.m.focus != focusDiff {
		t.Fatalf("dcur %d focus %v", h.m.dcur, h.m.focus)
	}
	h.m.scrollDiff(10)
	h.send(clickMsg(tea.MouseLeft, r.x+4, r.y+4))
	if h.m.dcur != h.m.scroll+4 {
		t.Fatalf("dcur %d, want %d", h.m.dcur, h.m.scroll+4)
	}
	h.send(clickMsg(tea.MouseLeft, r.x+4, r.y+r.h-1))
	if h.m.dcur >= len(h.m.ensureLines()) {
		t.Fatalf("dcur %d past the end (%d lines)", h.m.dcur, len(h.m.ensureLines()))
	}
}

func TestClickOnTheListSelectsTheGroup(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.load(testPatch)
	h.keys("enter")
	if h.m.focus != focusDiff {
		t.Fatal("not on the diff pane")
	}
	h.send(clickMsg(tea.MouseLeft, 3, h.m.listRect().y+1))
	if h.m.cursor != 1 || h.m.focus != focusList {
		t.Fatalf("cursor %d focus %v", h.m.cursor, h.m.focus)
	}
	if g, ok := h.m.currentGroup(); !ok || g.Title != "Changes in (root)" {
		t.Fatalf("wrong group %+v", g)
	}
}

func TestClickIgnoresOverlaysAndOtherButtons(t *testing.T) {
	h := shortHarness(t)
	h.m.setDcur(0)
	r := h.m.diffRect()
	h.m.ov = ovHelp
	h.send(clickMsg(tea.MouseLeft, r.x+4, r.y+4))
	if h.m.dcur != 0 {
		t.Fatalf("a click under the help overlay moved the cursor to %d", h.m.dcur)
	}
	h.m.ov = ovNone
	h.send(clickMsg(tea.MouseRight, r.x+4, r.y+5))
	h.send(wheelMsg(tea.MouseWheelLeft, r.x+4, r.y+3))
	if h.m.dcur != 0 || h.m.scroll != 0 {
		t.Fatalf("right and horizontal events acted: dcur %d scroll %d", h.m.dcur, h.m.scroll)
	}
	h.send(clickMsg(tea.MouseLeft, r.x+4, 0))
	h.send(wheelMsg(tea.MouseWheelDown, r.x+4, 0))
	if h.m.dcur != 0 || h.m.scroll != 0 {
		t.Fatalf("a header click acted: dcur %d scroll %d", h.m.dcur, h.m.scroll)
	}
}

func TestDiffRectMatchesTheLayout(t *testing.T) {
	for _, sz := range [][2]int{{120, 12}, {80, 14}, {140, 10}} {
		t.Run(fmt.Sprint(sz), func(t *testing.T) {
			h := newHarness(t, nil, nil)
			h.send(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			h.load(testPatch)
			h.keys("a")
			h.m.scrollDiff(6)
			lines := h.m.ensureLines()
			r := h.m.diffRect()
			rows := strings.Split(h.screen(), "\n")
			if r.y < 0 || r.y+r.h > len(rows) {
				t.Fatalf("rect %+v outside %d rows", r, len(rows))
			}
			if want := lines[h.m.scroll].Plain; !strings.Contains(rows[r.y], want) {
				t.Fatalf("row %d is %q, want it to hold %q", r.y, rows[r.y], want)
			}
			h.send(clickMsg(tea.MouseLeft, r.x+1, r.y))
			if h.m.dcur != h.m.scroll {
				t.Fatalf("click on the first pane row: dcur %d, scroll %d", h.m.dcur, h.m.scroll)
			}
		})
	}
}
