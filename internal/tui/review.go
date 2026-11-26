package tui

import (
	"fmt"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/tui/diffview"
	"github.com/k22i/clump/internal/tui/view"
)

type foldKey struct {
	group string
	file  int
}

func (m *Model) loadReviewed() {
	m.reviewed = map[string]bool{}
	if m.o.Store == nil || m.doc == nil || len(m.doc.rkeys) == 0 {
		return
	}
	keys := make([]string, 0, len(m.doc.rkeys))
	for _, k := range m.doc.rkeys {
		keys = append(keys, k)
	}
	got, err := m.o.Store.Reviewed(keys)
	if err != nil {
		m.setFlash("reviewed marks not loaded: "+err.Error(), true)
		return
	}
	m.reviewed = got
}

func (m *Model) isReviewed(id string) bool {
	if m.doc == nil {
		return false
	}
	k, ok := m.doc.rkeys[id]
	return ok && m.reviewed[k]
}

func (m *Model) allReviewed(ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if !m.isReviewed(id) {
			return false
		}
	}
	return true
}

func (m *Model) countReviewed(ids []string) int {
	n := 0
	for _, id := range ids {
		if m.isReviewed(id) {
			n++
		}
	}
	return n
}

func (m *Model) setReviewed(ids []string, on bool) error {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		k, ok := m.doc.rkeys[id]
		if !ok {
			continue
		}
		keys = append(keys, k)
		if on {
			m.reviewed[k] = true
		} else {
			delete(m.reviewed, k)
		}
	}
	m.linesOK = false
	if m.o.Store == nil || len(keys) == 0 {
		return nil
	}
	return m.o.Store.SetReviewed(keys, on, m.o.Now())
}

func countsTowardReview(g group.Group) bool {
	return g.ID != group.GeneratedID && len(g.Hunks) > 0
}

func (m *Model) reviewProgress() (done, total int) {
	for _, g := range m.groups() {
		if !countsTowardReview(g) {
			continue
		}
		total++
		if m.allReviewed(g.Hunks) {
			done++
		}
	}
	return done, total
}

func (m *Model) foldGroup() string {
	if m.mode == modeAll {
		return ""
	}
	if g, ok := m.currentGroup(); ok {
		return g.ID
	}
	return ""
}

func (m *Model) fileState() func(fileIdx int, ids []string) view.FileState {
	d := m.doc.L.Diff
	grp := m.foldGroup()
	return func(fi int, ids []string) view.FileState {
		var f *gitdiff.File
		if fi >= 0 && fi < len(d.Files) {
			f = d.Files[fi]
		}
		st := view.FileState{Reviewed: m.allReviewed(ids)}
		if f != nil {
			st.Generated = f.Generated
		}
		st.Folded = st.Reviewed || st.Generated != ""
		if v, ok := m.fold[foldKey{grp, fi}]; ok {
			st.Folded = v
		}
		return st
	}
}

func (m *Model) numberWidth() int {
	if !m.lineNumbers || m.doc == nil || m.doc.L == nil {
		return 0
	}
	if m.doc.numW == 0 {
		m.doc.numW = diffview.NumberWidth(m.doc.L.Diff)
	}
	return m.doc.numW
}

func (m *Model) fileLineAt(i int) int {
	lines := m.ensureLines()
	for j := min(i, len(lines)-1); j >= 0; j-- {
		switch lines[j].Kind {
		case view.KindFile:
			return j
		case view.KindBoundary, view.KindGroup:
		case view.KindBlank:
			if j < i {
				return -1
			}
		}
	}
	return -1
}

func (m *Model) headerLine(fileIdx int) int {
	for i, l := range m.ensureLines() {
		if l.Kind == view.KindFile && l.FileIdx == fileIdx {
			return i
		}
	}
	return -1
}

func (m *Model) toggleLineNumbers() {
	m.lineNumbers = !m.lineNumbers
	m.keepCursorLine(func() { m.linesOK = false })
	if m.lineNumbers {
		m.setFlash("line numbers on", false)
	} else {
		m.setFlash("line numbers off", false)
	}
}

func (m *Model) toggleSplit() {
	m.split = !m.split
	m.keepCursorLine(func() { m.linesOK = false })
	if m.split {
		m.setFlash("side by side on", false)
	} else {
		m.setFlash("side by side off", false)
	}
}

