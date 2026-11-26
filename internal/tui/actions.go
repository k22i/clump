package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/report"
	"github.com/k22i/clump/internal/tui/sys"
	"github.com/k22i/clump/internal/tui/view"
)

// groups returns the groups currently shown.
func (m *Model) groups() []group.Group {
	if m.doc == nil || m.doc.hist == nil {
		return nil
	}
	return m.doc.hist.Current().Groups
}

func (m *Model) currentGroup() (group.Group, bool) {
	gs := m.groups()
	if m.cursor < 0 || m.cursor >= len(gs) {
		return group.Group{}, false
	}
	return gs[m.cursor], true
}

func (m *Model) setCursor(i int) {
	n := len(m.groups())
	if n == 0 {
		m.cursor = 0
		return
	}
	i = max(0, min(i, n-1))
	if i != m.cursor {
		m.cursor = i
		if m.mode == modeGrouped {
			m.linesOK, m.scroll, m.dcur = false, 0, 0
		} else {
			m.scrollToGroup()
		}
	}
}

func (m *Model) moveGroup(delta int) {
	m.setCursor(m.cursor + delta)
}

func (m *Model) toggleAll() {
	if m.mode == modeGrouped {
		m.mode = modeAll
		m.focus = focusDiff
		m.linesOK = false
		m.scrollToGroup()
	} else {
		m.mode = modeGrouped
		m.linesOK, m.scroll, m.dcur = false, 0, 0
	}
}

// scrollToGroup puts the diff cursor on the first hunk of the selected group
// in the all view.
func (m *Model) scrollToGroup() {
	g, ok := m.currentGroup()
	if !ok || len(g.Hunks) == 0 {
		return
	}
	lines := m.ensureLines()
	for i, l := range lines {
		if l.HunkID == g.Hunks[0] && l.Kind == view.KindHunk {
			m.setDcur(i)
			m.scroll = max(0, i-2) // keep the file header and boundary visible
			return
		}
	}
}

func (m *Model) setDcur(i int) {
	lines := m.ensureLines()
	if len(lines) == 0 {
		m.dcur, m.scroll = 0, 0
		return
	}
	m.dcur = max(0, min(i, len(lines)-1))
	h := m.bodyHeight()
	if m.dcur < m.scroll {
		m.scroll = m.dcur
	}
	if m.dcur >= m.scroll+h {
		m.scroll = m.dcur - h + 1
	}
	if m.mode == modeAll {
		// Keep the group cursor on the group under the diff cursor.
		if id := m.hunkAt(m.dcur); id != "" {
			if gi := m.doc.hist.Current().GroupOf(id); gi >= 0 {
				m.cursor = gi
			}
		}
	}
}

// hunkAt returns the hunk ID at or after line i (the nearest following hunk
// line), "" if none.
func (m *Model) hunkAt(i int) string {
	lines := m.ensureLines()
	if i >= 0 && i < len(lines) && len(lines[i].BlockHunks) > 0 {
		return lines[i].BlockHunks[0] // a file header: its first hunk
	}
	for j := i; j >= 0 && j < len(lines); j++ {
		if lines[j].HunkID != "" {
			return lines[j].HunkID
		}
	}
	for j := min(i, len(lines)-1); j >= 0; j-- {
		if lines[j].HunkID != "" {
			return lines[j].HunkID
		}
	}
	return ""
}

// cursorHunk is the hunk the user is pointing at: under the diff cursor, or
// the first hunk of the selected group.
func (m *Model) cursorHunk() string {
	if m.focus == focusDiff {
		return m.hunkAt(m.dcur)
	}
	if g, ok := m.currentGroup(); ok && len(g.Hunks) > 0 {
		return g.Hunks[0]
	}
	return ""
}

// ---- marks and visual mode -------------------------------------------

func (m *Model) requireGrouped() bool {
	if m.doc == nil || !m.doc.grouped() {
		m.setFlash("wait for the groups before editing", true)
		return false
	}
	return true
}

func (m *Model) toggleMark() {
	if !m.requireGrouped() {
		return
	}
	var ids []string
	switch {
	case m.visual:
		ids = m.visualRange()
		m.visual = false
	case m.focus == focusList:
		if g, ok := m.currentGroup(); ok {
			ids = g.Hunks
		}
	default:
		lines := m.ensureLines()
		if m.dcur >= 0 && m.dcur < len(lines) && lines[m.dcur].Kind == view.KindFile {
			ids = lines[m.dcur].BlockHunks // x on a file header marks the file
		} else if id := m.cursorHunk(); id != "" {
			ids = []string{id}
		}
	}
	if len(ids) == 0 {
		return
	}
	on := !m.marks[ids[0]]
	for _, id := range ids {
		if on {
			m.marks[id] = true
		} else {
			delete(m.marks, id)
		}
	}
	m.linesOK = false
	if len(m.marks) > 0 {
		m.setFlash(fmt.Sprintf("%d marked — M to move them", len(m.marks)), false)
	}
}

