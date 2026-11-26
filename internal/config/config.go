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
	"github.com/k22i/clump/internal/handoff"
	"github.com/k22i/clump/internal/provider"
)

//go:embed default.toml
var DefaultTOML string

type Config struct {
	Default string `toml:"default"`
	Cheap   string `toml:"cheap"`
	Strong  string `toml:"strong"`

	Ceiling        int       `toml:"ceiling"`
	Budget         int       `toml:"budget"`
	MaxGroups      int       `toml:"max_groups"`
	AutoGroup      bool      `toml:"auto_group"`
	FollowWorktree bool      `toml:"follow_worktree"`
	Then           string    `toml:"then"`
	LineNumbers    bool      `toml:"line_numbers"`
	SideBySide     bool      `toml:"side_by_side"`
	Generated      Generated `toml:"generated"`
	Handoff        string    `toml:"handoff"`

	Providers []provider.Config `toml:"-"`
	Agents    []handoff.Agent   `toml:"-"`
}

type Generated struct {
	Detect   bool     `toml:"detect"`
	Builtin  bool     `toml:"builtin"`
	Patterns []string `toml:"patterns"`
	Review   []string `toml:"review"`
}

func (g Generated) Options() generated.Options {
	return generated.Options{Detect: g.Detect, Builtin: g.Builtin, Patterns: g.Patterns, Review: g.Review}
}

const (
	pathEnv = "CLUMP_CONFIG"
	pathXDG = "XDG_CONFIG_HOME"
)

const (
	aliasDefault = "default"
	aliasCheap   = "cheap"
	aliasStrong  = "strong"
)

const providerPrefix = "providers"

const agentsPrefix = "agents"

var envVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var secretKeyParts = []string{"api_key", "key", "token", "secret"}

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
	Handoff        string                     `toml:"handoff"`
	Providers      map[string]provider.Config `toml:"providers"`
	Agents         map[string]handoff.Agent   `toml:"agents"`
}

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
		Handoff:        "",
		Agents:         handoff.DefaultAgents(),
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

const claudeSystemPrompt = "You are a JSON API. Follow the user message exactly and output only JSON."

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

func Parse(name, text string) (*Config, error) {
	var raw rawConfig
	md, err := toml.Decode(text, &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
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
	if md.IsDefined("handoff") {
		cfg.Handoff = raw.Handoff
	}
	cfg.Providers = overlayProviders(cfg.Providers, raw.Providers, md.Keys())
	cfg.Agents = overlayAgents(cfg.Agents, raw.Agents, md.Keys())

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return cfg, nil
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func overlayProviders(base []provider.Config, from map[string]provider.Config, keys []toml.Key) []provider.Config {
	out := make([]provider.Config, len(base), len(base)+len(from))
	copy(out, base)

	seen := make(map[string]bool, len(from))
	for _, key := range keys {
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

func providerIndex(ps []provider.Config, name string) int {
	for i, p := range ps {
		if p.Name == name {
			return i
		}
	}
	return -1
}

func overlayAgents(base []handoff.Agent, from map[string]handoff.Agent, keys []toml.Key) []handoff.Agent {
	out := make([]handoff.Agent, len(base), len(base)+len(from))
	copy(out, base)
	seen := make(map[string]bool, len(from))
	for _, key := range keys {
		if len(key) < 2 || key[0] != agentsPrefix || seen[key[1]] {
			continue
		}
		name := key[1]
		seen[name] = true
		a := from[name]
		a.Name = name
		if a.Where == "" {
			a.Where = handoff.WhereReplace
		}
		if i := agentIndex(out, name); i >= 0 {
			out[i] = a
			continue
		}
		out = append(out, a)
	}
	return out
}

func agentIndex(as []handoff.Agent, name string) int {
	for i, a := range as {
		if a.Name == name {
			return i
		}
	}
	return -1
}

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
	for _, a := range c.Agents {
		if err := validateAgent(a); err != nil {
			return err
		}
	}
	if c.Handoff != "" && agentIndex(c.Agents, c.Handoff) < 0 {
		return fmt.Errorf("handoff: no agent %q in [agents] (have %s)", c.Handoff, agentNames(c.Agents))
	}
	if _, err := provider.NewRegistry(c.Providers, c.Aliases()); err != nil {
		return aliasError(err)
	}
	return nil
}

func validateAgent(a handoff.Agent) error {
	if len(a.Argv) == 0 || a.Argv[0] == "" {
		return fmt.Errorf("%s.%s.argv: required, the prompt becomes its last element", agentsPrefix, a.Name)
	}
	if !handoff.ValidWhere(a.Where) {
		return fmt.Errorf("%s.%s.where: must be one of %s, %s, %s (got %q)",
			agentsPrefix, a.Name, handoff.WhereReplace, handoff.WherePane, handoff.WherePrint, a.Where)
	}
	return nil
}

func agentNames(as []handoff.Agent) string {
	names := make([]string, 0, len(as))
	for _, a := range as {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

func (c *Config) Agent(name string) (handoff.Agent, bool) {
	if i := agentIndex(c.Agents, name); i >= 0 {
		return c.Agents[i], true
	}
	return handoff.Agent{}, false
}

func (c *Config) ResolveHandoffAgent(name string, lookPath func(string) (string, error)) (handoff.Agent, error) {
	if name == "" {
		name = c.Handoff
	}
	if name != "" {
		a, ok := c.Agent(name)
		if !ok {
			return handoff.Agent{}, fmt.Errorf("no handoff agent %q in the config", name)
		}
		return a, nil
	}
	for _, a := range c.Agents {
		if _, err := lookPath(a.Argv[0]); err == nil {
			return a, nil
		}
	}
	return handoff.Agent{}, errors.New("no handoff agent found on PATH: install one of " +
		agentNames(c.Agents) + ", or configure [agents.<name>] and set handoff = \"<name>\" (clump doctor lists what it finds)")
}

func haveHTTPURLScheme(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

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

func (c *Config) Registry() (*provider.Registry, error) {
	return provider.NewRegistry(c.Providers, c.Aliases())
}

func (c *Config) Provider(name string) (provider.Config, bool) {
	if i := providerIndex(c.Providers, name); i >= 0 {
		return c.Providers[i], true
	}
	return provider.Config{}, false
}