func (m *Model) keepCursorLine(change func()) {
	lines := m.ensureLines()
	var at view.Line
	ok := m.dcur >= 0 && m.dcur < len(lines)
	if ok {
		at = lines[m.dcur]
	}
	change()
	lines = m.ensureLines()
	if !ok {
		return
	}
	for i, l := range lines {
		if l.Kind == at.Kind && l.FileIdx == at.FileIdx && l.HunkID == at.HunkID &&
			l.OldLine == at.OldLine && l.NewLine == at.NewLine {
			m.setDcur(i)
			return
		}
	}
}

func (m *Model) toggleFold() {
	if m.focus != focusDiff {
		m.foldAll()
		return
	}
	hi := m.fileLineAt(m.dcur)
	if hi < 0 {
		return
	}
	l := m.ensureLines()[hi]
	m.fold[foldKey{m.foldGroup(), l.FileIdx}] = !l.Folded
	m.linesOK = false
	if j := m.headerLine(l.FileIdx); j >= 0 {
		m.setDcur(j)
	}
}

func (m *Model) foldAll() {
	lines := m.ensureLines()
	var files []int
	allFolded := true
	for _, l := range lines {
		if l.Kind == view.KindFile {
			files = append(files, l.FileIdx)
			allFolded = allFolded && l.Folded
		}
	}
	if len(files) == 0 {
		return
	}
	grp := m.foldGroup()
	for _, fi := range files {
		m.fold[foldKey{grp, fi}] = !allFolded
	}
	m.linesOK = false
	m.setDcur(m.dcur)
	if allFolded {
		m.setFlash("unfolded "+plural(len(files), "file"), false)
	} else {
		m.setFlash("folded "+plural(len(files), "file"), false)
	}
}

func (m *Model) toggleReviewed() {
	if m.doc == nil || m.doc.L == nil || len(m.doc.L.Diff.Hunks) == 0 {
		return
	}
	if m.focus == focusDiff {
		m.reviewFile()
		return
	}
	g, ok := m.currentGroup()
	if !ok || len(g.Hunks) == 0 {
		return
	}
	on := !m.allReviewed(g.Hunks)
	if err := m.setReviewed(g.Hunks, on); err != nil {
		m.setFlash("reviewed mark not saved: "+err.Error(), true)
		return
	}
	m.clearFolds(g.ID, g.Hunks)
	if !on {
		m.setFlash("unreviewed: "+g.Title, false)
		return
	}
	m.advanceAfterReview(fmt.Sprintf("✓ %s", g.Title))
}

func (m *Model) reviewFile() {
	hi := m.fileLineAt(m.dcur)
	if hi < 0 {
		return
	}
	l := m.ensureLines()[hi]
	ids := l.BlockHunks
	if len(ids) == 0 {
		return
	}
	on := !m.allReviewed(ids)
	if err := m.setReviewed(ids, on); err != nil {
		m.setFlash("reviewed mark not saved: "+err.Error(), true)
		return
	}
	delete(m.fold, foldKey{m.foldGroup(), l.FileIdx})
	path := m.doc.L.Diff.Files[l.FileIdx].DisplayPath()
	if !on {
		m.setFlash("unreviewed: "+path, false)
		if j := m.headerLine(l.FileIdx); j >= 0 {
			m.setDcur(j)
		}
		return
	}
	if m.mode == modeGrouped {
		if g, ok := m.currentGroup(); ok && m.allReviewed(g.Hunks) {
			m.advanceAfterReview("✓ " + g.Title)
			return
		}
	}
	lines := m.ensureLines()
	j := m.headerLine(l.FileIdx)
	next := j
	for i := j + 1; i >= 1 && i < len(lines); i++ {
		if lines[i].Kind == view.KindFile {
			next = i
			break
		}
	}
	if next >= 0 {
		m.setDcur(next)
	}
	m.setFlash("✓ "+path, false)
}

func (m *Model) clearFolds(grp string, ids []string) {
	d := m.doc.L.Diff
	for _, id := range ids {
		if h := d.Hunk(id); h != nil {
			delete(m.fold, foldKey{grp, h.FileIdx})
			delete(m.fold, foldKey{"", h.FileIdx})
		}
	}
}

func (m *Model) advanceAfterReview(what string) {
	done, total := m.reviewProgress()
	gs := m.groups()
	n := len(gs)
	for step := 1; step < n; step++ {
		i := (m.cursor + step) % n
		if countsTowardReview(gs[i]) && !m.allReviewed(gs[i].Hunks) {
			if m.mode == modeGrouped {
				m.setCursor(i)
			}
			m.setFlash(fmt.Sprintf("%s · %d/%d reviewed · next: %s", what, done, total, gs[i].Title), false)
			return
		}
	}
	m.linesOK = false
	m.setFlash(fmt.Sprintf("%s · all %d groups reviewed", what, total), false)
}
