package tui

import (
	tea "charm.land/bubbletea/v2"
)

type confirmKind int

const (
	confirmSend    confirmKind = iota // y sends the job above the ceiling
	confirmRegroup                    // d discard / s snapshot / esc
)

type confirmState struct {
	question string
	kind     confirmKind
}

func (m *Model) askConfirm(q string, k confirmKind) {
	m.confirm = confirmState{question: q, kind: k}
	m.ov = ovConfirm
}

func (m *Model) keyConfirm(key string) tea.Cmd {
	switch m.confirm.kind {
	case confirmSend:
		m.ov = ovNone
		if key == "y" || key == "Y" {
			return m.groupNow()
		}
		if m.doc != nil && m.doc.job != nil {
			m.setFlash("not sent — press g to group ("+m.doc.job.Est.String()+")", false)
		}
		return nil
	case confirmRegroup:
		switch key {
		case "d", "D":
			m.ov = ovNone
			return m.doRegroup(false)
		case "s", "S":
			m.ov = ovNone
			return m.doRegroup(true)
		case "esc", "n", "q":
			m.ov = ovNone
			m.setFlash("regroup cancelled", false)
		}
	}
	return nil
}

type targetAct int

const (
	actMove targetAct = iota
	actMerge
)

type targetOpt struct {
	id    string // "" = new group
	label string
}

type targetState struct {
	title  string
	opts   []targetOpt
	cursor int
	act    targetAct
	hunks  []string
	src    string
}

func (m *Model) keyTarget(key string) tea.Cmd {
	t := &m.target
	switch key {
	case "j", "down", "ctrl+n":
		t.cursor = min(t.cursor+1, len(t.opts)-1)
	case "k", "up", "ctrl+p":
		t.cursor = max(t.cursor-1, 0)
	case "esc", "q":
		m.ov = ovNone
	case "enter":
		m.ov = ovNone
		if t.cursor >= 0 && t.cursor < len(t.opts) {
			return m.finishTarget(t.opts[t.cursor])
		}
	default:
		// 1-9 pick a group by number.
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			n := int(key[0] - '0')
			for i, o := range t.opts {
				if o.id != "" && len(o.label) > 0 && labelNumber(o.label) == n {
					m.ov = ovNone
					_ = i
					return m.finishTarget(o)
				}
			}
		}
	}
	return nil
}

func labelNumber(label string) int {
	n := 0
	for _, c := range label {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

type inputAct int

const (
	inputNew inputAct = iota
	inputRename
	inputMoveNew
	inputComment // in.id holds the comment's anchor key
)

type inputState struct {
	act   inputAct
	title lineInput
	desc  lineInput
	field int // 0 title, 1 description (rename only)
	id    string
	hunks []string
}

func (s *inputState) active() *lineInput {
	if s.field == 1 {
		return &s.desc
	}
	return &s.title
}

func (m *Model) openInput(act inputAct, prompt, title, desc, id string, hunks []string) {
	m.input = inputState{act: act, title: lineInput{prompt: prompt}, desc: lineInput{prompt: "Explanation: "}, id: id, hunks: hunks}
	m.input.title.set(title)
	m.input.desc.set(desc)
	m.ov = ovInput
}

func (m *Model) keyInput(k tea.KeyPressMsg) tea.Cmd {
	in := &m.input
	if k.String() == "tab" || k.String() == "shift+tab" {
		if in.act == inputRename {
			in.field = 1 - in.field
		}
		return nil
	}
	done, cancel := in.active().key(k)
	switch {
	case cancel:
		m.ov = ovNone
	case done:
		if in.act == inputRename && in.field == 0 {
			in.field = 1 // enter on the title moves to the explanation
			return nil
		}
		m.ov = ovNone
		return m.finishInput()
	}
	return nil
}

type menuItem struct {
	name, label string
}

type menuState struct {
	items  []menuItem
	cursor int
}

func (m *Model) keyMenu(key string) tea.Cmd {
	s := &m.menu
	switch key {
	case "j", "down":
		s.cursor = min(s.cursor+1, len(s.items)-1)
	case "k", "up":
		s.cursor = max(s.cursor-1, 0)
	case "esc", "q", "m":
		m.ov = ovNone
	case "enter":
		m.ov = ovNone
		if s.cursor >= 0 && s.cursor < len(s.items) {
			return m.switchProvider(s.items[s.cursor].name)
		}
	}
	return nil
}
