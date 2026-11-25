// Package listview renders clump's group list.
package listview

import "github.com/k22i/clump/internal/tui/view"

// Window returns the half-open range [start, end) of n rows to show in
// height lines so that cursor is visible.
func Window(n, cursor, height int) (start, end int) {
	panic("listview: Window not implemented")
}

// Render renders the visible rows, one per line, joined with "\n" (no
// trailing newline).
func Render(rows []view.ListRow, cursor, width, height int, th view.Theme) string {
	panic("listview: Render not implemented")
}
