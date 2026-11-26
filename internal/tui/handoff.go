package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/handoff"
	"github.com/k22i/clump/internal/tui/sys"
)

type handoffState struct {
	items  []menuItem
	cursor int
	title  string
}

type handoffMsg struct {
	agent  string
	via    string
	err    error
	stderr string
}

const handoffPrintItem = "print"

func (m *Model) handoffWhere(a handoff.Agent) string {
	if m.watch != nil && a.Where == handoff.WhereReplace {
		return handoff.WherePane
	}
	return a.Where
}

func (m *Model) openHandoff() tea.Cmd {
	if m.doc == nil || m.doc.L == nil || len(m.doc.L.Diff.Hunks) == 0 {
		return nil
	}
	placed := m.reportComments()
	if len(placed) == 0 {
		m.setFlash("no comments to hand off: write them with C", true)
		return nil
	}
	var items []menuItem
	for _, a := range m.o.Cfg.Agents {
		if m.watch != nil && a.Where == handoff.WherePrint {
			continue
		}
		items = append(items, menuItem{name: a.Name, label: a.Name + " — " + m.handoffWhere(a)})
	}
	if m.watch == nil {
		items = append(items, menuItem{name: handoffPrintItem, label: "print — quit and print the prompt, launch nothing"})
	}
	if len(items) == 0 {
		m.setFlash("no coding agent to hand off to: configure [agents.<name>] or install claude, codex or opencode", true)
		return nil
	}
	m.handoff = handoffState{
		items:  items,
		cursor: 0,
		title:  fmt.Sprintf("Hand off %s from %s", plural(len(placed), "comment"), handoffRef(m.doc.L.Ref)),
	}
	m.ov = ovHandoff
	return nil
}

func handoffRef(ref string) string {
	if ref == "" || ref == "stdin" {
		return "this diff"
	}
	return ref
}

func (m *Model) keyHandoff(key string) tea.Cmd {
	s := &m.handoff
	switch key {
	case "j", "down":
		s.cursor = min(s.cursor+1, len(s.items)-1)
	case "k", "up":
		s.cursor = max(s.cursor-1, 0)
	case "esc", "q", "H":
		m.ov = ovNone
		return nil
	case "enter":
		m.ov = ovNone
		if s.cursor >= 0 && s.cursor < len(s.items) {
			return m.pickHandoff(s.items[s.cursor].name)
		}
	}
	return nil
}

func (m *Model) handoffPrompt() (string, []string) {
	placed := m.reportComments()
	return handoff.Prompt(handoff.Review{
		Title: m.doc.L.Title, Ref: m.doc.L.Ref,
		Comments: placed, GroupTitle: m.groupTitle(),
	}), handoff.AnchorKeys(placed)
}

func (m *Model) pickHandoff(name string) tea.Cmd {
	prompt, anchors := m.handoffPrompt()
	if name == handoffPrintItem {
		m.request = &handoff.Request{Where: handoff.WherePrint, Prompt: prompt, Dir: m.o.Repo, Anchors: anchors}
		return tea.Quit
	}
	a, ok := m.o.Cfg.Agent(name)
	if !ok {
		m.setFlash("no agent "+name, true)
		return nil
	}
	if m.handoffWhere(a) == handoff.WherePrint {
		m.request = &handoff.Request{Agent: a.Name, Where: handoff.WherePrint, Prompt: prompt, Dir: m.o.Repo, Anchors: anchors}
		return tea.Quit
	}
	path, err := sys.LookPath(a.Argv[0])
	if err != nil {
		m.setFlash("handoff: "+a.Argv[0]+" is not on PATH (clump doctor lists what it finds)", true)
		return nil
	}
	switch m.handoffWhere(a) {
	case handoff.WhereReplace:
		m.request = &handoff.Request{Agent: a.Name, Where: handoff.WhereReplace,
			Argv: a.ArgvWith(prompt), Path: path, Prompt: prompt, Dir: m.o.Repo, Anchors: anchors}
		return tea.Quit
	case handoff.WherePane:
		return m.handoffPane(a, prompt, anchors)
	}
	return nil
}

func (m *Model) handoffPane(a handoff.Agent, prompt string, anchors []string) tea.Cmd {
	paneArgv, via, ok := handoff.PaneArgv(handoff.PaneEnvironment(os.Getenv), m.o.Repo, handoff.LabelFor(a.Name), a.ArgvWith(prompt))
	if !ok {
		m.setFlash("no Rex or tmux session to open a pane in (--where print prints the prompt)", true)
		return nil
	}
	m.setFlash("handing off to "+a.Name+"…", false)
	agent, dir, store := a.Name, m.o.Repo, m.o.Store
	now := m.o.Now
	return sys.PaneCmd(paneArgv, dir, func(pm sys.PaneMsg) tea.Msg {
		if pm.Err == nil && store != nil && len(anchors) > 0 {
			_, _ = store.StampHandoff(anchors, now())
		}
		return handoffMsg{agent: agent, via: via, err: pm.Err, stderr: pm.Stderr}
	})
}

func (m *Model) onHandoffMsg(msg handoffMsg) tea.Cmd {
	if msg.err != nil {
		reason := msg.err.Error()
		if msg.stderr != "" {
			reason = msg.stderr
		}
		m.setFlash("handoff to "+msg.agent+" failed: "+reason+" (quit and run `clump handoff --where print` for the prompt)", true)
		return nil
	}
	m.setFlash("handed off to "+msg.agent+" in a new pane; the comments are stamped", false)
	return m.status("done", "handed off to "+msg.agent)
}
