package tui

// Review comments: writing, storing and locating them in the pane.
//
// A comment attaches to a line (a rendered diff line), a file (its whole
// change) or a group, each keyed by content (see internal/comment). Comments
// load with the diff and after a regroup; a new group's key is fetched on
// first use so comments survive group edits without a reload. C comments
// the target under the cursor and opens the one-line input; D deletes the
// comment row under the cursor.

import (
	"strings"
	"time"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/report"
	"github.com/k22i/clump/internal/tui/view"
)

// commentRef names one rendered comment row's comment, for D.
type commentRef struct {
	key string
	id  string
}

// reportDoc is the document the outputs see, comments included.
func (m *Model) reportDoc() report.Doc {
	d := m.doc.report()
	d.Comments = m.reportComments()
	return d
}

// reportComments places the doc's comments on the grouping in play,
// stable in whole-diff order: files and lines in diff order, then groups.
func (m *Model) reportComments() []comment.Placed {
	if m.doc == nil || m.doc.L == nil || len(m.comments) == 0 {
		return nil
	}
	anchors := comment.Anchors(m.doc.L.Diff, m.doc.Hash, m.groupIDs())
	res := comment.Resolve(anchors, m.comments)
	return comment.Place(res, m.doc.L.Diff, m.doc.hist.Current())
}

// loadComments reads the comments of every anchor of the doc and its current
// grouping (group comments follow the grouping on screen).
func (m *Model) loadComments() {
	m.comments = map[string][]comment.Comment{}
	m.loadedKeys = map[string]bool{}
	m.commentAt = map[int]commentRef{}
	if m.doc == nil || m.doc.L == nil || m.o.Store == nil {
		return
	}
	keys := m.commentKeys()
	if len(keys) == 0 {
		return
	}
	got, err := m.o.Store.Comments(keys)
	if err != nil {
		m.setFlash("comments not loaded: "+err.Error(), true)
		return
	}
	for _, k := range keys {
		m.loadedKeys[k] = true
	}
	m.comments = got
}

// commentKeys lists the anchor keys of the doc as shown now.
func (m *Model) commentKeys() []string {
	seen := map[string]bool{}
	var keys []string
	for _, a := range comment.Anchors(m.doc.L.Diff, m.doc.Hash, m.groupIDs()) {
		if !seen[a.Key] {
			seen[a.Key] = true
			keys = append(keys, a.Key)
		}
	}
	return keys
}

// groupIDs is the current grouping's group IDs (""-free).
func (m *Model) groupIDs() []string {
	gs := m.groups()
	ids := make([]string, 0, len(gs))
	for _, g := range gs {
		ids = append(ids, g.ID)
	}
	return ids
}

// commentsOn returns the comments stored on key, fetching the key once when
// it is first seen (a group's key can be newer than the last load).
func (m *Model) commentsOn(key string) []comment.Comment {
	if cs, ok := m.comments[key]; ok {
		return cs
	}
	if m.loadedKeys[key] {
		return nil
	}
	m.loadedKeys[key] = true
	if m.o.Store == nil {
		return nil
	}
	got, err := m.o.Store.Comments([]string{key})
	if err != nil {
		m.setFlash("comments not loaded: "+err.Error(), true)
		return nil
	}
	if len(got) > 0 {
		m.comments[key] = got[key]
		return got[key]
	}
	return nil
}

// fileKey is the anchor key of file fi's whole change.
func (m *Model) fileKey(fi int) string {
	if m.doc == nil || m.doc.L == nil || fi < 0 || fi >= len(m.doc.L.Diff.Files) {
		return ""
	}
	f := m.doc.L.Diff.Files[fi]
	rkeys := make([]string, 0, len(f.Hunks))
	for _, h := range f.Hunks {
		rkeys = append(rkeys, h.ReviewKey())
	}
	return comment.FileKey(f.DisplayPath(), rkeys)
}

// typedComment is the anchor key and prompt title for the target under the
// cursor; ok is false when there is nothing to comment on.
func (m *Model) typedComment() (key, target string, ok bool) {
	if m.doc == nil || m.doc.L == nil {
		return "", "", false
	}
	if m.focus == focusList {
		g, gok := m.currentGroup()
		if !gok || m.mode != modeGrouped {
			return "", "", false
		}
		return comment.GroupKey(m.doc.Hash, g.ID), g.Title, true
	}
	lines := m.ensureLines()
	if m.dcur < 0 || m.dcur >= len(lines) {
		return "", "", false
	}
	l := lines[m.dcur]
	switch l.Kind {
	case view.KindFile:
		fi := l.FileIdx
		if fi < 0 || fi >= len(m.doc.L.Diff.Files) {
			return "", "", false
		}
		return m.fileKey(fi), m.doc.L.Diff.Files[fi].DisplayPath(), true
	case view.KindGroup:
		if m.mode != modeGrouped {
			return "", "", false
		}
		if g, gok := m.currentGroup(); gok {
			return comment.GroupKey(m.doc.Hash, g.ID), g.Title, true
		}
	case view.KindAdd, view.KindDel, view.KindContext:
		rk, rok := m.doc.rkeys[l.HunkID]
		if !rok {
			return "", "", false
		}
		side, n := comment.LineTarget(l.Kind == view.KindDel, l.OldLine, l.NewLine)
		return comment.LineKey(rk, side, n), "", true
	}
	return "", "", false
}

