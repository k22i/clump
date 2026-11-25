//go:build !darwin && !linux

package rex

// enterRawMode has no termios to work with on this platform, so DetectTheme
// reports ThemeUnknown (or the CLUMP_THEME override) without querying.
func enterRawMode(fd int) (restore func() error, ok bool) {
	return nil, false
}

// termiosSnapshot has nothing to report on this platform.
func termiosSnapshot(fd int) (snap string, ok bool) {
	return "", false
}
