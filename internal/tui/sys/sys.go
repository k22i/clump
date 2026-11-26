package sys

import (
	"bytes"
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

const copyTimeout = 3 * time.Second

type CopiedMsg struct {
	Via string
	Err error
}

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

func CopyCmd(text string) tea.Cmd {
	native := func() tea.Msg {
		return runCopy(ClipboardArgv(runtime.GOOS, os.Getenv, exec.LookPath), text)
	}
	return tea.Batch(tea.SetClipboard(text), native)
}

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

type EditedMsg struct{ Err error }

func EditorFromEnv(getenv func(string) string) string {
	if v := strings.TrimSpace(getenv("VISUAL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(getenv("EDITOR")); v != "" {
		return v
	}
	return "vi"
}

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

func HunkTarget(h *gitdiff.Hunk) (path string, line int, ok bool) {
	if h == nil || h.Pseudo || h.File == nil || h.File.NewPath == "" {
		return "", 0, false
	}
	return h.File.Path(), hunkLine(h), true
}

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

type PaneMsg struct {
	Err    error
	Stderr string
}

func PaneCmd(argv []string, dir string, wrap func(PaneMsg) tea.Msg) tea.Cmd {
	if wrap == nil {
		wrap = func(pm PaneMsg) tea.Msg { return pm }
	}
	return func() tea.Msg {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = dir
		var errb bytes.Buffer
		cmd.Stderr = &errb
		err := cmd.Run()
		return wrap(PaneMsg{Err: err, Stderr: errb.String()})
	}
}

func LookPath(name string) (string, error) { return exec.LookPath(name) }
