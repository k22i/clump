// Package sys is clump's glue to the outside world from the TUI: copying to
// the clipboard (OSC 52 plus a native tool) and opening $EDITOR at a hunk.
package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/gitdiff"
)

// copyTimeout bounds a native clipboard helper, so a wedged tool cannot hold
// the update loop.
const copyTimeout = 3 * time.Second

// CopiedMsg reports the result of CopyCmd.
type CopiedMsg struct {
	// Via is "osc52" or "osc52+<tool>".
	Via string
	Err error
}

// ClipboardArgv picks a native clipboard command, or nil for none.
func ClipboardArgv(goos string, getenv func(string) string, lookPath func(string) (string, error)) []string {
	found := func(name string) bool {
		_, err := lookPath(name)
		return err == nil
	}
	if goos == "darwin" && found("pbcopy") {
		return []string{"pbcopy"}
	}
	if getenv("WAYLAND_DISPLAY") != "" && found("wl-copy") {
		return []string{"wl-copy"}
	}
	if getenv("DISPLAY") != "" {
		if found("xclip") {
			return []string{"xclip", "-selection", "clipboard"}
		}
		if found("xsel") {
			return []string{"xsel", "--clipboard", "--input"}
		}
	}
	return nil
}

// CopyCmd copies text with OSC 52 and, when one exists, the native tool.
func CopyCmd(text string) tea.Cmd {
	native := func() tea.Msg {
		return runCopy(ClipboardArgv(runtime.GOOS, os.Getenv, exec.LookPath), text)
	}
	return tea.Batch(tea.SetClipboard(text), native)
}

// runCopy pipes text into argv, if there is one, and reports how the copy
// went. OSC 52 is sent by the caller either way, so a failing native tool is
// reported but does not undo the copy.
func runCopy(argv []string, text string) CopiedMsg {
	if len(argv) == 0 {
		return CopiedMsg{Via: "osc52"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), copyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return CopiedMsg{Via: "osc52", Err: fmt.Errorf("%s: %w", argv[0], err)}
	}
	return CopiedMsg{Via: "osc52+" + argv[0]}
}

// EditedMsg reports that the editor exited.
type EditedMsg struct{ Err error }

// EditorFromEnv is $VISUAL, else $EDITOR, else "vi".
func EditorFromEnv(getenv func(string) string) string {
	if v := strings.TrimSpace(getenv("VISUAL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(getenv("EDITOR")); v != "" {
		return v
	}
	return "vi"
}

// EditorArgv builds the argv that opens file at line with editor.
//
// editor is split like a POSIX shell, without expansion, so "code --wait" and
// "/Applications/VS Code.app/…/code -n" work. The flavour is chosen from the
// base name of the program: VS Code and its forks take "--goto file:line",
// Sublime and Zed take "file:line", everything else takes "+line file". A
// line of zero or less opens the file without a position.
func EditorArgv(editor, file string, line int) ([]string, error) {
	words, err := splitWords(editor)
	if err != nil {
		return nil, err
	}
	if len(words) == 0 {
		return nil, errors.New("editor: empty $EDITOR")
	}
	argv := append([]string(nil), words...)
	if line <= 0 {
		return append(argv, file), nil
	}
	target := file + ":" + strconv.Itoa(line)
	switch filepath.Base(words[0]) {
	case "code", "code-insiders", "codium", "cursor", "windsurf":
		return append(argv, "--goto", target), nil
	case "subl", "sublime_text", "zed":
		return append(argv, target), nil
	default:
		return append(argv, "+"+strconv.Itoa(line), file), nil
	}
}

// splitWords splits a command line into words the way a POSIX shell does,
// without expansion: whitespace separates words, '...' is literal, "..." only
// understands \" and \\, and a backslash outside quotes escapes the next
// character.
func splitWords(s string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		case c == '\'':
			inWord = true
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("editor: unterminated quote in %q", s)
			}
			word.WriteString(s[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			inWord = true
			i++
			for {
				if i >= len(s) {
					return nil, fmt.Errorf("editor: unterminated quote in %q", s)
				}
				if s[i] == '"' {
					break
				}
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
					i++
				}
				word.WriteByte(s[i])
				i++
			}
		case c == '\\':
			inWord = true
			if i+1 < len(s) {
				i++
				word.WriteByte(s[i])
			}
		default:
			inWord = true
			word.WriteByte(c)
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}

// HunkTarget is the file and line to open for h; ok is false when there is
// nothing to open (a pseudo-hunk or a deleted file).
func HunkTarget(h *gitdiff.Hunk) (path string, line int, ok bool) {
	if h == nil || h.Pseudo || h.File == nil || h.File.NewPath == "" {
		return "", 0, false
	}
	return h.File.Path(), hunkLine(h), true
}

// hunkLine is the new-side line number to land the cursor on: the first added
// line, else the first context line, else the hunk's new start. It is never
// less than 1.
func hunkLine(h *gitdiff.Hunk) int {
	line, firstContext := 0, 0
	n := h.NewStart
	for _, l := range h.Lines {
		switch {
		case l == "" || l[0] == ' ':
			if firstContext == 0 {
				firstContext = n
			}
			n++
		case l[0] == '+':
			line = n
			n++
		case l[0] == '-':
			// deleted line: does not advance the new side
		}
		if line != 0 {
			break
		}
	}
	if line == 0 {
		line = firstContext
	}
	if line == 0 {
		line = h.NewStart
	}
	if line < 1 {
		line = 1
	}
	return line
}

// EditCmd suspends the TUI and runs argv in dir, then sends EditedMsg.
func EditCmd(argv []string, dir string) tea.Cmd {
	if len(argv) == 0 {
		return func() tea.Msg {
			return EditedMsg{Err: errors.New("editor: empty command")}
		}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return EditedMsg{Err: err} })
}
