package lazygit

import "io"

// PushCommand returns the renderer command lazygit should run:
// "clump push", or "clump push --then '<then>'" with then single-quoted for
// a POSIX shell: each single quote inside then becomes the four bytes
// quote, backslash, quote, quote.
func PushCommand(then string) string {
	panic("lazygit: PushCommand not implemented")
}

// Snippet returns the YAML to add to lazygit's config.yml:
//
//	git:
//	  diffRenderers:
//	    - command: <PushCommand(then)>
//	      name: clump
//
// The command value is YAML double-quoted.
func Snippet(then string) string {
	panic("lazygit: Snippet not implemented")
}

// ConfigPath returns lazygit's config file: $LG_CONFIG_FILE (first entry if
// comma-separated), else $XDG_CONFIG_HOME/lazygit/config.yml, else
// ~/Library/Application Support/lazygit/config.yml on darwin when that file
// exists, else ~/.config/lazygit/config.yml.
func ConfigPath() (string, error) {
	panic("lazygit: ConfigPath not implemented")
}

// InstallOpts configures Install.
type InstallOpts struct {
	// Path is the lazygit config file.
	Path string
	// Then is the renderer to keep when the config has none (config.Then).
	Then string
	// Write applies the change; otherwise Install only prints it.
	Write bool
	// In and Out are used for the y/N confirmation and messages.
	In  io.Reader
	Out io.Writer
}

// Install prints, or with Write and a "y" answer applies, the lazygit
// config change. See DESIGN.md "lazygit install" for the editing rules.
func Install(o InstallOpts) error {
	panic("lazygit: Install not implemented")
}
