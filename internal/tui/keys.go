package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/tui/helpview"
	"github.com/k22i/clump/internal/tui/view"
)

// HelpSections is the key reference shown by ? (and in the README).
func HelpSections(watch bool) []view.HelpSection {
	s := []view.HelpSection{
		{Title: "Navigate", Entries: []view.HelpEntry{
			{Keys: "j/k ↓/↑", Desc: "next/previous group (lines in the diff pane)"},
			{Keys: "enter l →", Desc: "focus the diff pane"},
			{Keys: "esc h ←", Desc: "back to the group list"},
			{Keys: "]/[", Desc: "next/previous group from anywhere"},
			{Keys: "}/{", Desc: "next/previous hunk (diff pane)"},
			{Keys: "ctrl+d/ctrl+u", Desc: "half page down/up"},
			{Keys: "a", Desc: "toggle grouped / all hunks in file order"},
		}},
		{Title: "Review", Entries: []view.HelpEntry{
			{Keys: "c", Desc: "mark the file (diff pane) or group (list) reviewed; then on to the next unreviewed group"},
			{Keys: "z", Desc: "fold/unfold the file (diff pane) or every file of the group (list)"},
			{Keys: "Z", Desc: "fold/unfold every file in the pane"},
			{Keys: "#", Desc: "line numbers on/off"},
		}},
		{Title: "Search & tools", Entries: []view.HelpEntry{
			{Keys: "/", Desc: "search the diff pane"},
			{Keys: "n/N", Desc: "next/previous match (while searching)"},
			{Keys: "e", Desc: "open $EDITOR at the hunk"},
			{Keys: "y", Desc: "copy a Markdown summary"},
			{Keys: "g", Desc: "group now (when waiting to send)"},
			{Keys: "r", Desc: "regroup (bypass the cache)"},
			{Keys: "m", Desc: "switch provider"},
			{Keys: "?", Desc: "this help"},
			{Keys: "q ctrl+c", Desc: "quit"},
		}},
		{Title: "Edit groups", Entries: []view.HelpEntry{
			{Keys: "x", Desc: "mark the hunk (the whole group in the list)"},
			{Keys: "v", Desc: "visual mode: mark a range of hunks"},
			{Keys: "M", Desc: "move marked hunks to a group or a new one"},
			{Keys: "n", Desc: "new empty group"},
			{Keys: "R", Desc: "rename group and edit its explanation"},
			{Keys: "d", Desc: "delete group (hunks go to Ungrouped)"},
			{Keys: "J/K", Desc: "move group down/up"},
			{Keys: "g", Desc: "merge this group into another"},
			{Keys: "u/ctrl+r", Desc: "undo/redo"},
		}},
	}
	if watch {
		s = append(s, view.HelpSection{Title: "Watch", Entries: []view.HelpEntry{
			{Keys: "A", Desc: "toggle auto-group"},
		}})
	}
	return s
}

// onKey dispatches a key press: overlays first, then global keys, then the
// focused pane.
func (m *Model) onKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return m.quit()
	}
	switch m.ov {
	case ovHelp:
		if key == "?" || key == "esc" || key == "q" {
			m.ov = ovNone
		}
		return nil
	case ovSearch:
		return m.keySearch(k)
	case ovConfirm:
		return m.keyConfirm(key)
	case ovTarget:
		return m.keyTarget(key)
	case ovInput:
		return m.keyInput(k)
	case ovProvider:
		return m.keyMenu(key)
	case ovPick:
		return m.keyPick(k)
	}
	if m.doc == nil {
		if key == "q" {
			return m.quit()
		}
		return nil
	}
	m.flash = ""
	switch key {
	case "q":
		return m.quit()
	case "?":
		m.ov = ovHelp
		return nil
	case "]":
		m.moveGroup(1)
		return nil
	case "[":
		m.moveGroup(-1)
		return nil
	case "a":
		m.toggleAll()
		return nil
	case "/":
		m.ov = ovSearch
		m.searchBox.set(m.query)
		return nil
	case "n":
		if m.query != "" {
			m.jumpMatch(true)
			return nil
		}
		return m.startNewGroup()
	case "N":
		if m.query != "" {
			m.jumpMatch(false)
		}
		return nil
	case "e":
		return m.openEditor()
	case "y":
		return m.copySummary()
	case "r":
		return m.regroup()
	case "m":
		m.openProviderMenu()
		return nil
	case "x":
		m.toggleMark()
		return nil
	case "c":
		m.toggleReviewed()
		return nil
	case "z":
		m.toggleFold()
		return nil
	case "Z":
		m.foldAll()
		return nil
	case "#":
		m.toggleLineNumbers()
		return nil
	case "v":
		m.toggleVisual()
		return nil
	case "M":
		return m.startMove()
	case "R":
		return m.startRename()
	case "d":
		return m.deleteGroup()
	case "J":
		return m.reorder(1)
	case "K":
		return m.reorder(-1)
	case "g":
		if m.doc != nil && m.doc.state == stNeedsConfirm {
			return m.groupNow()
		}
		return m.startMerge()
	case "u":
		return m.undo()
	case "ctrl+r":
		return m.redo()
	case "A":
		if m.watch != nil {
			return m.watch.toggleAuto(m)
		}
		return nil
	case "esc":
		if m.visual {
			m.visual = false
			return nil
		}
		if m.query != "" {
			m.query, m.matches = "", nil
			return nil
		}
		m.focus = focusList
		return nil
	}
	if m.focus == focusDiff {
		m.keyDiff(key)
	} else {
		m.keyList(key)
	}
	return nil
}

