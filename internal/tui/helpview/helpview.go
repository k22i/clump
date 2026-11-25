// Package helpview renders clump's key help overlay and implements search
// over rendered lines (/, n, N).
package helpview

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/tui/view"
)

// helpTitle is the overlay's first line.
const helpTitle = "clump keys (? or esc to close)"

// moreMarker replaces the tail of the overlay when it does not fit.
const moreMarker = "… (more)"

// columnSep separates the two columns.
const columnSep = " │ "

// twoColumnMinWidth is the narrowest width at which Render splits the overlay
// into two columns.
const twoColumnMinWidth = 80

// Render renders the help overlay in at most height lines of at most width
// cells.
func Render(sections []view.HelpSection, width, height int, th view.Theme) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	blocks := make([][]hline, len(sections))
	for i, sec := range sections {
		blocks[i] = sectionLines(sec, th)
	}
	head := []hline{{{text: helpTitle, st: &th.Title}}, {}}
	lines := concat(head, blocks)
	if len(lines) > height && width >= twoColumnMinWidth {
		if left, right, ok := splitColumns(head, blocks); ok {
			lines = joinColumns(left, right, (width-3)/2)
		}
	}
	if len(lines) > height {
		lines = append(lines[:height-1:height-1], hline{{text: moreMarker}})
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.truncate(width).render(th)
	}
	return strings.Join(out, "\n")
}

// FindMatches returns the indexes of lines whose Plain text contains query
// (smart case: case-insensitive unless query has an upper-case letter).
func FindMatches(lines []view.Line, query string) []int {
	if query == "" {
		return nil
	}
	fold := !hasUpper(query)
	needle := query
	if fold {
		needle = strings.ToLower(query)
	}
	var out []int
	for i, l := range lines {
		hay := l.Plain
		if fold {
			hay = strings.ToLower(hay)
		}
		if strings.Contains(hay, needle) {
			out = append(out, i)
		}
	}
	return out
}

// NextMatch returns the first match index strictly after (forward) or before
// (backward) line from, wrapping around; -1 when matches is empty.
func NextMatch(matches []int, from int, forward bool) int {
	if len(matches) == 0 {
		return -1
	}
	if forward {
		for _, m := range matches {
			if m > from {
				return m
			}
		}
		return matches[0]
	}
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i] < from {
			return matches[i]
		}
	}
	return matches[len(matches)-1]
}

// Highlight returns l's Plain text with every occurrence of query wrapped in
// th.Match (smart case as in FindMatches). Under a plain theme it returns
// l.Plain.
func Highlight(l view.Line, query string, th view.Theme) string {
	if th.Plain || query == "" {
		return l.Plain
	}
	hay := []rune(l.Plain)
	needle := []rune(query)
	if len(needle) == 0 || len(needle) > len(hay) {
		return l.Plain
	}
	fold := !hasUpper(query)
	var b strings.Builder
	for i := 0; i < len(hay); {
		if matchAt(hay, needle, i, fold) {
			b.WriteString(th.Match.Render(string(hay[i : i+len(needle)])))
			i += len(needle)
			continue
		}
		b.WriteRune(hay[i])
		i++
	}
	return b.String()
}

// hasUpper reports whether s has an upper-case letter: clump's smart-case
// rule makes a query with one case-sensitive.
func hasUpper(s string) bool { return strings.ContainsFunc(s, unicode.IsUpper) }

// matchAt reports whether needle matches hay at rune index i, ignoring case
// when fold is set.
func matchAt(hay, needle []rune, i int, fold bool) bool {
	if i < 0 || i+len(needle) > len(hay) {
		return false
	}
	for j, r := range needle {
		h := hay[i+j]
		if fold {
			h, r = unicode.ToLower(h), unicode.ToLower(r)
		}
		if h != r {
			return false
		}
	}
	return true
}

// seg is a run of text inside a help line and the style it is drawn with; a
// nil style means unstyled.
type seg struct {
	text string
	st   *lipgloss.Style
}

// hline is one help line, split into segments so it can be cut and padded by
// cell count without breaking an SGR sequence. The concatenation of its
// segments' texts is the line's plain form.
type hline []seg

