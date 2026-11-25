// Package deps pins the module's whole dependency set in go.mod while the
// packages that use them are still stubs. Delete it once every dependency
// is imported for real.
package deps

import (
	_ "charm.land/bubbles/v2/viewport"
	_ "charm.land/bubbletea/v2"
	_ "charm.land/lipgloss/v2"
	_ "github.com/BurntSushi/toml"
	_ "github.com/alecthomas/chroma/v2"
	_ "golang.org/x/sys/unix"
)
