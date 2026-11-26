package tui

import (
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/tui/view"
)

type lineInput struct {
	prompt string
	value  []rune
	pos    int
}

func (l *lineInput) set(s string) { l.value, l.pos = []rune(s), len([]rune(s)) }
func (l *lineInput) text() string { return string(l.value) }

func (l *lineInput) key(k tea.KeyPressMsg) (done, cancel bool) {
	switch k.String() {
	case "enter":
		return true, false
	case "esc", "ctrl+c":
		return false, true
	case "backspace", "ctrl+h":
		if l.pos > 0 {
			l.value = append(l.value[:l.pos-1], l.value[l.pos:]...)
			l.pos--
		}
	case "delete", "ctrl+d":
		if l.pos < len(l.value) {
			l.value = append(l.value[:l.pos], l.value[l.pos+1:]...)
		}
	case "left", "ctrl+b":
		if l.pos > 0 {
			l.pos--
		}
	case "right", "ctrl+f":
		if l.pos < len(l.value) {
			l.pos++
		}
	case "home", "ctrl+a":
		l.pos = 0
	case "end", "ctrl+e":
		l.pos = len(l.value)
	case "ctrl+u":
		l.value, l.pos = l.value[l.pos:], 0
	case "ctrl+w":
		i := l.pos
		for i > 0 && unicode.IsSpace(l.value[i-1]) {
			i--
		}
		for i > 0 && !unicode.IsSpace(l.value[i-1]) {
			i--
		}
		l.value, l.pos = append(l.value[:i], l.value[l.pos:]...), i
	default:
		if t := k.Text; t != "" {
			l.insert(t)
		}
	}
	return false, false
}

func (l *lineInput) insert(t string) {
	var rs []rune
	for _, r := range t {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if unicode.IsControl(r) {
			continue
		}
		rs = append(rs, r)
	}
	out := make([]rune, 0, len(l.value)+len(rs))
	out = append(out, l.value[:l.pos]...)
	out = append(out, rs...)
	out = append(out, l.value[l.pos:]...)
	l.value, l.pos = out, l.pos+len(rs)
}

func (l *lineInput) view(width int, th view.Theme, active bool) string {
	before := string(l.value[:l.pos])
	cur := " "
	after := ""
	if l.pos < len(l.value) {
		cur = string(l.value[l.pos])
		after = string(l.value[l.pos+1:])
	}
	if !active {
		return view.Truncate(l.prompt+string(l.value), width)
	}
	plain := l.prompt + before + cur + after
	if view.Truncate(plain, width) != plain {
		tail := []rune(before)
		for len(tail) > 0 && len([]rune(l.prompt))+len(tail)+2 > width {
			tail = tail[1:]
		}
		before = string(tail)
		after = ""
	}
	return l.prompt + before + th.Style(th.Selected, cur) + view.Truncate(after, max(0, width-len([]rune(l.prompt+before))-1))
}
