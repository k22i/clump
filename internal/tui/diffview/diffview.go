// Package diffview renders hunks for clump's diff pane: syntax highlighting
// with Chroma by file extension, and word-level highlighting of paired -/+
// lines.
package diffview

import (
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui/view"
)

// HunkOpts configures RenderHunk.
type HunkOpts struct {
	// Width caps every line (<= 0 means no cap).
	Width int
	// Marked draws the hunk's gutter as marked ("x ").
	Marked bool
	// Path picks the syntax lexer by file name ("" disables highlighting).
	Path string
}

// RenderHunk renders one hunk: its header line, then its body lines.
func RenderHunk(h *gitdiff.Hunk, th view.Theme, o HunkOpts) []view.Line {
	panic("diffview: RenderHunk not implemented")
}

// RenderGroupLines renders the given hunks grouped by file in diff order:
// for each file a view.RenderFileHeader line, then its hunks, then a blank
// line between files. Unknown IDs are skipped; order of hunkIDs is ignored.
func RenderGroupLines(d *gitdiff.Diff, hunkIDs []string, th view.Theme, width int, marked map[string]bool) []view.Line {
	panic("diffview: RenderGroupLines not implemented")
}

// Span is a byte range [Start, End) of changed text.
type Span struct{ Start, End int }

// WordDiff finds the changed spans between a removed line and the added
// line paired with it (both without their -/+ prefix). ok is false when the
// lines are too long or too different to be worth highlighting.
func WordDiff(oldText, newText string) (oldSpans, newSpans []Span, ok bool) {
	panic("diffview: WordDiff not implemented")
}
