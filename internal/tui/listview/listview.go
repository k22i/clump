// Package listview renders clump's group list.
package listview

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/tui/view"
)

// cursorMark starts the cursor row; every other row is indented by its width.
const cursorMark = "▸ "

// titleGap is the least space between a row's title and its counts, so the
// two never read as one run of text.
const titleGap = 2

// Window returns the half-open range [start, end) of n rows to show in
// height lines so that cursor is visible.
//
// When all rows fit it is (0, n); otherwise the window is centred on cursor
// and clamped to the last page. A height of zero or less shows nothing.
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

// Render renders the visible rows, one per line, joined with "\n" (no
// trailing newline). No visible rows render "".
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

// NaturalWidth is the width rows need to show every title whole beside its
// counts: the widest row, but never less than minWidth. The caller adds the
// padding; marks ("x2") are left out so marking a hunk does not resize the
// list under the diff pane.
func NaturalWidth(rows []view.ListRow, minWidth int) int {
	w := max(minWidth, lipgloss.Width(cursorMark))
	for _, r := range rows {
		w = max(w, lipgloss.Width(cursorMark)+lipgloss.Width(r.Title)+titleGap+lipgloss.Width(counts(r)))
	}
	return w
}

// counts is a row's right-hand column: the added and deleted totals and the
// hunk count, or the review progress while the group is being reviewed.
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

// renderRow renders one list row in at most width cells: the cursor mark,
// the title, then the " +a \u2212d Nh" counts (and " xN" markers).
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

	// A reviewed group reads as done: dim, like Ungrouped.
	dim := r.Ungrouped || done

	// Everything up to the counts fits: pad the title to the free room so the
	// counts are flush right and the row is exactly width cells.
	if room := width - lipgloss.Width(prefix) - lipgloss.Width(right); room >= 1 {
		title := view.Truncate(r.Title, max(1, room-titleGap))
		if pad := room - lipgloss.Width(title); pad > 0 {
			title += strings.Repeat(" ", pad)
		}
		return styleRow(prefix, title, right, dim, isCursor, th)
	}

	// Too narrow for the counts: cut the whole row, keeping each part intact
	// so it can still be styled on its own.
	p, title, rl := cutRow(prefix, r.Title, right, width)
	return styleRow(p, title, rl, dim, isCursor, th)
}

// cutRow splits prefix+title+right at width cells the way
// view.Truncate(prefix+title+right, width) cuts it, returning the surviving
// part of each field.
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

// styleRow styles the three parts of a row: the cursor row as a whole in
// th.Selected; other rows with the title dim when the row is muted (the
// Ungrouped or Generated files group, or a fully reviewed one; unstyled
// otherwise) and the counts always dim. The plain text of the
// result is exactly prefix+title+right.
func styleRow(prefix, title, right string, ungrouped, isCursor bool, th view.Theme) string {
	if isCursor {
		return th.Style(th.Selected, prefix+title+right)
	}
	if ungrouped {
		return prefix + th.Style(th.Dim, title) + th.Style(th.Dim, right)
	}
	return prefix + title + th.Style(th.Dim, right)
}