func (m *Model) toggleVisual() {
	if !m.requireGrouped() {
		return
	}
	if m.visual {
		m.visual = false
		return
	}
	m.focus = focusDiff
	m.visual = true
	m.visualAt = m.cursorHunk()
	m.setFlash("visual: move to extend, x to mark, M to move, esc to stop", false)
}

// visualRange is every hunk between the visual anchor and the cursor, in
// display order.
func (m *Model) visualRange() []string {
	cur := m.cursorHunk()
	var order []string
	seen := map[string]bool{}
	for _, l := range m.ensureLines() {
		if l.HunkID != "" && !seen[l.HunkID] {
			seen[l.HunkID] = true
			order = append(order, l.HunkID)
		}
	}
	a, b := -1, -1
	for i, id := range order {
		if id == m.visualAt {
			a = i
		}
		if id == cur {
			b = i
		}
	}
	if a < 0 || b < 0 {
		if cur != "" {
			return []string{cur}
		}
		return nil
	}
	if a > b {
		a, b = b, a
	}
	return append([]string(nil), order[a:b+1]...)
}

// selection is what M acts on: the marks, else the visual range, else the
// hunk under the cursor.
func (m *Model) selection() []string {
	if len(m.marks) > 0 {
		var ids []string
		for _, h := range m.doc.L.Diff.Hunks {
			if m.marks[h.ID] {
				ids = append(ids, h.ID)
			}
		}
		return ids
	}
	if m.visual {
		return m.visualRange()
	}
	if id := m.cursorHunk(); id != "" {
		return []string{id}
	}
	return nil
}

// ---- edits --------------------------------------------------------------

// edit applies op through the history, saves the override and reports.
func (m *Model) edit(what string, op func(group.Grouping) (group.Grouping, error)) tea.Cmd {
	if !m.requireGrouped() {
		return nil
	}
	changed, err := m.doc.hist.Apply(op)
	if err != nil {
		m.setFlash(what+": "+err.Error(), true)
		return nil
	}
	if !changed {
		m.setFlash(what+": nothing changed", false)
		return nil
	}
	m.afterEdit(what)
	return nil
}

func (m *Model) afterEdit(what string) {
	m.linesOK = false
	m.setCursor(m.cursor) // clamp
	if err := m.saveOverride(); err != nil {
		m.setFlash(what+" (not saved: "+err.Error()+")", true)
		return
	}
	m.setFlash(what+" — u to undo", false)
}

// saveOverride writes the effective grouping as the override for this diff,
// or removes the override when the grouping equals the base again and no
// snapshots are kept.
func (m *Model) saveOverride() error {
	if m.o.Store == nil {
		return nil
	}
	d := m.doc
	if !d.edited() && len(d.snapshots) == 0 {
		return m.o.Store.DeleteOverride(d.Hash)
	}
	g := group.Grouping{}
	if d.edited() {
		g = d.hist.Current()
	}
	return m.o.Store.SaveOverride(cache.Override{
		DiffHash: d.Hash, BaseCacheKey: d.baseKey, EditedAt: m.o.Now().UTC(),
		Grouping: g, Snapshots: d.snapshots,
	})
}

func (m *Model) undo() tea.Cmd {
	if !m.requireGrouped() {
		return nil
	}
	if !m.doc.hist.Undo() {
		m.setFlash("nothing to undo", false)
		return nil
	}
	m.afterEdit("undone")
	return nil
}

func (m *Model) redo() tea.Cmd {
	if !m.requireGrouped() {
		return nil
	}
	if !m.doc.hist.Redo() {
		m.setFlash("nothing to redo", false)
		return nil
	}
	m.afterEdit("redone")
	return nil
}

func (m *Model) reorder(delta int) tea.Cmd {
	g, ok := m.currentGroup()
	if !ok {
		return nil
	}
	cmd := m.edit("moved group", func(x group.Grouping) (group.Grouping, error) { return group.Reorder(x, g.ID, delta) })
	if i := m.doc.hist.Current().Find(g.ID); i >= 0 {
		m.cursor = i
	}
	return cmd
}

func (m *Model) deleteGroup() tea.Cmd {
	g, ok := m.currentGroup()
	if !ok {
		return nil
	}
	return m.edit(fmt.Sprintf("deleted %q", g.Title), func(x group.Grouping) (group.Grouping, error) { return group.Delete(x, g.ID) })
}

