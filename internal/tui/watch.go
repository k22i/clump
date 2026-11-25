package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/engine"
)

// WatchOptions configures watch mode (Phase 4).
type WatchOptions struct{}

type watchState struct {
	auto   bool
	dimmed bool
	send   func(tea.Msg)
}

func newWatchState(o *WatchOptions) *watchState                   { return &watchState{auto: true} }
func (w *watchState) start() tea.Cmd                              { return nil }
func (w *watchState) request(m *Model, j *engine.Job) tea.Cmd     { return nil }
func (w *watchState) onGrouped(m *Model, g groupedMsg) tea.Cmd    { return nil }
func (w *watchState) onMsg(m *Model, msg tea.Msg) (tea.Cmd, bool) { return nil, false }
func (w *watchState) toggleAuto(m *Model) tea.Cmd                 { return nil }
func (w *watchState) headerTag() string                           { return "" }
func (w *watchState) close()                                      {}
