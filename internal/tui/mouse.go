package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/tui/listview"
)

const wheelStep = 3

type rect struct{ x, y, w, h int }

func (r rect) has(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

func (m *Model) listRect() rect {
	if m.wide() {
		return rect{0, 2, m.listWidth(), m.bodyHeight()}
	}
	return rect{0, 2, m.width, m.listHeight()}
}

func (m *Model) diffRect() rect {
	bodyH := m.bodyHeight()
	if m.wide() {
		return rect{m.listWidth() + 1, 2, m.diffWidth(), bodyH}
	}
	lh := m.listHeight()
	return rect{0, lh + 3, m.width, bodyH - lh - 1}
}

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

func (m *Model) scrollDiff(delta int) {
	lines := m.ensureLines()
	if len(lines) == 0 {
		return
	}
	maxScroll := max(0, len(lines)-m.bodyHeight())
	m.scroll = max(0, min(m.scroll, maxScroll))
	m.scroll = max(0, min(m.scroll+delta, maxScroll))
	m.pinDcur()
}

func (m *Model) pinDcur() {
	h := m.bodyHeight()
	switch {
	case m.dcur < m.scroll:
		m.dcur = m.scroll
	case m.dcur >= m.scroll+h:
		m.dcur = m.scroll + h - 1
	default:
		return
	}
	m.syncGroupCursor()
}
