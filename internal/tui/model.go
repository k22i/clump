package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/engine"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/handoff"
	"github.com/k22i/clump/internal/tui/view"
)

type Options struct {
	Engine   *engine.Engine
	Store    *cache.Store
	Cfg      *config.Config
	Repo     string
	Provider string
	Theme    view.Theme
	Load     func(ctx context.Context) (*gitdiff.Loaded, error)
	Pick     bool
	PR       string
	Yes      bool
	Watch    *WatchOptions
	Status   StatusFunc
	Now      func() time.Time
}

type StatusFunc func(kind, msg string) tea.Cmd

type focus int

const (
	focusList focus = iota
	focusDiff
)

type viewMode int

const (
	modeGrouped viewMode = iota
	modeAll
)

type overlay int

const (
	ovNone overlay = iota
	ovHelp
	ovSearch
	ovConfirm
	ovTarget
	ovInput
	ovProvider
	ovPick
	ovSubmit
	ovHandoff
)

type Model struct {
	o   Options
	ctx context.Context
	th  view.Theme

	width, height int

	doc      *Doc
	provider string
	gen      int
	cancel   context.CancelFunc

	focus  focus
	mode   viewMode
	cursor int

	lines   []view.Line
	linesOK bool
	linesW  int
	scroll  int
	dcur    int

	marks    map[string]bool
	reviewed map[string]bool
	fold     map[foldKey]bool

	comments    map[string][]comment.Comment
	loadedKeys  map[string]bool
	commentAt   map[int]commentRef
	lineNumbers bool
	split       bool
	visual      bool
	visualAt    string
	query       string
	matches     []int
	ov          overlay
	confirm     confirmState
	target      targetState
	input       inputState
	menu        menuState
	pick        pickState
	submit      submitState
	handoff     handoffState
	request     *handoff.Request
	searchBox   lineInput
	spin        spinner.Model

	flash    string
	flashErr bool

	watch *watchState
}

func New(ctx context.Context, o Options) *Model {
	if o.Now == nil {
		o.Now = time.Now
	}
	sb := lineInput{prompt: "/"}
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m := &Model{o: o, ctx: ctx, th: o.Theme, provider: o.Provider, marks: map[string]bool{}, searchBox: sb, spin: sp,
		reviewed: map[string]bool{}, fold: map[foldKey]bool{}, lineNumbers: o.Cfg != nil && o.Cfg.LineNumbers,
		split:    o.Cfg != nil && o.Cfg.SideBySide,
		comments: map[string][]comment.Comment{}, loadedKeys: map[string]bool{}, commentAt: map[int]commentRef{},
		width: 100, height: 30}
	if o.Watch != nil {
		m.watch = newWatchState(o.Watch)
	}
	return m
}

func Run(ctx context.Context, o Options) (*handoff.Request, error) {
	m := New(ctx, o)
	opts := []tea.ProgramOption{tea.WithContext(ctx)}
	p := tea.NewProgram(m, opts...)
	if m.watch != nil {
		m.watch.send = p.Send
	}
	final, err := p.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		err = nil
	}
	var req *handoff.Request
	if fm, ok := final.(*Model); ok {
		if fm.cancel != nil {
			fm.cancel()
		}
		if fm.watch != nil {
			fm.watch.close()
		}
		req = fm.request
	}
	return req, err
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick}
	switch {
	case m.watch != nil:
		cmds = append(cmds, m.watch.start())
	case m.o.Pick:
		cmds = append(cmds, m.openPicker())
	case m.o.Load != nil:
		cmds = append(cmds, m.loadCmd(m.o.Load))
	}
	return tea.Batch(cmds...)
}

type loadedMsg struct {
	l   *gitdiff.Loaded
	err error
}

type groupedMsg struct {
	hash string
	gen  int
	out  *engine.Outcome
	err  error
}

type flashMsg struct{ id int }

func (m *Model) loadCmd(load func(context.Context) (*gitdiff.Loaded, error)) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		l, err := load(ctx)
		return loadedMsg{l, err}
	}
}

func (m *Model) open(l *gitdiff.Loaded, auto bool) tea.Cmd {
	if m.o.Engine != nil {
		m.o.Engine.MarkGenerated(m.ctx, l.Diff, m.o.Repo)
	}
	m.doc = newDoc(l)
	m.cursor, m.scroll, m.dcur, m.marks, m.visual = 0, 0, 0, map[string]bool{}, false
	m.fold = map[foldKey]bool{}
	m.loadReviewed()
	m.loadComments()
	m.linesOK = false
	if m.mode == modeAll {
		m.focus = focusDiff
	}
	if len(l.Diff.Hunks) == 0 {
		m.doc.state = stFailed
		m.doc.err = errors.New("the diff is empty")
		return nil
	}
	if m.loadOverride() {
		return m.status("done", plural(len(m.doc.hist.Current().Groups), "group")+" (edited)")
	}
	if m.o.Store != nil && !m.noCacheRead() {
		if out, ok, err := m.o.Engine.Lookup(l.Diff, m.provider); err == nil && ok {
			m.doc.setBase(out.Result.Grouping, out.Key, out.Provider, true, out.Result.Fallback, out.Result.Notices)
			return m.status("done", plural(len(out.Result.Groups), "group")+" (cached)")
		}
	}
	return m.requestGroup(auto)
}

