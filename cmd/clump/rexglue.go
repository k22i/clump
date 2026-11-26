package main

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/provider"
	"github.com/k22i/clump/internal/rex"
	"github.com/k22i/clump/internal/tui"
)

func detectDark() (bool, error) {
	if t, ok := rex.ThemeFromEnv(os.Getenv); ok {
		return t != rex.ThemeLight, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return true, nil
	}
	defer tty.Close()
	t, err := rex.DetectTheme(tty, os.Getenv, 100*time.Millisecond)
	return t != rex.ThemeLight, err
}

func statusFunc() tui.StatusFunc {
	var pending string
	r := rex.NewReporter(os.Getenv, func(seq string) { pending = seq })
	if !r.Enabled() {
		return nil
	}
	return func(kind, msg string) tea.Cmd {
		st := rex.Status{Progress: -1, Msg: msg}
		switch kind {
		case "working":
			st.State = rex.StateWorking
		case "blocked":
			st.State, st.Kind = rex.StateBlocked, rex.KindQuestion
		case "done":
			st.State = rex.StateDone
		case "error":
			st.State, st.Msg = rex.StateError, provider.Redact(msg)
		default:
			st.State, st.Msg = rex.StateIdle, ""
		}
		pending = ""
		r.Report(st)
		if pending == "" {
			return nil
		}
		return tea.Raw(pending)
	}
}
