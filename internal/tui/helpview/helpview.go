// Package helpview renders clump's key help overlay and implements search
// over rendered lines (/, n, N).
package helpview

import "github.com/k22i/clump/internal/tui/view"

// Render renders the help overlay in at most height lines of at most width
// cells.
func Render(sections []view.HelpSection, width, height int, th view.Theme) string {
	panic("helpview: Render not implemented")
}

// FindMatches returns the indexes of lines whose Plain text contains query
// (smart case: case-insensitive unless query has an upper-case letter).
func FindMatches(lines []view.Line, query string) []int {
	panic("helpview: FindMatches not implemented")
}

// NextMatch returns the first match index strictly after (forward) or before
// (backward) line from, wrapping around; -1 when matches is empty.
func NextMatch(matches []int, from int, forward bool) int {
	panic("helpview: NextMatch not implemented")
}

// Highlight returns l's Plain text with every occurrence of query wrapped in
// th.Match (smart case as in FindMatches). Under a plain theme it returns
// l.Plain.
func Highlight(l view.Line, query string, th view.Theme) string {
	panic("helpview: Highlight not implemented")
}
