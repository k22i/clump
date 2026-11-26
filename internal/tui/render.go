package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/report"
	"github.com/k22i/clump/internal/tui/diffview"
	"github.com/k22i/clump/internal/tui/helpview"
	"github.com/k22i/clump/internal/tui/listview"
	"github.com/k22i/clump/internal/tui/rawview"
	"github.com/k22i/clump/internal/tui/view"
)

// wideMin is the width from which the list and the diff sit side by side.
const wideMin = 100

func (m *Model) wide() bool { return m.width >= wideMin }

// listWidth is the group list's width in the wide layout.
func (m *Model) listWidth() int { return max(24, min(48, m.width/3)) }

// diffWidth is the diff pane's width.
func (m *Model) diffWidth() int {
	if m.wide() {
		return max(10, m.width-m.listWidth()-1)
	}
	return max(10, m.width)
}

// listHeight is the list's height in the narrow layout.
func (m *Model) listHeight() int {
	h := m.height - 2
	return max(1, min(len(m.groups()), max(3, h/3)))
}

// bodyHeight is the diff pane's height.
func (m *Model) bodyHeight() int {
	h := m.height - 2 // header + footer
	if !m.wide() {
		h -= m.listHeight() + 1
	}
	return max(1, h)
}

// ensureLines (re)builds the diff pane's lines when stale.
func (m *Model) ensureLines() []view.Line {
	if m.linesOK {
		return m.lines
	}
	m.linesOK = true
	m.lines = nil
	if m.doc == nil || m.doc.L == nil {
		return nil
	}
	d := m.doc.L.Diff
	w := m.diffWidth()
	nw := m.numberWidth()
	if m.mode == modeAll {
		m.lines = rawview.RenderAllOpts(d, m.doc.hist.Current(), m.th, w, func(h *gitdiff.Hunk, width int) []view.Line {
			return diffview.RenderHunk(h, m.th, diffview.HunkOpts{Width: width, Marked: m.marks[h.ID], Path: h.File.Path(), Numbers: nw, Split: m.split})
		}, m.fileState())
	} else if g, ok := m.currentGroup(); ok {
		m.lines = append(m.groupBlock(g, w), diffview.RenderGroupLinesOpts(d, g.Hunks, m.th, w, diffview.GroupOpts{
			Marked: m.marks, Numbers: nw, File: m.fileState(), Split: m.split,
		})...)
	}
	m.lines = m.insertCommentRows(m.lines)
	if m.query != "" {
		m.matches = helpview.FindMatches(m.lines, m.query)
	}
	m.dcur = max(0, min(m.dcur, len(m.lines)-1))
	return m.lines
}

// groupBlock is the selected group's title and explanation above its hunks.
func (m *Model) groupBlock(g group.Group, w int) []view.Line {
	mk := func(s string, st lipgloss.Style) view.Line {
		s = view.Truncate(s, w)
		return view.Line{Text: m.th.Style(st, s), Plain: s, Kind: view.KindGroup, FileIdx: -1}
	}
	d := m.doc.L.Diff
	add, del, files := report.GroupStats(d, g)
	title := fmt.Sprintf("%d. %s", m.cursor+1, g.Title)
	if !m.doc.grouped() {
		title = g.Title
	}
	out := []view.Line{mk(title, m.th.Title)}
	stats := fmt.Sprintf("%s · %d hunks · +%d −%d", plural(files, "file"), len(g.Hunks), add, del)
	switch r := m.countReviewed(g.Hunks); {
	case len(g.Hunks) > 0 && r == len(g.Hunks):
		stats += " · ✓ reviewed"
	case r > 0:
		stats += fmt.Sprintf(" · %d/%d hunks reviewed", r, len(g.Hunks))
	}
	out = append(out, mk(stats, m.th.Dim))
	if desc := strings.TrimSpace(g.Description); desc != "" && m.doc.grouped() {
		for _, para := range strings.Split(desc, "\n") {
			for _, l := range wrap(para, w) {
				out = append(out, mk(l, m.th.Context))
			}
		}
	}
	if len(g.Hunks) == 0 {
		out = append(out, mk("(empty — mark hunks with x and move them here with M)", m.th.Dim))
	}
	out = append(out, view.Line{Kind: view.KindBlank, FileIdx: -1})
	return out
}

func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return fmt.Sprintf("%d %ss", n, w)
}

// wrap breaks s into lines of at most w cells at spaces.
func wrap(s string, w int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var out []string
	cur := ""
	for _, wd := range words {
		switch {
		case cur == "":
			cur = wd
		case lipgloss.Width(cur)+1+lipgloss.Width(wd) <= w:
			cur += " " + wd
		default:
			out = append(out, cur)
			cur = wd
		}
	}
	return append(out, cur)
}