// openComment is C.
func (m *Model) openComment() {
	key, target, ok := m.typedComment()
	if !ok {
		m.setFlash("move the cursor to a line, file header or group to comment on", true)
		return
	}
	prompt := "Comment: "
	if target != "" {
		prompt = "Comment on " + target + ": "
	}
	m.openInput(inputComment, prompt, "", "", key, nil)
}

// saveComment is the input overlay's enter, from finishInput.
func (m *Model) saveComment(key, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		m.setFlash("empty comment", false)
		return
	}
	now := m.o.Now()
	c := comment.Comment{ID: cache.CommentID(key, now, text), Text: text, At: now.UTC().Truncate(time.Second)}
	if m.o.Store != nil {
		var err error
		c, err = m.o.Store.AddComment(key, text, m.o.Now())
		if err != nil {
			m.setFlash("comment not saved: "+err.Error(), true)
			return
		}
		m.loadedKeys[key] = true
	}
	m.comments[key] = append(m.comments[key], c)
	m.linesOK = false
	m.setFlash("comment added", false)
}

// deleteComment is D: drop the comment row under the cursor.
func (m *Model) deleteComment() {
	ref, ok := m.commentAt[m.dcur]
	if !ok {
		m.setFlash("no comment on this line", true)
		return
	}
	if m.o.Store != nil {
		if err := m.o.Store.DeleteComment(ref.key, ref.id); err != nil {
			m.setFlash("comment not deleted: "+err.Error(), true)
			return
		}
	}
	cs := m.comments[ref.key]
	kept := make([]comment.Comment, 0, len(cs))
	for _, c := range cs {
		if c.ID != ref.id {
			kept = append(kept, c)
		}
	}
	if len(kept) > 0 {
		m.comments[ref.key] = kept
	} else {
		delete(m.comments, ref.key)
	}
	m.linesOK = false
	m.setFlash("comment deleted", false)
}

// insertCommentRows weaves comment rows into lines, one scroll per target:
// group comments after the group block, file comments after their header,
// line comments under their line. It records each row's comment in
// m.commentAt for D.
func (m *Model) insertCommentRows(lines []view.Line) []view.Line {
	m.commentAt = map[int]commentRef{}
	w := m.diffWidth()
	out := make([]view.Line, 0, len(lines)+len(m.comments)*2)
	// Group comments go after the group block's last line: find it once.
	groupEnd := -1
	if m.mode == modeGrouped {
		for i, l := range lines {
			if l.Kind != view.KindGroup {
				break
			}
			groupEnd = i
		}
	}
	var groupKey string
	if groupEnd >= 0 {
		if g, ok := m.currentGroup(); ok {
			groupKey = comment.GroupKey(m.doc.Hash, g.ID)
		}
	}
	for i, l := range lines {
		out = append(out, l)
		switch l.Kind {
		case view.KindGroup:
			if i == groupEnd && groupKey != "" {
				for _, c := range m.commentsOn(groupKey) {
					out = appendCommentRow(out, m, groupKey, c, w)
				}
			}
		case view.KindFile:
			if fk := m.fileKey(l.FileIdx); fk != "" {
				for _, c := range m.commentsOn(fk) {
					out = appendCommentRow(out, m, fk, c, w)
				}
			}
		case view.KindAdd, view.KindDel, view.KindContext:
			if l.HunkID == "" {
				continue
			}
			rk, ok := m.doc.rkeys[l.HunkID]
			if !ok {
				continue
			}
			side, n := comment.LineTarget(l.Kind == view.KindDel, l.OldLine, l.NewLine)
			key := comment.LineKey(rk, side, n)
			for _, c := range m.commentsOn(key) {
				out = appendCommentRow(out, m, key, c, w)
			}
		}
	}
	return out
}

// appendCommentRow adds the wrapped rows for one comment and records its
// first row in m.commentAt.
func appendCommentRow(out []view.Line, m *Model, key string, c comment.Comment, w int) []view.Line {
	rev := commentRef{key: key, id: c.ID}
	for i, seg := range wrap(c.Text, w-commentPrefixWidth()) {
		prefix := strings.Repeat(" ", commentPrefixWidth())
		if i == 0 {
			prefix = "✎ "
		}
		plain := prefix + seg
		row := view.Line{
			Text:  m.th.Style(m.th.Notice, plain),
			Plain: plain,
			Kind:  view.KindComment,
			// the comment follows its target's file for fileLineAt
			FileIdx: -1, Gutter: len([]rune(prefix)),
		}
		idx := len(out)
		out = append(out, row)
		if i == 0 {
			m.commentAt[idx] = rev
		}
	}
	return out
}

// commentPrefixWidth is the comment row marker.
func commentPrefixWidth() int { return 2 }
