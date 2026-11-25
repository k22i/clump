package rex

import (
	"os"
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

// ThemeFromEnv reads CLUMP_THEME: "dark" or "light" (case-insensitive,
// trimmed); ok is false for anything else.
func ThemeFromEnv(getenv func(string) string) (Theme, bool) {
	panic("rex: ThemeFromEnv not implemented")
}

// ParseThemeReply scans terminal input received after ThemeQuery. It looks
// for the colour-scheme report CSI ? 997 ; <1|2> n (1 = dark, 2 = light) and
// for the DA1 reply CSI ? <params> c. done is true once a DA1 reply has been
// seen (a terminal answers DA1 after 996 if it supports 996). Bytes before,
// between and after are ignored, so partial reads can be re-scanned as the
// buffer grows.
func ParseThemeReply(b []byte) (t Theme, done bool) {
	panic("rex: ParseThemeReply not implemented")
}

// DetectTheme returns the theme from CLUMP_THEME if set, else queries the
// terminal on tty: it puts tty in raw mode, writes ThemeQuery, reads until
// ParseThemeReply is done or timeout passes, and restores the mode. It
// returns ThemeUnknown when tty is not a terminal, on timeout without a 997
// report, or on any error. Must run before the TUI takes the terminal.
func DetectTheme(tty *os.File, getenv func(string) string, timeout time.Duration) Theme {
	panic("rex: DetectTheme not implemented")
}
