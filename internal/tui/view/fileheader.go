package view

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/gitdiff"
)

// RenderFileHeader renders the line above a file's hunks:
//
//	── src/a.go (M +3 −1) ─────────
//
// padded with "─" to width (never shorter than the text itself; cut with
// "…" when the text is wider than width). add and del are the totals of the
// hunks shown under it.
func RenderFileHeader(f *gitdiff.File, fileIdx, add, del int, th Theme, width int) Line {
	text := fmt.Sprintf("── %s (%s +%d −%d) ", f.DisplayPath(), f.Status, add, del)
	if w := lipgloss.Width(text); w < width {
		text += strings.Repeat("─", width-w)
	}
	text = Truncate(text, width)
	return Line{Text: th.style(th.FileHeader, text), Plain: text, Kind: KindFile, FileIdx: fileIdx}
}

// Truncate cuts plain (unstyled) text to at most width cells, ending in "…"
// when it had to cut. width <= 0 means no limit.
func Truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > width-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteString("…")
	return b.String()
}

// style renders s with st unless the theme is plain.
func (t Theme) style(st lipgloss.Style, s string) string {
	if t.Plain {
		return s
	}
	return st.Render(s)
}

// Style is the exported form of style for render helpers in this package's
// tests and callers: it renders s with st unless the theme is plain.
func (t Theme) Style(st lipgloss.Style, s string) string { return t.style(st, s) }