func (m *Model) startNewGroup() tea.Cmd {
	if !m.requireGrouped() {
		return nil
	}
	m.openInput(inputNew, "New group title: ", "", "", "", nil)
	return nil
}

func (m *Model) startRename() tea.Cmd {
	if !m.requireGrouped() {
		return nil
	}
	g, ok := m.currentGroup()
	if !ok {
		return nil
	}
	if g.ID == group.UngroupedID {
		m.setFlash("Ungrouped cannot be renamed; move its hunks into a group", true)
		return nil
	}
	m.openInput(inputRename, "Title: ", g.Title, g.Description, g.ID, nil)
	return nil
}

func (m *Model) startMove() tea.Cmd {
	if !m.requireGrouped() {
		return nil
	}
	ids := m.selection()
	if len(ids) == 0 {
		m.setFlash("nothing to move: mark hunks with x first", true)
		return nil
	}
	opts := []targetOpt{{id: "", label: "+ New group…"}}
	for i, g := range m.groups() {
		opts = append(opts, targetOpt{id: g.ID, label: fmt.Sprintf("%d. %s", i+1, g.Title)})
	}
	m.target = targetState{title: fmt.Sprintf("Move %d hunks to:", len(ids)), opts: opts, cursor: 1, act: actMove, hunks: ids}
	m.ov = ovTarget
	return nil
}

func (m *Model) startMerge() tea.Cmd {
	if !m.requireGrouped() {
		return nil
	}
	src, ok := m.currentGroup()
	if !ok {
		return nil
	}
	var opts []targetOpt
	for i, g := range m.groups() {
		if g.ID != src.ID {
			opts = append(opts, targetOpt{id: g.ID, label: fmt.Sprintf("%d. %s", i+1, g.Title)})
		}
	}
	if len(opts) == 0 {
		m.setFlash("no other group to merge into", true)
		return nil
	}
	m.target = targetState{title: fmt.Sprintf("Merge %q into:", src.Title), opts: opts, act: actMerge, src: src.ID}
	m.ov = ovTarget
	return nil
}

func (m *Model) finishTarget(o targetOpt) tea.Cmd {
	t := m.target
	switch t.act {
	case actMove:
		if o.id == "" {
			m.openInput(inputMoveNew, "New group title: ", "", "", "", t.hunks)
			return nil
		}
		cmd := m.edit(fmt.Sprintf("moved %d hunks", len(t.hunks)), func(x group.Grouping) (group.Grouping, error) {
			return group.Move(x, t.hunks, o.id)
		})
		m.clearMarks()
		m.cursor = max(0, m.doc.hist.Current().Find(o.id))
		return cmd
	case actMerge:
		cmd := m.edit("merged", func(x group.Grouping) (group.Grouping, error) { return group.Merge(x, t.src, o.id) })
		m.cursor = max(0, m.doc.hist.Current().Find(o.id))
		return cmd
	}
	return nil
}

func (m *Model) clearMarks() {
	m.marks = map[string]bool{}
	m.visual = false
	m.linesOK = false
}

func (m *Model) finishInput() tea.Cmd {
	in := m.input
	title := strings.TrimSpace(in.title.text())
	switch in.act {
	case inputNew:
		var newID string
		cmd := m.edit("new group", func(x group.Grouping) (group.Grouping, error) {
			out, id, err := group.NewGroup(x, title)
			newID = id
			return out, err
		})
		if i := m.doc.hist.Current().Find(newID); i >= 0 {
			m.setCursor(i)
		}
		return cmd
	case inputRename:
		return m.edit("renamed", func(x group.Grouping) (group.Grouping, error) {
			return group.Rename(x, in.id, title, in.desc.text())
		})
	case inputComment:
		m.saveComment(in.id, in.title.text())
	case inputMoveNew:
		var newID string
		cmd := m.edit(fmt.Sprintf("moved %d hunks to a new group", len(in.hunks)), func(x group.Grouping) (group.Grouping, error) {
			out, id, err := group.MoveToNew(x, in.hunks, title)
			newID = id
			return out, err
		})
		m.clearMarks()
		if i := m.doc.hist.Current().Find(newID); i >= 0 {
			m.setCursor(i)
		}
		return cmd
	}
	return nil
}

// ---- regroup, provider, copy, editor -------------------------------------