func (m *Model) quit() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	if m.watch != nil {
		m.watch.close()
	}
	return tea.Sequence(m.status("idle", ""), tea.Quit)
}

func (m *Model) keyList(key string) {
	n := len(m.groups())
	switch key {
	case "j", "down":
		m.setCursor(m.cursor + 1)
	case "k", "up":
		m.setCursor(m.cursor - 1)
	case "home":
		m.setCursor(0)
	case "end":
		m.setCursor(n - 1)
	case "enter", "l", "right", "tab":
		m.focus = focusDiff
	}
}

func (m *Model) keyDiff(key string) {
	lines := m.ensureLines()
	page := max(1, m.bodyHeight()-1)
	switch key {
	case "j", "down":
		m.setDcur(m.dcur + 1)
	case "k", "up":
		m.setDcur(m.dcur - 1)
	case "ctrl+d":
		m.setDcur(m.dcur + page/2)
	case "ctrl+u":
		m.setDcur(m.dcur - page/2)
	case "pgdown", "space", "ctrl+f":
		m.setDcur(m.dcur + page)
	case "pgup", "ctrl+b":
		m.setDcur(m.dcur - page)
	case "home":
		m.setDcur(0)
	case "end":
		m.setDcur(len(lines) - 1)
	case "}":
		for i := m.dcur + 1; i < len(lines); i++ {
			if lines[i].Kind == view.KindHunk || lines[i].Folded {
				m.setDcur(i)
				return
			}
		}
	case "{":
		for i := m.dcur - 1; i >= 0; i-- {
			if lines[i].Kind == view.KindHunk || lines[i].Folded {
				m.setDcur(i)
				return
			}
		}
	case "h", "left", "shift+tab":
		if m.mode == modeGrouped {
			m.focus = focusList
		}
	}
}

func (m *Model) keySearch(k tea.KeyPressMsg) tea.Cmd {
	done, cancel := m.searchBox.key(k)
	switch {
	case cancel:
		m.ov = ovNone
	case done:
		m.ov = ovNone
		m.query = m.searchBox.text()
		m.matches = helpview.FindMatches(m.ensureLines(), m.query)
		if m.query != "" {
			if len(m.matches) == 0 {
				m.setFlash("no match for "+m.query, true)
			} else {
				m.focus = focusDiff
				m.jumpMatchFrom(m.dcur-1, true)
			}
		}
	}
	return nil
}

func (m *Model) jumpMatch(forward bool) { m.jumpMatchFrom(m.dcur, forward) }

func (m *Model) jumpMatchFrom(from int, forward bool) {
	m.matches = helpview.FindMatches(m.ensureLines(), m.query)
	i := helpview.NextMatch(m.matches, from, forward)
	if i < 0 {
		m.setFlash("no match for "+m.query, true)
		return
	}
	m.focus = focusDiff
	m.setDcur(i)
}

// forwardInput sends pasted text to the active input.
func (m *Model) forwardInput(p tea.PasteMsg) tea.Cmd {
	switch m.ov {
	case ovSearch:
		m.searchBox.insert(p.Content)
	case ovInput:
		m.input.active().insert(p.Content)
	}
	return nil
}
