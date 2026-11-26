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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/k22i/clump/internal/generated"
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
	// sending and watch mode never auto-sends. Default 125000, a little over
	// Budget so that a prompt fitted to the budget is sent without asking.
	Ceiling int `toml:"ceiling"`
	// Budget is the token budget for one prompt. Default 120000.
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
	// LineNumbers shows old/new line numbers in the diff pane (toggle with
	// # in the TUI). Default true.
	LineNumbers bool `toml:"line_numbers"`
	// SideBySide draws hunks old-left / new-right in the diff pane
	// (toggle with s in the TUI). Default false.
	SideBySide bool `toml:"side_by_side"`
	// Generated configures which files are skipped from review.
	Generated Generated `toml:"generated"`

	// Providers by name, in file order (built-ins first).
	Providers []provider.Config `toml:"-"`
}

// Generated is the [generated] table: files whose hunks are not sent to the
// model and that the TUI shows folded in a "Generated files" group. See
// package generated for the detection rules.
type Generated struct {
	// Detect turns detection on (patterns, .gitattributes linguist-generated
	// and generator markers). Default true.
	Detect bool `toml:"detect"`
	// Builtin uses clump's built-in patterns (lock files, codegen output,
	// migration snapshots, minified bundles, vendored code). Default true.
	Builtin bool `toml:"builtin"`
	// Patterns are more paths to skip (gitignore-style globs).
	Patterns []string `toml:"patterns"`
	// Review are paths that are always reviewed, even when another rule
	// says they are generated.
	Review []string `toml:"review"`
}

// Options returns g as generated.Options.
func (g Generated) Options() generated.Options {
	return generated.Options{Detect: g.Detect, Builtin: g.Builtin, Patterns: g.Patterns, Review: g.Review}
}

// pathEnv and pathXDG are the environment variables Path honours.
const (
	pathEnv = "CLUMP_CONFIG"
	pathXDG = "XDG_CONFIG_HOME"
)

// alias names, as they appear in the config file and in provider.NewRegistry.
const (
	aliasDefault = "default"
	aliasCheap   = "cheap"
	aliasStrong  = "strong"
)

// providerPrefix is the TOML prefix of a provider table.
const providerPrefix = "providers"

// envVarName matches a portable environment variable name.
var envVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// secretKeyParts are the key names that would store an API key in the file.
// They are rejected with a pointer at api_key_env.
var secretKeyParts = []string{"api_key", "key", "token", "secret"}

// rawConfig mirrors the top-level keys of Config plus the provider tables, so
// toml can decode a file without touching the built-in defaults.
type rawConfig struct {
	Default        string                     `toml:"default"`
	Cheap          string                     `toml:"cheap"`
	Strong         string                     `toml:"strong"`
	Ceiling        int                        `toml:"ceiling"`
	Budget         int                        `toml:"budget"`
	MaxGroups      int                        `toml:"max_groups"`
	AutoGroup      bool                       `toml:"auto_group"`
	FollowWorktree bool                       `toml:"follow_worktree"`
	Then           string                     `toml:"then"`
	LineNumbers    bool                       `toml:"line_numbers"`
	SideBySide     bool                       `toml:"side_by_side"`
	Generated      Generated                  `toml:"generated"`
	Providers      map[string]provider.Config `toml:"providers"`
}

// Path is $CLUMP_CONFIG if set, else $XDG_CONFIG_HOME/clump/config.toml,
// else ~/.config/clump/config.toml.
func Path() (string, error) {
	if p := os.Getenv(pathEnv); p != "" {
		return p, nil
	}
	if xdg := os.Getenv(pathXDG); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "clump", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config path: %w", err)
	}
	return filepath.Join(home, ".config", "clump", "config.toml"), nil
}

