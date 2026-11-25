package rex

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"time"
)

// Theme is the terminal's colour scheme.
type Theme string

// Themes.
const (
	ThemeUnknown Theme = ""
	ThemeDark    Theme = "dark"
	ThemeLight   Theme = "light"
)

// ThemeQuery is written to the terminal: the colour-scheme query
// CSI ? 996 n followed by DA1 (CSI c) as a sentinel every terminal answers.
const ThemeQuery = "\x1b[?996n\x1b[c"

// themeReplyCap bounds what DetectTheme buffers from the terminal.
const themeReplyCap = 1024

// ThemeFromEnv reads CLUMP_THEME: "dark" or "light" (case-insensitive,
// trimmed); ok is false for anything else.
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

// ParseThemeReply scans terminal input received after ThemeQuery. It looks
// for the colour-scheme report CSI ? 997 ; <1|2> n (1 = dark, 2 = light) and
// for the DA1 reply CSI ? <params> c. done is true once a DA1 reply has been
// seen (a terminal answers DA1 after 996 if it supports 996). Bytes before,
// between and after are ignored, so partial reads can be re-scanned as the
// buffer grows.
func ParseThemeReply(b []byte) (t Theme, done bool) {
	for i := 0; i < len(b); i++ {
		if b[i] != 0x1b {
			continue
		}
		j := i + 1
		if j >= len(b) {
			break // a bare ESC at the end: wait for more bytes
		}
		if b[j] != '[' {
			continue // not CSI: an OSC or other escape, ignored
		}
		j++
		start := j
		for j < len(b) && b[j] >= 0x30 && b[j] <= 0x3f {
			j++ // parameter bytes
		}
		if j >= len(b) {
			break // a partial sequence at the end: ignored for now
		}
		final := b[j]
		if final < 0x40 || final > 0x7e {
			continue // not a final byte, so not a sequence: noise
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

// isDA1Reply reports whether params is the parameter string of a primary
// device-attributes reply, CSI ? <digits and ;> c.
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

// DetectTheme returns the theme from CLUMP_THEME if set, else queries the
// terminal on tty: it puts tty in raw mode, writes ThemeQuery, reads until
// ParseThemeReply is done or timeout passes, and restores the mode. It
// returns ThemeUnknown when tty is not a terminal, on timeout without a 997
// report, or on any error. Must run before the TUI takes the terminal.
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
		// The terminal mode is restored on every path, including errors and
		// timeouts. A failure here has nowhere to go: DetectTheme returns
		// only a Theme, and the caller is about to hand the terminal to the
		// TUI, which sets its own mode. Deliberately ignored.
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
			// VMIN=0/VTIME=1 makes a read return after 100 ms, so the loop
			// keeps watching the clock. An interrupt or a non-blocking fd is
			// waited out; anything else is a real error.
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
