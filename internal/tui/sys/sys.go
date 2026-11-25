// Package sys is clump's glue to the outside world from the TUI: copying to
// the clipboard (OSC 52 plus a native tool) and opening $EDITOR at a hunk.
package sys

import (
	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/gitdiff"
)

// CopiedMsg reports the result of CopyCmd.
type CopiedMsg struct {
	// Via is "osc52" or "osc52+<tool>".
	Via string
	Err error
}

// ClipboardArgv picks a native clipboard command, or nil for none.
func ClipboardArgv(goos string, getenv func(string) string, lookPath func(string) (string, error)) []string {
	panic("sys: ClipboardArgv not implemented")
}

// CopyCmd copies text with OSC 52 and, when one exists, the native tool.
func CopyCmd(text string) tea.Cmd {
	panic("sys: CopyCmd not implemented")
}

// EditedMsg reports that the editor exited.
type EditedMsg struct{ Err error }

// EditorFromEnv is $VISUAL, else $EDITOR, else "vi".
func EditorFromEnv(getenv func(string) string) string {
	panic("sys: EditorFromEnv not implemented")
}

// EditorArgv builds the argv that opens file at line with editor.
func EditorArgv(editor, file string, line int) ([]string, error) {
	panic("sys: EditorArgv not implemented")
}

// HunkTarget is the file and line to open for h; ok is false when there is
// nothing to open (a pseudo-hunk or a deleted file).
func HunkTarget(h *gitdiff.Hunk) (path string, line int, ok bool) {
	panic("sys: HunkTarget not implemented")
}

// EditCmd suspends the TUI and runs argv in dir, then sends EditedMsg.
func EditCmd(argv []string, dir string) tea.Cmd {
	panic("sys: EditCmd not implemented")
}