// Default returns the built-in configuration.
func Default() *Config {
	return &Config{
		Default:        aliasCheap,
		Cheap:          "claude-haiku",
		Strong:         "claude-sonnet",
		Ceiling:        125000,
		Budget:         120000,
		MaxGroups:      12,
		AutoGroup:      true,
		FollowWorktree: false,
		Then:           "",
		LineNumbers:    true,
		SideBySide:     false,
		Generated:      Generated{Detect: true, Builtin: true},
		Providers: []provider.Config{
			{
				Name:           "claude-haiku",
				Type:           provider.TypeCommand,
				Argv:           claudeArgv("haiku"),
				Format:         provider.FormatClaudeJSON,
				Model:          "haiku",
				InputPrice:     0.10,
				OutputPrice:    0.50,
				BytesPerToken:  2.0,
				OverheadTokens: 650,
				TimeoutSec:     180,
			},
			{
				Name:           "claude-sonnet",
				Type:           provider.TypeCommand,
				Argv:           claudeArgv("sonnet"),
				Format:         provider.FormatClaudeJSON,
				Model:          "sonnet",
				InputPrice:     2.0,
				OutputPrice:    10.0,
				BytesPerToken:  2.0,
				OverheadTokens: 650,
				TimeoutSec:     300,
			},
		},
	}
}

// claudeSystemPrompt is the system prompt both built-in command providers send,
// so the CLI answers with bare JSON.
const claudeSystemPrompt = "You are a JSON API. Follow the user message exactly and output only JSON."

// claudeArgv is the argv of the built-in `claude -p` provider for model: the
// CLI's own login, no tools, no repo config, and one JSON object on stdout. It
// returns a fresh slice on every call.
func claudeArgv(model string) []string {
	return []string{
		"claude",
		"-p",
		"--model", model,
		"--output-format", "json",
		"--tools", "",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--exclude-dynamic-system-prompt-sections",
		"--no-session-persistence",
		"--system-prompt", claudeSystemPrompt,
	}
}

// Load reads path. A missing file gives Default() and no error. Errors
// name the file, the line when TOML decoding fails, and the key at fault,
// e.g. `config.toml: providers.deepseek.base_url: required for type "openai"`.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Default(), nil
	case err != nil:
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return Parse(path, string(data))
}

// Parse is Load for TOML text; name is used in error messages.
func Parse(name, text string) (*Config, error) {
	var raw rawConfig
	md, err := toml.Decode(text, &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		// Undecoded is in file order, so the same bad file always reports the
		// same key.
		return nil, fmt.Errorf("%s: %w", name, unknownKeyError(undecoded[0]))
	}

	cfg := Default()
	if md.IsDefined("default") {
		cfg.Default = raw.Default
	}
	if md.IsDefined("cheap") {
		cfg.Cheap = raw.Cheap
	}
	if md.IsDefined("strong") {
		cfg.Strong = raw.Strong
	}
	if md.IsDefined("ceiling") {
		cfg.Ceiling = raw.Ceiling
	}
	if md.IsDefined("budget") {
		cfg.Budget = raw.Budget
	}
	if md.IsDefined("max_groups") {
		cfg.MaxGroups = raw.MaxGroups
	}
	if md.IsDefined("auto_group") {
		cfg.AutoGroup = raw.AutoGroup
	}
	if md.IsDefined("follow_worktree") {
		cfg.FollowWorktree = raw.FollowWorktree
	}
	if md.IsDefined("then") {
		cfg.Then = raw.Then
	}
	if md.IsDefined("line_numbers") {
		cfg.LineNumbers = raw.LineNumbers
	}
	if md.IsDefined("side_by_side") {
		cfg.SideBySide = raw.SideBySide
	}
	if md.IsDefined("generated", "detect") {
		cfg.Generated.Detect = raw.Generated.Detect
	}
	if md.IsDefined("generated", "builtin") {
		cfg.Generated.Builtin = raw.Generated.Builtin
	}
	if md.IsDefined("generated", "patterns") {
		cfg.Generated.Patterns = nilIfEmpty(raw.Generated.Patterns)
	}
	if md.IsDefined("generated", "review") {
		cfg.Generated.Review = nilIfEmpty(raw.Generated.Review)
	}
	cfg.Providers = overlayProviders(cfg.Providers, raw.Providers, md.Keys())

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return cfg, nil
}