func (m *Model) noCacheRead() bool { return false }

func (m *Model) loadOverride() bool {
	if m.o.Store == nil {
		return false
	}
	ov, ok, err := m.o.Store.LoadOverride(m.doc.Hash)
	if err != nil {
		m.setFlash("edits not loaded: "+err.Error(), true)
		return false
	}
	if !ok {
		return false
	}
	m.doc.snapshots = ov.Snapshots
	if len(ov.Grouping.Groups) == 0 {
		return false
	}
	if err := group.Check(m.doc.L.Diff, ov.Grouping); err != nil {
		m.doc.notices = append(m.doc.notices, "ignored saved edits that no longer match: "+err.Error())
		return false
	}
	base := ov.Grouping
	provider := "edited"
	if ov.BaseCacheKey != "" {
		if ent, ok, _ := m.o.Store.GetResult(ov.BaseCacheKey); ok && group.Check(m.doc.L.Diff, ent.Result.Grouping) == nil {
			base, provider = ent.Result.Grouping, ent.Provider
		}
	}
	m.doc.setBase(base, ov.BaseCacheKey, provider, false, false, nil)
	m.doc.hist.Reset(ov.Grouping)
	m.loadComments()
	return true
}

func (m *Model) requestGroup(auto bool) tea.Cmd {
	d := m.doc
	j, err := m.o.Engine.Prepare(engineInput(d), m.provider)
	if err != nil {
		d.state, d.err = stFailed, err
		return m.status("error", err.Error())
	}
	d.job = j
	if j.Est.OverCeiling && !m.o.Yes {
		d.state = stNeedsConfirm
		if m.watch == nil {
			m.askConfirm(fmt.Sprintf("Send %s to %s? (above the ceiling of %d tokens) [y/N]", j.Est, j.Provider, j.Est.Ceiling), confirmSend)
		}
		return m.status("blocked", fmt.Sprintf("press g to group (%s)", j.Est))
	}
	if !auto {
		d.state = stNeedsConfirm
		return m.status("idle", "")
	}
	return m.startJob(j)
}

func engineInput(d *Doc) engine.Input {
	return engine.Input{Diff: d.L.Diff, Context: d.L.Context}
}

func (m *Model) startJob(j *engine.Job) tea.Cmd {
	if m.watch != nil {
		return m.watch.request(m, j)
	}
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.gen++
	gen, hash := m.gen, m.doc.Hash
	m.doc.state, m.doc.started = stGrouping, m.o.Now()
	eng := m.o.Engine
	return tea.Batch(m.status("working", fmt.Sprintf("grouping with %s (%s)", j.Provider, j.Est)), func() tea.Msg {
		out, err := eng.Run(ctx, j, nil)
		return groupedMsg{hash: hash, gen: gen, out: out, err: err}
	})
}

func (m *Model) onGrouped(msg groupedMsg) tea.Cmd {
	if m.watch != nil {
		return m.watch.onGrouped(m, msg)
	}
	if m.doc == nil || msg.hash != m.doc.Hash || msg.gen != m.gen {
		return nil
	}
	return m.applyGrouped(msg)
}

func (m *Model) applyGrouped(msg groupedMsg) tea.Cmd {
	d := m.doc
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			d.state = stRaw
			return m.status("idle", "")
		}
		d.state, d.err = stFailed, msg.err
		return m.status("error", msg.err.Error())
	}
	d.setBase(msg.out.Result.Grouping, msg.out.Key, msg.out.Provider, false, msg.out.Result.Fallback, msg.out.Result.Notices)
	m.cursor, m.linesOK = 0, false
	m.fold = map[foldKey]bool{}
	m.loadComments()
	return m.status("done", plural(len(msg.out.Result.Groups), "group"))
}

func (m *Model) status(kind, msg string) tea.Cmd {
	if m.o.Status == nil {
		return nil
	}
	return m.o.Status(kind, msg)
}

func (m *Model) setFlash(s string, isErr bool) {
	m.flash, m.flashErr = s, isErr
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.linesOK = false
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case loadedMsg:
		if msg.err != nil {
			m.doc = &Doc{state: stFailed, err: msg.err, L: &gitdiff.Loaded{Diff: &gitdiff.Diff{}}, hist: group.NewHistory(group.Grouping{})}
			return m, m.status("error", msg.err.Error())
		}
		return m, m.open(msg.l, true)
	case groupedMsg:
		return m, m.onGrouped(msg)
	case tea.KeyPressMsg:
		return m, m.onKey(msg)
	case tea.MouseMsg:
		m.onMouse(msg)
		return m, nil
	case tea.PasteMsg:
		if m.ov == ovInput || m.ov == ovSearch {
			return m, m.forwardInput(msg)
		}
		return m, nil
	}
	if cmd, ok := m.onSideMsg(msg); ok {
		return m, cmd
	}
	if m.watch != nil {
		if cmd, ok := m.watch.onMsg(m, msg); ok {
			return m, cmd
		}
	}
	return m, nil
}

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = m.windowTitle()
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) windowTitle() string {
	if m.doc == nil || m.doc.L == nil || m.doc.L.Title == "" {
		return "clump"
	}
	return "clump: " + m.doc.L.Title
}
