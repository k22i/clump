package rex

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
)

type Theme string

const (
	ThemeUnknown Theme = ""
	ThemeDark    Theme = "dark"
	ThemeLight   Theme = "light"
)

const ThemeQuery = "\x1b[?996n\x1b[c"

const themeReplyCap = 1024

var errNoTermios = errors.New("rex: this platform has no termios")

type ReplyState int

const (
	ReplyIncomplete ReplyState = iota
	ReplyNoise
	ReplyDone
)

func (s ReplyState) String() string {
	switch s {
	case ReplyIncomplete:
		return "incomplete"
	case ReplyNoise:
		return "noise"
	case ReplyDone:
		return "done"
	}
	return "unknown"
}

func ThemeFromEnv(getenv func(string) string) (Theme, bool) {
	if getenv == nil {
		return ThemeUnknown, false
	}
	switch strings.ToLower(strings.TrimSpace(getenv("CLUMP_THEME"))) {
	case string(ThemeDark):
		return ThemeDark, true
	case string(ThemeLight):
		return ThemeLight, true
	}
	return ThemeUnknown, false
}

func ParseThemeReply(b []byte) (t Theme, state ReplyState) {
	state = ReplyNoise
	for i := 0; i < len(b); i++ {
		if b[i] != 0x1b {
			continue
		}
		j := i + 1
		if j >= len(b) {
			if state == ReplyDone {
				break
			}
			return t, ReplyIncomplete
		}
		if b[j] != '[' {
			continue
		}
		j++
		start := j
		for j < len(b) && b[j] >= 0x30 && b[j] <= 0x3f {
			j++
		}
		if j >= len(b) {
			if state == ReplyDone {
				break
			}
			return t, ReplyIncomplete
		}
		final := b[j]
		if final < 0x40 || final > 0x7e {
			continue
		}
		params := string(b[start:j])
		switch {
		case final == 'n' && params == "?997;1":
			t = ThemeDark
		case final == 'n' && params == "?997;2":
			t = ThemeLight
		case final == 'c' && isDA1Reply(params):
			state = ReplyDone
		}
		i = j
	}
	return t, state
}

func isDA1Reply(params string) bool {
	if params == "" || params[0] != '?' {
		return false
	}
	for i := 1; i < len(params); i++ {
		if c := params[i]; (c < '0' || c > '9') && c != ';' {
			return false
		}
	}
	return true
}

func DetectTheme(tty *os.File, getenv func(string) string, timeout time.Duration) (theme Theme, err error) {
	if t, ok := ThemeFromEnv(getenv); ok {
		return t, nil
	}
	if tty == nil {
		return ThemeUnknown, nil
	}
	restore, err := enterRawMode(int(tty.Fd()))
	if errors.Is(err, errNoTermios) || errors.Is(err, syscall.ENOTTY) {
		return ThemeUnknown, nil
	}
	if err != nil {
		return ThemeUnknown, fmt.Errorf("rex: put the terminal in raw mode to ask its colour scheme: %w", err)
	}
	defer func() {
		if rerr := restore(); rerr != nil {
			err = errors.Join(err, fmt.Errorf("rex: restore the terminal mode after asking its colour scheme: %w", rerr))
		}
	}()

	if _, werr := tty.WriteString(ThemeQuery); werr != nil {
		return ThemeUnknown, fmt.Errorf("rex: write the colour-scheme query: %w", werr)
	}

	buf := make([]byte, 0, 256)
	chunk := make([]byte, 256)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		room := themeReplyCap - len(buf)
		if room <= 0 {
			break
		}
		if room > len(chunk) {
			room = len(chunk)
		}
		n, rerr := tty.Read(chunk[:room])
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if t, state := ParseThemeReply(buf); state == ReplyDone {
				return t, nil
			}
		}
		if rerr != nil && n == 0 {
			if errors.Is(rerr, syscall.EINTR) {
				continue
			}
			if errors.Is(rerr, syscall.EAGAIN) || errors.Is(rerr, syscall.EWOULDBLOCK) {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return ThemeUnknown, fmt.Errorf("rex: read the colour-scheme reply: %w", rerr)
		}
	}
	return ThemeUnknown, nil
}