// nilIfEmpty makes `key = []` equal to leaving the key out.
func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

// overlayProviders applies the provider tables the file defines to base: a
// built-in provider named like a table is replaced in place, wholesale, and any
// other table is appended in file order.
func overlayProviders(base []provider.Config, from map[string]provider.Config, keys []toml.Key) []provider.Config {
	out := make([]provider.Config, len(base), len(base)+len(from))
	copy(out, base)

	seen := make(map[string]bool, len(from))
	for _, key := range keys {
		// A table shows up as providers.<name> (from a [providers.<name>]
		// header) or as providers.<name>.<key> (from dotted keys); either way
		// the second part names the provider.
		if len(key) < 2 || key[0] != providerPrefix || seen[key[1]] {
			continue
		}
		name := key[1]
		seen[name] = true

		pc := from[name]
		pc.Name = name
		if i := providerIndex(out, name); i >= 0 {
			out[i] = pc
			continue
		}
		out = append(out, pc)
	}
	return out
}

// providerIndex returns the index of the provider named name in ps, or -1.
func providerIndex(ps []provider.Config, name string) int {
	for i, p := range ps {
		if p.Name == name {
			return i
		}
	}
	return -1
}

// unknownKeyError turns an undecoded key into the error to report: a key that
// would hold a secret names api_key_env instead, any other key is unknown.
func unknownKeyError(key toml.Key) error {
	dotted := key.String()
	last := key[len(key)-1]
	for _, part := range secretKeyParts {
		if last == part {
			return fmt.Errorf("%s: API keys are never stored in the config; set api_key_env to the name of an environment variable instead", dotted)
		}
	}
	return fmt.Errorf("unknown key %q", dotted)
}

// Validate checks c. It is called by Load/Parse.
func (c *Config) Validate() error {
	if c.Ceiling < 0 {
		return errors.New("ceiling: must be >= 0")
	}
	if c.Budget < 1000 {
		return errors.New("budget: must be >= 1000")
	}
	if c.MaxGroups < 1 || c.MaxGroups > 50 {
		return errors.New("max_groups: must be between 1 and 50")
	}
	for _, p := range c.Generated.Patterns {
		if _, err := generated.Compile(p); err != nil {
			return fmt.Errorf("generated.patterns: %w", err)
		}
	}
	for _, p := range c.Generated.Review {
		if _, err := generated.Compile(p); err != nil {
			return fmt.Errorf("generated.review: %w", err)
		}
	}
	for _, p := range c.Providers {
		if err := validateProvider(p); err != nil {
			return err
		}
	}
	if _, err := provider.NewRegistry(c.Providers, c.Aliases()); err != nil {
		return aliasError(err)
	}
	return nil
}

