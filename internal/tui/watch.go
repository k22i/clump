package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/engine"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/ipc"
	"github.com/k22i/clump/internal/lazygit"
)

const DefaultDebounce = 400 * time.Millisecond

type WatchOptions struct {
	Repo           string
	Listener       *ipc.Listener
	Debounce       time.Duration
	AutoGroup      bool
	FollowWorktree bool
	Log            io.Writer
	Resolve        func(ctx context.Context, repo string, sel lazygit.Selection, diff string) (*gitdiff.Loaded, error)
}

type watchState struct {
	o      *WatchOptions
	auto   bool
	dimmed bool
	send   func(tea.Msg)

	ctx    context.Context
	cancel context.CancelFunc

	newest  int64
	pending *ipc.Msg
	seq     int

	lastSel lazygit.Selection

	inflight *inflightJob
	queued   *engine.Job
	gen      int

	logMu sync.Mutex
	once  sync.Once
}

type inflightJob struct {
	hash string
	gen  int
}

type pushMsg struct{ m ipc.Msg }
type debounceMsg struct{ seq int }
type resolvedMsg struct {
	seq int
	sel lazygit.Selection
	l   *gitdiff.Loaded
	err error
}

func newWatchState(o *WatchOptions) *watchState {
	if o.Debounce <= 0 {
		o.Debounce = DefaultDebounce
	}
	if o.Resolve == nil {
		o.Resolve = resolveSelection
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &watchState{o: o, auto: o.AutoGroup, ctx: ctx, cancel: cancel}
}

func (w *watchState) logf(format string, args ...any) {
	if w.o.Log == nil {
		return
	}
	w.logMu.Lock()
	defer w.logMu.Unlock()
	fmt.Fprintf(w.o.Log, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}

func (w *watchState) start() tea.Cmd {
	w.logf("watch start repo=%s auto=%v", w.o.Repo, w.auto)
	if w.o.Listener != nil && w.send != nil {
		l, send := w.o.Listener, w.send
		go func() {
			if err := l.Serve(w.ctx, func(m ipc.Msg) { send(pushMsg{m}) }); err != nil {
				w.logf("listener error: %v", err)
			}
		}()
	}
	return nil
}

func (w *watchState) close() {
	w.once.Do(func() {
		w.cancel()
		if w.o.Listener != nil {
			_ = w.o.Listener.Close()
		}
		w.logf("watch stop")
	})
}

func (w *watchState) headerTag() string {
	tag := "auto off (A)"
	if w.auto {
		tag = "auto on"
	}
	if w.dimmed {
		tag += " · not followed"
	}
	return tag
}

func (w *watchState) onMsg(m *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case pushMsg:
		return w.onPush(msg.m), true
	case debounceMsg:
		return w.onDebounce(m, msg.seq), true
	case resolvedMsg:
		return w.onResolved(m, msg), true
	}
	return nil, false
}

func (w *watchState) onPush(p ipc.Msg) tea.Cmd {
	if p.SentNS < w.newest {
		w.logf("push dropped: out of order (sent_ns %d < %d)", p.SentNS, w.newest)
		return nil
	}
	w.newest = p.SentNS
	w.pending = &p
	w.seq++
	seq := w.seq
	w.logf("push seq=%d bytes=%d argv=%q", seq, len(p.Diff), strings.Join(p.GitArgv, " "))
	return tea.Tick(w.o.Debounce, func(time.Time) tea.Msg { return debounceMsg{seq} })
}

func (w *watchState) onDebounce(m *Model, seq int) tea.Cmd {
	if seq != w.seq || w.pending == nil {
		return nil
	}
	p := *w.pending
	w.pending = nil
	sel := lazygit.Classify(p.GitArgv, p.Diff)
	act := lazygit.Decide(sel, w.o.FollowWorktree)
	w.logf("select seq=%d kind=%s ref=%s action=%s", seq, sel.Kind, sel.Ref, act)
	switch act {
	case lazygit.Dim:
		if m.doc != nil {
			w.dimmed = true
		}
		return nil
	case lazygit.CursorOnly:
		w.cursorTo(m, sel)
		return nil
	}
	repo, resolve, ctx := w.o.Repo, w.o.Resolve, w.ctx
	return func() tea.Msg {
		l, err := resolve(ctx, repo, sel, p.Diff)
		return resolvedMsg{seq: seq, sel: sel, l: l, err: err}
	}
}

func (w *watchState) cursorTo(m *Model, sel lazygit.Selection) {
	if m.doc == nil || w.lastSel.Kind != lazygit.KindCommit || w.lastSel.Commit != sel.Commit || len(sel.Paths) == 0 {
		if m.doc != nil {
			w.dimmed = true
		}
		return
	}
	w.dimmed = false
	path := sel.Paths[0]
	for _, h := range m.doc.L.Diff.Hunks {
		if h.File.Path() == path || h.File.OldPath == path {
			if gi := m.doc.hist.Current().GroupOf(h.ID); gi >= 0 {
				m.setCursor(gi)
				w.logf("cursor file=%s group=%d", path, gi+1)
			}
			return
		}
	}
}

func (w *watchState) onResolved(m *Model, r resolvedMsg) tea.Cmd {
	if r.seq != w.seq {
		w.logf("resolve dropped: superseded (seq %d < %d)", r.seq, w.seq)
		return nil
	}
	if r.err != nil {
		w.logf("resolve error: %v", r.err)
		m.setFlash("cannot follow this selection: "+r.err.Error(), true)
		if m.doc != nil {
			w.dimmed = true
		}
		return nil
	}
	if len(r.l.Diff.Hunks) == 0 {
		w.logf("select empty diff ref=%s", r.sel.Ref)
		if m.doc != nil {
			w.dimmed = true
		}
		return nil
	}
	hash := r.l.Diff.Hash()
	if m.doc != nil && m.doc.Hash == hash {
		w.dimmed = false
		w.lastSel = r.sel
		w.logf("dedupe hash=%s", hash[:12])
		return nil
	}
	w.lastSel, w.dimmed = r.sel, false
	w.queued = nil
	cmd := m.open(r.l, w.auto)
	switch d := m.doc; {
	case d.state == stGrouped && d.edited():
		w.logf("show hash=%s edited", hash[:12])
	case d.state == stGrouped && d.cached:
		w.logf("show hash=%s cached", hash[:12])
	case d.state == stNeedsConfirm && d.job != nil:
		w.logf("show hash=%s waiting for g (%s, over ceiling=%v, auto=%v)", hash[:12], d.job.Est, d.job.Est.OverCeiling, w.auto)
	}
	return cmd
}

func (w *watchState) request(m *Model, j *engine.Job) tea.Cmd {
	m.doc.state, m.doc.started = stGrouping, m.o.Now()
	if w.inflight != nil {
		if w.inflight.hash == j.DiffHash {
			w.logf("call join hash=%s (already in flight)", j.DiffHash[:12])
			w.queued = nil
			return nil
		}
		w.queued = j
		w.logf("call queued hash=%s", j.DiffHash[:12])
		return nil
	}
	return w.run(m, j)
}

func (w *watchState) run(m *Model, j *engine.Job) tea.Cmd {
	w.gen++
	w.inflight = &inflightJob{hash: j.DiffHash, gen: w.gen}
	gen, ctx, eng := w.gen, w.ctx, m.o.Engine
	w.logf("call start hash=%s provider=%s est=%q", j.DiffHash[:12], j.Provider, j.Est.String())
	return tea.Batch(m.status("working", fmt.Sprintf("grouping with %s (%s)", j.Provider, j.Est)), func() tea.Msg {
		out, err := eng.Run(ctx, j, nil)
		return groupedMsg{hash: j.DiffHash, gen: gen, out: out, err: err}
	})
}

func (w *watchState) onGrouped(m *Model, msg groupedMsg) tea.Cmd {
	if w.inflight == nil || msg.gen != w.inflight.gen {
		return nil
	}
	w.inflight = nil
	switch {
	case msg.err != nil:
		w.logf("call end hash=%s error=%q", msg.hash[:12], msg.err.Error())
	default:
		w.logf("call end hash=%s groups=%d calls=%d cost=%.6f source=%s elapsed=%s",
			msg.hash[:12], len(msg.out.Result.Groups), msg.out.Calls, msg.out.Usage.CostUSD, msg.out.Usage.CostSource, msg.out.Elapsed.Round(time.Millisecond))
		if msg.out.Result.Fallback || len(msg.out.Result.Notices) > 0 {
			raw := msg.out.Raw
			if len(raw) > 400 {
				raw = raw[:400] + "…"
			}
			w.logf("call notices=%q raw=%q", msg.out.Result.Notices, raw)
		}
	}
	var cmds []tea.Cmd
	if m.doc != nil && m.doc.Hash == msg.hash && m.doc.state == stGrouping {
		if errors.Is(msg.err, context.Canceled) {
			return nil
		}
		cmds = append(cmds, m.applyGrouped(msg))
	} else if msg.err == nil {
		w.logf("call stale hash=%s (cached, not shown)", msg.hash[:12])
	}
	if j := w.queued; j != nil {
		w.queued = nil
		if m.doc != nil && j.DiffHash == m.doc.Hash && m.doc.state == stGrouping {
			if out, ok, err := m.o.Engine.Lookup(m.doc.L.Diff, m.provider); err == nil && ok {
				m.doc.setBase(out.Result.Grouping, out.Key, out.Provider, true, out.Result.Fallback, out.Result.Notices)
				w.logf("queued call satisfied by the cache hash=%s", j.DiffHash[:12])
			} else {
				cmds = append(cmds, w.run(m, j))
			}
		} else {
			w.logf("queued call dropped hash=%s", j.DiffHash[:12])
		}
	}
	return tea.Batch(cmds...)
}

func (w *watchState) toggleAuto(m *Model) tea.Cmd {
	w.auto = !w.auto
	w.logf("auto=%v", w.auto)
	if !w.auto {
		m.setFlash("auto-group off: diffs are no longer sent; g groups this one", false)
		return nil
	}
	m.setFlash("auto-group on with "+m.providerLabel(), false)
	if d := m.doc; d != nil && d.state == stNeedsConfirm && d.job != nil && !d.job.Est.OverCeiling {
		return m.startJob(d.job)
	}
	return nil
}

func resolveSelection(ctx context.Context, repo string, sel lazygit.Selection, diff string) (*gitdiff.Loaded, error) {
	d, err := gitdiff.Parse(diff)
	if errors.Is(err, gitdiff.ErrCombined) || (err == nil && sel.Kind == lazygit.KindCommit && len(d.Files) == 0 && len(d.Commits) > 0 && d.Commits[0].Merge) {
		return gitdiff.LoadRev(ctx, repo, sel.Commit)
	}
	if err != nil {
		return nil, err
	}
	l := &gitdiff.Loaded{Diff: d, Raw: diff}
	switch sel.Kind {
	case lazygit.KindCommit:
		hash, subject, msg, err := gitdiff.CommitMessage(ctx, repo, sel.Commit)
		if err != nil {
			return nil, err
		}
		l.Title, l.Ref, l.Context = subject, gitdiff.ShortRev(hash), gitdiff.CommitContext(msg)
	case lazygit.KindStash:
		hash, subject, msg, err := gitdiff.CommitMessage(ctx, repo, sel.Stash)
		if err != nil {
			return nil, err
		}
		name := strings.TrimPrefix(sel.Stash, "refs/")
		l.Title, l.Ref, l.Context = name+": "+subject, gitdiff.ShortRev(hash), gitdiff.CommitContext(msg)
	case lazygit.KindRange, lazygit.KindDiffMode:
		l.Ref = gitdiff.ShortRev(sel.From) + ".." + gitdiff.ShortRev(sel.To)
		l.Title = gitdiff.ShortRev(sel.From) + " vs " + gitdiff.ShortRev(sel.To)
		if sel.Kind == lazygit.KindRange {
			l.Title = "range " + l.Ref
		}
		l.Context = gitdiff.RangeContext(ctx, repo, sel.From, sel.To)
	case lazygit.KindWorktree:
		l.Title, l.Ref = "working tree", "worktree"
		if sel.Cached {
			l.Title, l.Ref = "staged changes", "staged"
		}
	default:
		l.Title, l.Ref = "selection", sel.Ref
	}
	return l, nil
}
