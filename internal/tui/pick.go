package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui/listview"
	"github.com/k22i/clump/internal/tui/view"
)

// pickItem is one choice in the --pick picker.
type pickItem struct {
	label  string
	detail string
	load   func(ctx context.Context) (*gitdiff.Loaded, error)
}

type pickState struct {
	items   []pickItem
	filter  lineInput
	cursor  int
	loading bool
	err     error
}

type pickLoadedMsg struct {
	items []pickItem
	err   error
}

// openPicker loads the choices: this branch vs its default base, then the
// recent commits.
func (m *Model) openPicker() tea.Cmd {
	m.ov = ovPick
	m.pick = pickState{loading: true, filter: lineInput{prompt: "filter: "}}
	repo := m.o.Repo
	ctx := m.ctx
	return func() tea.Msg {
		var items []pickItem
		if base, err := gitdiff.DefaultBase(ctx, repo); err == nil {
			br, _ := gitdiff.CurrentBranch(ctx, repo)
			if br != "" && br != base && "origin/"+br != base {
				items = append(items, pickItem{label: fmt.Sprintf("This branch vs %s", base), detail: br,
					load: func(ctx context.Context) (*gitdiff.Loaded, error) { return gitdiff.LoadBranch(ctx, repo) }})
			}
		}
		commits, err := gitdiff.RecentCommits(ctx, repo, 50)
		if err != nil {
			return pickLoadedMsg{err: err}
		}
		for _, c := range commits {
			sha := c.Hash
			items = append(items, pickItem{label: c.Short + " " + c.Subject, detail: c.RelDate,
				load: func(ctx context.Context) (*gitdiff.Loaded, error) { return gitdiff.LoadRev(ctx, repo, sha) }})
		}
		return pickLoadedMsg{items: items}
	}
}

func (m *Model) onPickLoaded(msg pickLoadedMsg) {
	m.pick.loading, m.pick.items, m.pick.err = false, msg.items, msg.err
}

// filtered returns the items matching the filter.
func (p *pickState) filtered() []pickItem {
	q := strings.ToLower(strings.TrimSpace(p.filter.text()))
	if q == "" {
		return p.items
	}
	var out []pickItem
	for _, it := range p.items {
		if strings.Contains(strings.ToLower(it.label+" "+it.detail), q) {
			out = append(out, it)
		}
	}
	return out
}

func (m *Model) keyPick(k tea.KeyPressMsg) tea.Cmd {
	p := &m.pick
	items := p.filtered()
	switch k.String() {
	case "esc", "ctrl+c":
		if m.doc == nil {
			return m.quit()
		}
		m.ov = ovNone
		return nil
	case "down", "ctrl+n", "ctrl+j":
		p.cursor = min(p.cursor+1, len(items)-1)
		return nil
	case "up", "ctrl+p", "ctrl+k":
		p.cursor = max(p.cursor-1, 0)
		return nil
	case "enter":
		if p.cursor < 0 || p.cursor >= len(items) {
			return nil
		}
		m.ov = ovNone
		m.doc = &Doc{state: stLoading}
		return m.loadCmd(items[p.cursor].load)
	}
	p.filter.key(k)
	p.cursor = 0
	return nil
}

func (m *Model) renderPick(w, h int) string {
	p := &m.pick
	lines := []string{"", "  " + m.th.Style(m.th.Title, "clump: what do you want to review?"), "  " + p.filter.view(w-4, m.th, true), ""}
	switch {
	case p.loading:
		lines = append(lines, "  "+m.spin.View()+" reading the repository…")
	case p.err != nil:
		lines = append(lines, "  "+m.th.Style(m.th.Error, "error: "+p.err.Error()))
	default:
		items := p.filtered()
		start, end := listview.Window(len(items), p.cursor, max(1, h-6))
		for i := start; i < end; i++ {
			it := items[i]
			row := view.Truncate(fmt.Sprintf("  %s  %s", it.label, m.th.Style(m.th.Dim, it.detail)), w)
			if i == p.cursor {
				row = m.th.Style(m.th.Selected, view.Truncate("▸ "+it.label+"  "+it.detail, w))
			}
			lines = append(lines, row)
		}
		if len(items) == 0 {
			lines = append(lines, "  (nothing matches)")
		}
	}
	lines = append(lines, "", "  "+m.th.Style(m.th.Dim, "type to filter · ↑/↓ · enter open · esc quit"))
	return strings.Join(lines, "\n")
}