func (m *Model) regroup() tea.Cmd {
	if m.doc == nil || m.doc.state == stLoading {
		return nil
	}
	if m.doc.state == stGrouping {
		m.setFlash("already grouping", false)
		return nil
	}
	if m.doc.edited() {
		m.askConfirm("Regroup discards your edits: [d] discard  [s] keep them as a snapshot  [esc] cancel", confirmRegroup)
		return nil
	}
	return m.requestGroup(true, true)
}

// doRegroup runs after the regroup confirmation.
func (m *Model) doRegroup(keep bool) tea.Cmd {
	d := m.doc
	if keep {
		d.snapshots = append(d.snapshots, cache.Snapshot{
			Name: "before regroup " + m.o.Now().Format("2006-01-02 15:04"), At: m.o.Now().UTC(), Grouping: d.hist.Current(),
		})
	}
	d.hist.Reset(d.base)
	if err := m.saveOverride(); err != nil {
		m.setFlash("could not update saved edits: "+err.Error(), true)
	}
	m.linesOK = false
	return m.requestGroup(true, true)
}

func (m *Model) openProviderMenu() {
	if m.o.Engine == nil {
		return
	}
	names := m.o.Engine.Reg.Names()
	cur, _ := m.o.Engine.Reg.Resolve(m.provider)
	var items []menuItem
	sel := 0
	for i, n := range names {
		label := n
		if al := m.o.Engine.Reg.Aliases(n); len(al) > 0 {
			label += " (" + strings.Join(al, ", ") + ")"
		}
		if n == cur {
			sel = i
		}
		items = append(items, menuItem{name: n, label: label})
	}
	m.menu = menuState{items: items, cursor: sel}
	m.ov = ovProvider
}

func (m *Model) switchProvider(name string) tea.Cmd {
	m.provider = name
	if m.doc == nil || m.doc.L == nil || m.doc.state == stLoading || len(m.doc.L.Diff.Hunks) == 0 {
		return nil
	}
	if m.doc.edited() {
		m.setFlash("provider is now "+name+"; your edits are kept (r regroups with it)", false)
		return nil
	}
	if out, ok, err := m.o.Engine.Lookup(m.doc.L.Diff, name); err == nil && ok {
		m.doc.setBase(out.Result.Grouping, out.Key, out.Provider, true, out.Result.Fallback, out.Result.Notices)
		m.linesOK = false
		m.setFlash("provider is now "+name+" (cached)", false)
		return nil
	}
	m.setFlash("provider is now "+name, false)
	return m.requestGroup(m.autoGroup(), false)
}

// autoGroup is whether opening a diff may call the provider by itself.
func (m *Model) autoGroup() bool {
	if m.watch != nil {
		return m.watch.auto
	}
	return true
}

func (m *Model) copySummary() tea.Cmd {
	if m.doc == nil || m.doc.L == nil {
		return nil
	}
	m.setFlash("copying summary…", false)
	return sys.CopyCmd(report.Markdown(m.reportDoc()))
}

func (m *Model) openEditor() tea.Cmd {
	id := m.cursorHunk()
	h := m.doc.L.Diff.Hunk(id)
	if h == nil {
		return nil
	}
	path, line, ok := sys.HunkTarget(h)
	if !ok {
		m.setFlash("nothing to open for this hunk", true)
		return nil
	}
	if m.o.Repo == "" {
		m.setFlash("not in a repository; cannot open "+path, true)
		return nil
	}
	argv, err := sys.EditorArgv(sys.EditorFromEnv(os.Getenv), filepath.Join(m.o.Repo, path), line)
	if err != nil {
		m.setFlash(err.Error(), true)
		return nil
	}
	return sys.EditCmd(argv, m.o.Repo)
}

// groupNow sends the prepared job (watch: g; above the ceiling: y).
func (m *Model) groupNow() tea.Cmd {
	d := m.doc
	if d == nil || d.state != stNeedsConfirm {
		return nil
	}
	if d.job == nil {
		return m.requestGroup(true, false)
	}
	return m.startJob(d.job, false)
}

// onSideMsg handles results from sys commands.
func (m *Model) onSideMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case sys.CopiedMsg:
		if msg.Err != nil {
			m.setFlash("copied via OSC 52 ("+msg.Err.Error()+")", false)
		} else {
			m.setFlash("copied Markdown summary ("+msg.Via+")", false)
		}
		return nil, true
	case sys.EditedMsg:
		if msg.Err != nil && !errors.Is(msg.Err, os.ErrProcessDone) {
			m.setFlash("editor: "+msg.Err.Error(), true)
		}
		return nil, true
	case submitMsg:
		return m.onSubmitMsg(msg), true
	case pickLoadedMsg:
		m.onPickLoaded(msg)
		return nil, true
	}
	return nil, false
}
