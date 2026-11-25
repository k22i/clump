// Package rawview renders clump's "all" view: the whole diff in file order,
// each hunk labelled with its group, with group boundaries marked.
package rawview

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/tui/view"
)

// LabelWidth is the width of the "[ n] " label column RenderAll adds.
const LabelWidth = 5

// noGroupTitle is the boundary title of a hunk that no group lists.
const noGroupTitle = "(no group)"

// HunkRenderer renders one hunk at the given width (diffview.RenderHunk in
// the app; a stub in tests).
type HunkRenderer func(h *gitdiff.Hunk, width int) []view.Line

// RenderAll renders every hunk of d in file order. See DESIGN.md §9.
//
// Each file with hunks gets a file header (summing all of its hunks), then its
// hunks, each hunk labelled with the 1-based position of its group in g. A
// boundary line is emitted whenever the group changes between consecutive
// hunks in file order. Hunks no group lists are shown as "?" with the title
// "(no group)". An empty diff renders an empty, non-nil slice.
func RenderAll(d *gitdiff.Diff, g group.Grouping, th view.Theme, width int, hunk HunkRenderer) []view.Line {
	return RenderAllOpts(d, g, th, width, hunk, nil)
}

// FileStateFunc gives a file's header state from its index and hunk IDs.
type FileStateFunc func(fileIdx int, hunkIDs []string) view.FileState

// RenderAllOpts is RenderAll with per-file header state (nil: all
// expanded). A folded file renders its header only and its hunks emit no
// group boundary: boundaries compare shown hunks only.
func RenderAllOpts(d *gitdiff.Diff, g group.Grouping, th view.Theme, width int, hunk HunkRenderer, file FileStateFunc) []view.Line {
	out := make([]view.Line, 0, len(d.Hunks)+len(d.Files)*2)
	if d == nil {
		return out
	}
	// pos is each hunk's group number (0 when no group lists it), titles maps
	// hunk ID to that group's title.
	pos := make(map[string]int, len(d.Hunks))
	titles := make(map[string]string, len(d.Hunks))
	for i, gr := range g.Groups {
		for _, id := range gr.Hunks {
			pos[id] = i + 1
			titles[id] = gr.Title
		}
	}
	hunkWidth := width - LabelWidth
	if width <= 0 {
		hunkWidth = 0
	}

	prevGroup := 0
	first := true
	files := 0
	for fileIdx, f := range d.Files {
		if len(f.Hunks) == 0 {
			continue
		}
		if files > 0 {
			out = append(out, view.Line{Kind: view.KindBlank, FileIdx: -1})
		}
		files++

		add, del := 0, 0
		ids := make([]string, len(f.Hunks))
		for i, h := range f.Hunks {
			add += h.Add
			del += h.Del
			ids[i] = h.ID
		}
		var st view.FileState
		if file != nil {
			st = file(fileIdx, ids)
		}
		out = append(out, view.RenderFileHeaderState(f, fileIdx, add, del, ids, st, th, width))
		if st.Folded {
			continue
		}

		for _, h := range f.Hunks {
			n, ok := pos[h.ID]
			title := titles[h.ID]
			if !ok {
				n, title = 0, noGroupTitle
			}
			if first || n != prevGroup {
				out = append(out, boundaryLine(n, title, th, width))
				prevGroup = n
				first = false
			}
			out = append(out, labelLines(hunk(h, hunkWidth), n, th)...)
		}
	}
	return out
}

// boundaryLine renders the "┄┄ 2. Assets ┄┄┄" marker that starts a run of
// hunks belonging to one group, padded with "┄" to width.
func boundaryLine(n int, title string, th view.Theme, width int) view.Line {
	label := "?."
	if n > 0 {
		label = fmt.Sprintf("%d.", n)
	}
	text := "┄┄ " + label + " " + title + " "
	if w := lipgloss.Width(text); w < width {
		text += strings.Repeat("┄", width-w)
	}
	text = view.Truncate(text, width)
	return view.Line{
		Text:    th.Style(th.GroupLabel, text),
		Plain:   text,
		Kind:    view.KindBoundary,
		FileIdx: -1,
	}
}

// labelLines prefixes a hunk's rendered lines with the "[ n] " label column:
// the hunk's first KindHunk line carries the label, every other line five
// spaces. All other fields of each line are kept as the renderer set them.
func labelLines(lines []view.Line, n int, th view.Theme) []view.Line {
	label := "[ ?] "
	if n > 0 {
		label = fmt.Sprintf("[%2d] ", n)
	}
	blank := strings.Repeat(" ", LabelWidth)
	out := make([]view.Line, len(lines))
	labeled := false
	for i, ln := range lines {
		if !labeled && ln.Kind == view.KindHunk {
			ln.Plain = label + ln.Plain
			ln.Text = th.Style(th.GroupLabel, label) + ln.Text
			labeled = true
		} else {
			ln.Plain = blank + ln.Plain
			ln.Text = blank + ln.Text
		}
		ln.Gutter += LabelWidth
		out[i] = ln
	}
	return out
}
