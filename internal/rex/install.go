package rex

import "io"

// ConfigPath is Rex's init file: $XDG_CONFIG_HOME/rex/init.lua, else
// ~/.config/rex/init.lua.
func ConfigPath() (string, error) {
	panic("rex: ConfigPath not implemented")
}

// ModulePath is where `clump rex install` writes clump's Lua module:
// $XDG_DATA_HOME/clump/rex/init.lua, else ~/.local/share/clump/rex/init.lua.
func ModulePath() (string, error) {
	panic("rex: ModulePath not implemented")
}

// DofileLine is the line to add to Rex's init.lua:
//
//	dofile("<modulePath>") -- clump: actions clump_pick, clump_watch, clump_watch_close
//
// modulePath is written as a Lua double-quoted string (\ and " escaped).
func DofileLine(modulePath string) string {
	panic("rex: DofileLine not implemented")
}

// InstallOpts configures Install.
type InstallOpts struct {
	ConfigPath string // Rex init.lua
	ModulePath string // where the module goes
	Module     []byte // the module's contents (embedded rex/init.lua)
	Write      bool   // append the dofile line after confirmation
	In         io.Reader
	Out        io.Writer
}

// Install always writes Module to ModulePath (clump's own file, mode 0644,
// parent dirs 0755), then prints DofileLine. With Write it also asks
// "Append this line to <ConfigPath>? [y/N] " and on "y"/"yes" copies
// ConfigPath to ConfigPath + ".clump-backup-<YYYYMMDD-HHMMSS>" (if it exists)
// and appends the line (with a leading newline if the file does not end in
// one). If ConfigPath already contains the module path it says so and
// changes nothing.
func Install(o InstallOpts) error {
	panic("rex: Install not implemented")
}