// haveHTTPURLScheme reports whether u starts with http:// or https://.
func haveHTTPURLScheme(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// validateProvider checks one provider table. Every error names the key at
// fault as providers.<name>.<key>.
func validateProvider(p provider.Config) error {
	key := func(k string) string { return providerPrefix + "." + p.Name + "." + k }

	if p.Type == "" {
		return fmt.Errorf("%s: required", key("type"))
	}
	switch p.Type {
	case provider.TypeCommand:
		if len(p.Argv) == 0 || p.Argv[0] == "" {
			return fmt.Errorf("%s: required for type %q", key("argv"), provider.TypeCommand)
		}
		switch p.Format {
		case "", provider.FormatRaw, provider.FormatClaudeJSON, provider.FormatOpencodeJSON:
		default:
			return fmt.Errorf("%s: must be one of %s, %s, %s (got %q)",
				key("format"), provider.FormatRaw, provider.FormatClaudeJSON, provider.FormatOpencodeJSON, p.Format)
		}
	case provider.TypeOpenAI:
		if p.BaseURL == "" {
			return fmt.Errorf("%s: required for type %q", key("base_url"), provider.TypeOpenAI)
		}
		if !haveHTTPURLScheme(p.BaseURL) {
			return fmt.Errorf("%s: must start with http:// or https://", key("base_url"))
		}
		if p.Model == "" {
			return fmt.Errorf("%s: required for type %q", key("model"), provider.TypeOpenAI)
		}
	case provider.TypeAnthropic:
		if p.Model == "" {
			return fmt.Errorf("%s: required for type %q", key("model"), provider.TypeAnthropic)
		}
		if p.BaseURL != "" && !haveHTTPURLScheme(p.BaseURL) {
			return fmt.Errorf("%s: must start with http:// or https://", key("base_url"))
		}
	case provider.TypeFake:
		// Nothing to check beyond the shared fields below.
	default:
		return fmt.Errorf("%s: must be one of %s, %s, %s, %s (got %q)",
			key("type"), provider.TypeCommand, provider.TypeOpenAI, provider.TypeAnthropic, provider.TypeFake, p.Type)
	}

	if p.APIKeyEnv != "" && !envVarName.MatchString(p.APIKeyEnv) {
		return fmt.Errorf("%s: must be an environment variable name", key("api_key_env"))
	}
	if p.InputPrice < 0 {
		return fmt.Errorf("%s: must be >= 0", key("input_price"))
	}
	if p.OutputPrice < 0 {
		return fmt.Errorf("%s: must be >= 0", key("output_price"))
	}
	if bpt := p.BytesPerToken; bpt != 0 && (bpt <= 0.5 || bpt > 10) {
		return fmt.Errorf("%s: must be 0 (default) or between 0.5 and 10", key("bytes_per_token"))
	}
	if p.OverheadTokens < 0 {
		return fmt.Errorf("%s: must be >= 0", key("overhead_tokens"))
	}
	if p.TimeoutSec < 0 {
		return fmt.Errorf("%s: must be >= 0", key("timeout"))
	}
	if p.MaxOutputTokens < 0 {
		return fmt.Errorf("%s: must be >= 0", key("max_output_tokens"))
	}
	if p.FakeDelayMS < 0 {
		return fmt.Errorf("%s: must be >= 0", key("fake_delay_ms"))
	}
	return nil
}

// aliasError files an error from provider.NewRegistry under the key that
// caused it: a bad provider table under "providers", a bad alias under the
// alias name.
func aliasError(err error) error {
	msg := err.Error()
	if strings.HasPrefix(msg, "provider") || strings.HasPrefix(msg, "duplicate") {
		return fmt.Errorf("%s: %w", providerPrefix, err)
	}
	for _, alias := range []string{aliasCheap, aliasStrong, aliasDefault} {
		if strings.Contains(msg, fmt.Sprintf("alias %q", alias)) {
			return fmt.Errorf("%s: %w", alias, err)
		}
	}
	return err
}

// Aliases returns the alias map for provider.NewRegistry: "cheap", "strong"
// and "default" (each only when non-empty).
func (c *Config) Aliases() map[string]string {
	out := make(map[string]string, 3)
	if c.Cheap != "" {
		out[aliasCheap] = c.Cheap
	}
	if c.Strong != "" {
		out[aliasStrong] = c.Strong
	}
	if c.Default != "" {
		out[aliasDefault] = c.Default
	}
	return out
}

// Registry builds a provider.Registry from c.
func (c *Config) Registry() (*provider.Registry, error) {
	return provider.NewRegistry(c.Providers, c.Aliases())
}

// Provider returns the provider config with the given name (not alias).
func (c *Config) Provider(name string) (provider.Config, bool) {
	if i := providerIndex(c.Providers, name); i >= 0 {
		return c.Providers[i], true
	}
	return provider.Config{}, false
}
