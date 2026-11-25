// Package rawview renders clump's "all" view: the whole diff in file order,
// each hunk labelled with its group, with group boundaries marked.
package rawview

import (
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/tui/view"
)

// LabelWidth is the width of the "[ n] " label column RenderAll adds.
const LabelWidth = 5

// HunkRenderer renders one hunk at the given width (diffview.RenderHunk in
// the app; a stub in tests).
type HunkRenderer func(h *gitdiff.Hunk, width int) []view.Line

// RenderAll renders every hunk of d in file order. See DESIGN.md §9.
func RenderAll(d *gitdiff.Diff, g group.Grouping, th view.Theme, width int, hunk HunkRenderer) []view.Line {
	panic("rawview: RenderAll not implemented")
}