// listRows are the group list's rows.
func (m *Model) listRows() []view.ListRow {
	var rows []view.ListRow
	d := m.doc.L.Diff
	for _, g := range m.groups() {
		add, del, _ := report.GroupStats(d, g)
		marks := 0
		for _, id := range g.Hunks {
			if m.marks[id] {
				marks++
			}
		}
		rows = append(rows, view.ListRow{ID: g.ID, Title: g.Title, Add: add, Del: del, Hunks: len(g.Hunks), Marks: marks,
			Ungrouped: g.ID == group.UngroupedID || g.ID == group.GeneratedID, Reviewed: m.countReviewed(g.Hunks)})
	}
	return rows
}

func (m *Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	header := m.header()
	footer := m.footer()
	bodyH := m.height - 2
	var body string
	switch {
	case m.ov == ovPick:
		body = m.renderPick(m.width, bodyH)
	case m.doc == nil && m.watch != nil:
		body = m.placeholder("watching "+m.watch.o.Repo+": select a commit, range, stash or diffing-mode comparison in lazygit", bodyH)
	case m.doc == nil || m.doc.state == stLoading:
		body = m.placeholder(m.spin.View()+" loading…", bodyH)
	case m.doc.state == stFailed && (m.doc.L == nil || len(m.doc.L.Diff.Hunks) == 0):
		body = m.placeholder(m.th.Style(m.th.Error, "error: "+errText(m.doc.err)), bodyH)
	case m.ov == ovHelp:
		body = helpview.Render(HelpSections(m.watch != nil), m.width, bodyH, m.th)
	case m.ov == ovTarget:
		body = m.renderChoice(m.target.title, targetLabels(m.target.opts), m.target.cursor, bodyH)
	case m.ov == ovProvider:
		var labels []string
		for _, it := range m.menu.items {
			labels = append(labels, it.label)
		}
		body = m.renderChoice("Group with provider:", labels, m.menu.cursor, bodyH)
	case m.ov == ovSubmit:
		var labels []string
		for _, it := range m.submit.items {
			labels = append(labels, it.label)
		}
		body = m.renderChoice(m.submit.title, labels, m.submit.cursor, bodyH)
	default:
		body = m.renderBody(bodyH)
	}
	body = fitLines(body, m.width, bodyH)
	return header + "\n" + body + "\n" + footer
}

