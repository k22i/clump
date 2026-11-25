// Package config loads ~/.config/clump/config.toml.
//
// The file is optional. Built-in defaults (Default()) define two command
// providers, "claude-haiku" and "claude-sonnet", with the aliases
// cheap = "claude-haiku", strong = "claude-sonnet" and default = "cheap".
// A file overlays the defaults: top-level keys it sets replace the default
// values, and a [providers.<name>] table it defines replaces the built-in
// provider of that name wholesale (or adds a new one).
package config

import (
	_ "embed"

	"github.com/k22i/clump/internal/provider"
)

// DefaultTOML is the commented default config printed by `clump config init`.
// Loading it must give exactly Default().
//
//go:embed default.toml
var DefaultTOML string

// Config is the whole configuration.
type Config struct {
	// Default is the provider (name or alias) for one-shot mode. Default "cheap".
	Default string `toml:"default"`
	// Cheap and Strong are aliases to provider names. Watch mode auto-groups
	// with Cheap.
	Cheap  string `toml:"cheap"`
	Strong string `toml:"strong"`

	// Ceiling is the token estimate above which one-shot mode asks before
	// sending and watch mode never auto-sends. Default 40000.
	Ceiling int `toml:"ceiling"`
	// Budget is the token budget for one prompt. Default 60000.
	Budget int `toml:"budget"`
	// MaxGroups caps the model's groups. Default 12. Range 1..50.
	MaxGroups int `toml:"max_groups"`
	// AutoGroup turns on auto-grouping in watch mode. Default true.
	AutoGroup bool `toml:"auto_group"`
	// FollowWorktree makes watch follow lazygit's working-tree diffs.
	// Default false.
	FollowWorktree bool `toml:"follow_worktree"`
	// Then is the diff renderer `clump lazygit install` wraps
	// (`clump push --then '<Then>'`), e.g. "delta --paging=never". Default "".
	Then string `toml:"then"`

	// Providers by name, in file order (built-ins first).
	Providers []provider.Config `toml:"-"`
}

// Path is $CLUMP_CONFIG if set, else $XDG_CONFIG_HOME/clump/config.toml,
// else ~/.config/clump/config.toml.
func Path() (string, error) {
	panic("config: Path not implemented")
}

// Default returns the built-in configuration.
func Default() *Config {
	panic("config: Default not implemented")
}

// Load reads path. A missing file gives Default() and no error. Errors
// name the file, the line when TOML decoding fails, and the key at fault,
// e.g. `config.toml: providers.deepseek.base_url: required for type "openai"`.
func Load(path string) (*Config, error) {
	panic("config: Load not implemented")
}

// Parse is Load for TOML text; name is used in error messages.
func Parse(name, text string) (*Config, error) {
	panic("config: Parse not implemented")
}

// Validate checks c. It is called by Load/Parse.
func (c *Config) Validate() error {
	panic("config: Config.Validate not implemented")
}

// Aliases returns the alias map for provider.NewRegistry: "cheap", "strong"
// and "default" (each only when non-empty).
func (c *Config) Aliases() map[string]string {
	panic("config: Config.Aliases not implemented")
}

// Registry builds a provider.Registry from c.
func (c *Config) Registry() (*provider.Registry, error) {
	panic("config: Config.Registry not implemented")
}

// Provider returns the provider config with the given name (not alias).
func (c *Config) Provider(name string) (provider.Config, bool) {
	panic("config: Config.Provider not implemented")
}
