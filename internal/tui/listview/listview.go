package listview

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/tui/view"
)

const cursorMark = "▸ "

const titleGap = 2

func Window(n, cursor, height int) (start, end int) {
	if height <= 0 || n <= 0 {
		return 0, 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > n-1 {
		cursor = n - 1
	}
	if n <= height {
		return 0, n
	}
	start = cursor - height/2
	if start < 0 {
		start = 0
	}
	if maxStart := n - height; start > maxStart {
		start = maxStart
	}
	return start, start + height
}

func Render(rows []view.ListRow, cursor, width, height int, th view.Theme) string {
	start, end := Window(len(rows), cursor, height)
	if start >= end {
		return ""
	}
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, renderRow(rows[i], i == cursor, width, th))
	}
	return strings.Join(lines, "\n")
}

func NaturalWidth(rows []view.ListRow, minWidth int) int {
	w := max(minWidth, lipgloss.Width(cursorMark))
	for _, r := range rows {
		w = max(w, lipgloss.Width(cursorMark)+lipgloss.Width(r.Title)+titleGap+lipgloss.Width(counts(r)))
	}
	return w
}

func counts(r view.ListRow) string {
	right := fmt.Sprintf(" +%d \u2212%d %dh", r.Add, r.Del, r.Hunks)
	switch {
	case r.Hunks > 0 && r.Reviewed >= r.Hunks:
		right += " \u2713"
	case r.Reviewed > 0:
		right = fmt.Sprintf(" +%d \u2212%d %d/%dh", r.Add, r.Del, r.Reviewed, r.Hunks)
	}
	return right
}

func renderRow(r view.ListRow, isCursor bool, width int, th view.Theme) string {
	if width <= 0 {
		return ""
	}
	prefix := "  "
	if isCursor {
		prefix = cursorMark
	}
	right := counts(r)
	done := r.Hunks > 0 && r.Reviewed >= r.Hunks
	if r.Marks > 0 {
		right += fmt.Sprintf(" x%d", r.Marks)
	}

	dim := r.Ungrouped || done

	if room := width - lipgloss.Width(prefix) - lipgloss.Width(right); room >= 1 {
		title := view.Truncate(r.Title, max(1, room-titleGap))
		if pad := room - lipgloss.Width(title); pad > 0 {
			title += strings.Repeat(" ", pad)
		}
		return styleRow(prefix, title, right, dim, isCursor, th)
	}

	p, title, rl := cutRow(prefix, r.Title, right, width)
	return styleRow(p, title, rl, dim, isCursor, th)
}

func cutRow(prefix, title, right string, width int) (p, t, r string) {
	if width <= 0 {
		return "", "", ""
	}
	p = view.Truncate(prefix, width)
	if p != prefix {
		return p, "", ""
	}
	rest := width - lipgloss.Width(p)
	if rest <= 0 {
		return p, "", ""
	}
	t = view.Truncate(title, rest)
	if t != title {
		return p, t, ""
	}
	rest -= lipgloss.Width(t)
	if rest <= 0 {
		return p, t, ""
	}
	return p, t, view.Truncate(right, rest)
}

func styleRow(prefix, title, right string, ungrouped, isCursor bool, th view.Theme) string {
	if isCursor {
		return th.Style(th.Selected, prefix+title+right)
	}
	if ungrouped {
		return prefix + th.Style(th.Dim, title) + th.Style(th.Dim, right)
	}
	return prefix + title + th.Style(th.Dim, right)
}