func errText(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

func targetLabels(opts []targetOpt) []string {
	var out []string
	for _, o := range opts {
		out = append(out, o.label)
	}
	return out
}

func (m *Model) placeholder(s string, h int) string {
	lines := make([]string, h)
	if h > 1 {
		lines[h/2] = "  " + s
	} else {
		lines[0] = s
	}
	return strings.Join(lines, "\n")
}

// renderChoice renders a titled list with a cursor.
func (m *Model) renderChoice(title string, items []string, cursor, h int) string {
	lines := []string{"", "  " + m.th.Style(m.th.Title, title), ""}
	start, end := listview.Window(len(items), cursor, max(1, h-5))
	for i := start; i < end; i++ {
		row := "    " + items[i]
		if i == cursor {
			row = "  ▸ " + items[i]
			row = m.th.Style(m.th.Selected, view.Truncate(row, m.width))
		}
		lines = append(lines, row)
	}
	lines = append(lines, "", "  "+m.th.Style(m.th.Dim, "enter choose · esc cancel"))
	return strings.Join(lines, "\n")
}

// renderBody is the list plus the diff pane.
func (m *Model) renderBody(bodyH int) string {
	rows := m.listRows()
	dw := m.diffWidth()
	if m.wide() {
		lw := m.listWidth()
		list := listview.Render(rows, m.cursor, lw, bodyH, m.listTheme())
		diff := m.renderDiff(dw, bodyH)
		ll := padLines(list, lw, bodyH)
		dl := padLines(diff, dw, bodyH)
		sep := m.th.Style(m.th.Border, "│")
		out := make([]string, bodyH)
		for i := range out {
			out[i] = ll[i] + sep + dl[i]
		}
		return strings.Join(out, "\n")
	}
	lh := m.listHeight()
	list := listview.Render(rows, m.cursor, m.width, lh, m.listTheme())
	sep := m.th.Style(m.th.Border, strings.Repeat("─", m.width))
	diff := m.renderDiff(dw, bodyH-lh-1)
	return strings.Join(padLines(list, m.width, lh), "\n") + "\n" + sep + "\n" + diff
}

// listTheme dims the list when it is not focused (keeps the cursor visible).
func (m *Model) listTheme() view.Theme {
	th := m.th
	if m.focus == focusDiff && !th.Plain {
		th.Selected = th.Selected.Faint(true)
	}
	return th
}

// renderDiff renders the visible window of the diff pane.
func (m *Model) renderDiff(w, h int) string {
	lines := m.ensureLines()
	if len(lines) == 0 {
		return ""
	}
	// The last page stays full: the window never scrolls past the last line.
	if maxScroll := max(0, len(lines)-h); m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	matchSet := map[int]bool{}
	for _, i := range m.matches {
		matchSet[i] = true
	}
	var vis []string
	visual := map[string]bool{}
	if m.visual {
		for _, id := range m.visualRange() {
			visual[id] = true
		}
	}
	dim := m.watch != nil && m.watch.dimmed
	for i := m.scroll; i < len(lines) && i < m.scroll+h; i++ {
		l := lines[i]
		text := l.Text
		switch {
		case m.focus == focusDiff && i == m.dcur:
			text = m.th.Style(m.th.Selected, padTo(l.Plain, w))
		case visual[l.HunkID] && l.HunkID != "":
			text = m.th.Style(m.th.Marked, l.Plain)
		case matchSet[i] && m.query != "":
			text = helpview.Highlight(l, m.query, m.th)
		case dim:
			text = m.th.Style(m.th.Dim, l.Plain)
		}
		vis = append(vis, text)
	}
	return strings.Join(vis, "\n")
}

// header is the top line: subject, ref, provider and markers.
func (m *Model) header() string {
	var parts []string
	state := ""
	if m.doc != nil && m.doc.L != nil {
		d := m.doc
		if d.L.Title != "" {
			parts = append(parts, d.L.Title)
		}
		if d.L.Ref != "" {
			parts = append(parts, d.L.Ref)
		}
		parts = append(parts, "→ "+m.providerLabel())
		switch {
		case d.edited():
			parts = append(parts, "edited")
		case d.cached:
			parts = append(parts, "cached")
		}
		if d.fallback {
			parts = append(parts, "per-file fallback")
		}
		if d.grouped() {
			if done, total := m.reviewProgress(); done > 0 {
				parts = append(parts, fmt.Sprintf("%d/%d reviewed", done, total))
			}
		}
		switch d.state {
		case stGrouping:
			state = fmt.Sprintf("%s grouping… %ds", m.spin.View(), int(m.o.Now().Sub(d.started)/time.Second))
		case stNeedsConfirm:
			if d.job != nil {
				state = "press g to group (" + d.job.Est.String() + ")"
			}
		case stFailed:
			state = "error"
		}
	}
	if m.watch != nil {
		parts = append(parts, m.watch.headerTag())
	}
	left := "clump · " + strings.Join(parts, " · ")
	if state != "" {
		left += "  " + state
	}
	return m.th.Style(m.th.Title, view.Truncate(left, m.width))
}

func (m *Model) providerLabel() string {
	if m.doc != nil && m.doc.provider != "" && m.doc.grouped() {
		return m.doc.provider
	}
	if m.o.Engine != nil {
		if n, err := m.o.Engine.Reg.Resolve(m.provider); err == nil {
			return n
		}
	}
	return m.provider
}

// footer is the notices bar / input line.
func (m *Model) footer() string {
	w := m.width
	switch m.ov {
	case ovSearch:
		return m.searchBox.view(w, m.th, true)
	case ovConfirm:
		return m.th.Style(m.th.Notice, view.Truncate(m.confirm.question, w))
	case ovInput:
		if m.input.act == inputRename {
			t := m.input.title.view(w/2, m.th, m.input.field == 0)
			d := m.input.desc.view(w-w/2-1, m.th, m.input.field == 1)
			return t + " " + d
		}
		return m.input.title.view(w, m.th, true)
	}
	if m.flash != "" {
		st := m.th.Notice
		if m.flashErr {
			st = m.th.Error
		}
		return m.th.Style(st, view.Truncate(m.flash, w))
	}
	if m.doc != nil {
		if m.doc.state == stFailed && m.doc.err != nil {
			return m.th.Style(m.th.Error, view.Truncate("error: "+m.doc.err.Error(), w))
		}
		if len(m.doc.notices) > 0 {
			return m.th.Style(m.th.Notice, view.Truncate(strings.Join(m.doc.notices, " · "), w))
		}
	}
	hint := "? keys · c reviewed · z fold · a all · s split · / search · x mark · M move · u undo · q quit"
	if m.visual {
		hint = "VISUAL · move to extend · x mark · M move · esc"
	}
	if m.query != "" {
		hint = fmt.Sprintf("/%s: %d matches · n/N · esc clears", m.query, len(m.matches))
	}
	return m.th.Style(m.th.Dim, view.Truncate(hint, w))
}

// padLines splits s into exactly h lines, each padded to w cells.
func padLines(s string, w, h int) []string {
	var lines []string
	if s != "" {
		lines = strings.Split(s, "\n")
	}
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = padTo(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// padTo pads s (styled or not) with spaces to w cells.
func padTo(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// fitLines makes s exactly h lines.
func fitLines(s string, w, h int) string {
	return strings.Join(padLines(s, w, h), "\n")
}