// plain returns l's text without styling.
func (l hline) plain() string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.text)
	}
	return b.String()
}

// width is l's width in terminal cells.
func (l hline) width() int { return lipgloss.Width(l.plain()) }

// render draws l with th, which draws nothing under a plain theme.
func (l hline) render(th view.Theme) string {
	if th.Plain {
		return l.plain()
	}
	var b strings.Builder
	for _, s := range l {
		if s.st == nil {
			b.WriteString(s.text)
			continue
		}
		b.WriteString(s.st.Render(s.text))
	}
	return b.String()
}

// truncate cuts l to at most width cells, ending in "…" when it had to cut,
// exactly as view.Truncate cuts plain text.
func (l hline) truncate(width int) hline {
	if width <= 0 || l.width() <= width {
		return l
	}
	out := make(hline, 0, len(l)+1)
	cells := 0
	cut := false
	for _, s := range l {
		if cut {
			break
		}
		var b strings.Builder
		for _, r := range s.text {
			rw := lipgloss.Width(string(r))
			if cells+rw > width-1 {
				cut = true
				break
			}
			b.WriteRune(r)
			cells += rw
		}
		if b.Len() > 0 {
			out = append(out, seg{text: b.String(), st: s.st})
		}
	}
	return append(out, seg{text: "…"})
}

// pad returns l followed by spaces up to width cells; l is returned unchanged
// when it is already that wide.
func (l hline) pad(width int) hline {
	n := width - l.width()
	if n <= 0 {
		return l
	}
	out := make(hline, len(l), len(l)+1)
	copy(out, l)
	return append(out, seg{text: strings.Repeat(" ", n)})
}

// sectionLines renders one help section: its title, then one line per entry
// with Keys padded to the section's widest Keys.
func sectionLines(s view.HelpSection, th view.Theme) []hline {
	kw := 0
	for _, e := range s.Entries {
		kw = max(kw, lipgloss.Width(e.Keys))
	}
	out := make([]hline, 0, 1+len(s.Entries))
	out = append(out, hline{{text: s.Title, st: &th.Title}})
	for _, e := range s.Entries {
		pad := strings.Repeat(" ", kw-lipgloss.Width(e.Keys))
		out = append(out, hline{
			{text: "  "},
			{text: e.Keys, st: &th.Key},
			{text: pad + "  " + e.Desc},
		})
	}
	return out
}

// concat lays head out above blocks with an empty line between sections.
func concat(head []hline, blocks [][]hline) []hline {
	out := make([]hline, 0, len(head)+2*len(blocks))
	out = append(out, head...)
	for i, b := range blocks {
		out = append(out, b...)
		if i < len(blocks)-1 {
			out = append(out, hline{})
		}
	}
	return out
}

// splitColumns splits the sections at the boundary that makes the two
// columns' line counts closest, preferring the earlier boundary on a tie; ok
// is false when there is no boundary to split at.
func splitColumns(head []hline, blocks [][]hline) (left, right []hline, ok bool) {
	if len(blocks) < 2 {
		return nil, nil, false
	}
	best, bestDiff := 0, 0
	for k := 1; k < len(blocks); k++ {
		d := len(concat(head, blocks[:k])) - len(concat(nil, blocks[k:]))
		if d < 0 {
			d = -d
		}
		if best == 0 || d < bestDiff {
			best, bestDiff = k, d
		}
	}
	return concat(head, blocks[:best]), concat(nil, blocks[best:]), true
}

// joinColumns joins the two columns with columnSep: each line of either column
// is cut to colw cells, the left one padded to colw cells, and the shorter
// column padded with empty lines.
func joinColumns(left, right []hline, colw int) []hline {
	out := make([]hline, 0, max(len(left), len(right)))
	for i := 0; i < max(len(left), len(right)); i++ {
		var l, r hline
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		line := make(hline, 0, len(l)+1+len(r))
		line = append(line, l.truncate(colw).pad(colw)...)
		line = append(line, seg{text: columnSep})
		line = append(line, r.truncate(colw)...)
		out = append(out, line)
	}
	return out
}
