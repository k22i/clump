package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/tui/listview"
)

// wheelStep is how many lines one wheel notch moves the diff pane.
const wheelStep = 3

// rect is a screen rectangle: x and y are the zero-based coordinates of its
// top-left cell, w and h its size in cells.
type rect struct{ x, y, w, h int }

// has reports whether the cell (x, y) is inside r.
func (r rect) has(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

// listRect is the group list's rectangle on screen.
func (m *Model) listRect() rect {
	if m.wide() {
		return rect{0, 1, m.listWidth(), m.height - 2}
	}
	return rect{0, 1, m.width, m.listHeight()}
}

// diffRect is the diff pane's rectangle on screen. It mirrors the layout of
// renderBody: the pane starts under the header, right of the list and its
// separator when wide, below the list and its rule when stacked.
func (m *Model) diffRect() rect {
	bodyH := m.height - 2 // header + footer
	if m.wide() {
		return rect{m.listWidth() + 1, 1, m.diffWidth(), bodyH}
	}
	lh := m.listHeight()
	return rect{0, lh + 2, m.width, bodyH - lh - 1}
}

// onMouse routes a mouse event: the wheel scrolls the pane under the pointer
// and a left click puts the cursor on what was clicked.
func (m *Model) onMouse(msg tea.MouseMsg) {
	if m.doc == nil || m.ov != ovNone || m.width <= 0 || m.height <= 0 {
		return
	}
	ev := msg.Mouse()
	switch msg.(type) {
	case tea.MouseWheelMsg:
		switch ev.Button {
		case tea.MouseWheelUp:
			m.wheel(-wheelStep, ev.X, ev.Y)
		case tea.MouseWheelDown:
			m.wheel(wheelStep, ev.X, ev.Y)
		}
	case tea.MouseClickMsg:
		if ev.Button == tea.MouseLeft {
			m.click(ev.X, ev.Y)
		}
	}
}

// wheel scrolls by delta lines: the diff pane under the pointer, else the
// group list (one group per notch).
func (m *Model) wheel(delta, x, y int) {
	switch {
	case m.diffRect().has(x, y):
		m.scrollDiff(delta)
	case m.listRect().has(x, y) && len(m.groups()) > 0:
		step := 1
		if delta < 0 {
			step = -1
		}
		m.setCursor(m.cursor + step)
	}
}

// click moves the cursor to what was clicked: a diff line, or a group row.
func (m *Model) click(x, y int) {
	switch {
	case m.diffRect().has(x, y):
		if len(m.ensureLines()) == 0 {
			return
		}
		m.focus = focusDiff
		m.setDcur(m.scroll + y - m.diffRect().y)
	case m.listRect().has(x, y) && len(m.groups()) > 0:
		start, _ := listview.Window(len(m.groups()), m.cursor, m.listRect().h)
		if m.mode == modeGrouped {
			m.focus = focusList
		}
		m.setCursor(start + y - m.listRect().y)
	}
}

// scrollDiff scrolls the diff pane by delta lines without moving the cursor
// off the window: a cursor the scroll would leave behind sticks to the nearest
// edge, so the arrow keys carry on from there inside the window. It scrolls
// nothing when the whole diff already fits.
func (m *Model) scrollDiff(delta int) {
	lines := m.ensureLines()
	if len(lines) == 0 {
		return
	}
	maxScroll := max(0, len(lines)-m.bodyHeight())
	m.scroll = max(0, min(m.scroll, maxScroll)) // a rebuild may leave it past the end
	m.scroll = max(0, min(m.scroll+delta, maxScroll))
	m.pinDcur()
}

// pinDcur keeps the diff cursor inside the visible window, so the cursor is
// never scrolled out of sight.
func (m *Model) pinDcur() {
	h := m.bodyHeight()
	switch {
	case m.dcur < m.scroll:
		m.dcur = m.scroll // the window scrolled past it: stick to the top
	case m.dcur >= m.scroll+h:
		m.dcur = m.scroll + h - 1 // the window scrolled back under it: stick to the bottom
	default:
		return
	}
	m.syncGroupCursor()
}
