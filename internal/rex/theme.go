package rex

import (
	"errors"
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

func ParseThemeReply(b []byte) (t Theme, done bool) {
	for i := 0; i < len(b); i++ {
		if b[i] != 0x1b {
			continue
		}
		j := i + 1
		if j >= len(b) {
			break
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
			break
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
			done = true
		}
		i = j
	}
	return t, done
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

func DetectTheme(tty *os.File, getenv func(string) string, timeout time.Duration) Theme {
	if t, ok := ThemeFromEnv(getenv); ok {
		return t
	}
	if tty == nil {
		return ThemeUnknown
	}
	restore, ok := enterRawMode(int(tty.Fd()))
	if !ok {
		return ThemeUnknown
	}
	defer func() {
		_ = restore()
	}()

	if _, err := tty.WriteString(ThemeQuery); err != nil {
		return ThemeUnknown
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
		n, err := tty.Read(chunk[:room])
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if t, done := ParseThemeReply(buf); done {
				return t
			}
		}
		if err != nil && n == 0 {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return ThemeUnknown
		}
	}
	return ThemeUnknown
}
