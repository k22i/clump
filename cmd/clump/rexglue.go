package main

import (
	"os"
	"strings"

	"github.com/k22i/clump/internal/tui"
)

// detectDark reports whether to use the dark theme (Phase 5 wires the
// terminal query in).
func detectDark() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("CLUMP_THEME")), "light")
}

// statusFunc returns the Rex program-status reporter (Phase 5).
func statusFunc() tui.StatusFunc { return nil }
