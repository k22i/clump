package view

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/gitdiff"
)

func RenderFileHeader(f *gitdiff.File, fileIdx, add, del int, th Theme, width int) Line {
	return RenderFileHeaderState(f, fileIdx, add, del, nil, FileState{}, th, width)
}

func RenderFileHeaderState(f *gitdiff.File, fileIdx, add, del int, hunks []string, st FileState, th Theme, width int) Line {
	var b strings.Builder
	b.WriteString("── ")
	if st.Reviewed {
		b.WriteString("✓ ")
	}
	fmt.Fprintf(&b, "%s (%s +%d −%d) ", f.DisplayPath(), f.Status, add, del)
	if st.Folded {
		n := len(hunks)
		unit := "hunks"
		if n == 1 {
			unit = "hunk"
		}
		fmt.Fprintf(&b, "· %d %s folded ", n, unit)
	}
	if st.Generated != "" {
		fmt.Fprintf(&b, "· generated (%s) ", st.Generated)
	}
	text := b.String()
	if w := lipgloss.Width(text); w < width {
		text += strings.Repeat("─", width-w)
	}
	text = Truncate(text, width)
	style := th.FileHeader
	if st.Reviewed || st.Generated != "" {
		style = th.Dim
	}
	return Line{Text: th.style(style, text), Plain: text, Kind: KindFile, FileIdx: fileIdx,
		BlockHunks: hunks, Folded: st.Folded}
}

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

func (t Theme) style(st lipgloss.Style, s string) string {
	if t.Plain {
		return s
	}
	return st.Render(s)
}

func (t Theme) Style(st lipgloss.Style, s string) string { return t.style(st, s) }
