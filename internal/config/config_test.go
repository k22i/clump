package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/provider"
)

func providerNames(ps []provider.Config) []string {
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = p.Name
	}
	return names
}

func TestDefaultTOMLEqualsDefault(t *testing.T) {
	got, err := Parse("default.toml", DefaultTOML)
	if err != nil {
		t.Fatalf("Parse(default.toml) error: %v", err)
	}
	want := Default()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(default.toml) = %+v, want %+v", got, want)
	}
}

func TestDefaultIsFresh(t *testing.T) {
	first := Default()
	first.Providers[0].Argv[0] = "mutated"
	first.Providers[0].Name = "mutated"
	first.Cheap = "mutated"

	second := Default()
	if got := second.Providers[0].Argv[0]; got != "claude" {
		t.Errorf("Providers[0].Argv[0] = %q after mutating an earlier Default(), want %q", got, "claude")
	}
	if got := second.Providers[0].Name; got != "claude-haiku" {
		t.Errorf("Providers[0].Name = %q after mutating an earlier Default(), want %q", got, "claude-haiku")
	}
	if got := second.Cheap; got != "claude-haiku" {
		t.Errorf("Cheap = %q after mutating an earlier Default(), want %q", got, "claude-haiku")
	}
}

func TestDefaultValidates(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Errorf("Default().Validate() = %v, want nil", err)
	}
}

func TestEmptyFile(t *testing.T) {
	got, err := Parse("cfg.toml", "")
	if err != nil {
		t.Fatalf("Parse(\"\") error: %v", err)
	}
	if want := Default(); !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(\"\") = %+v, want %+v", got, want)
	}
}

func TestOverlayTopLevel(t *testing.T) {
	cfg, err := Parse("cfg.toml", "ceiling = 5000\nauto_group = false\n")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Ceiling != 5000 {
		t.Errorf("Ceiling = %d, want 5000", cfg.Ceiling)
	}
	if cfg.AutoGroup {
		t.Error("AutoGroup = true, want false")
	}
	if cfg.Budget != 120000 {
		t.Errorf("Budget = %d, want the default 120000", cfg.Budget)
	}
	if want := Default().Providers; !reflect.DeepEqual(cfg.Providers, want) {
		t.Errorf("Providers = %+v, want the defaults %+v", cfg.Providers, want)
	}
	if cfg.Cheap != "claude-haiku" || cfg.Strong != "claude-sonnet" || cfg.Default != "cheap" {
		t.Errorf("aliases = %q/%q/%q, want the defaults", cfg.Cheap, cfg.Strong, cfg.Default)
	}
}

func TestAddProvider(t *testing.T) {
	const text = `cheap = "deepseek"

[providers.deepseek]
type = "openai"
base_url = "https://api.deepseek.com/v1"
model = "deepseek-chat"
api_key_env = "DEEPSEEK_API_KEY"
`
	cfg, err := Parse("cfg.toml", text)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if want := []string{"claude-haiku", "claude-sonnet", "deepseek"}; !reflect.DeepEqual(providerNames(cfg.Providers), want) {
		t.Fatalf("provider names = %v, want %v", providerNames(cfg.Providers), want)
	}
	deepseek, ok := cfg.Provider("deepseek")
	if !ok {
		t.Fatal("Provider(\"deepseek\") not found")
	}
	if deepseek.Name != "deepseek" {
		t.Errorf("deepseek.Name = %q, want %q", deepseek.Name, "deepseek")
	}
	if deepseek.Type != provider.TypeOpenAI || deepseek.Model != "deepseek-chat" ||
		deepseek.BaseURL != "https://api.deepseek.com/v1" || deepseek.APIKeyEnv != "DEEPSEEK_API_KEY" {
		t.Errorf("deepseek = %+v, want the table from the file", deepseek)
	}
	if cfg.Cheap != "deepseek" {
		t.Errorf("Cheap = %q, want %q", cfg.Cheap, "deepseek")
	}

	reg, err := cfg.Registry()
	if err != nil {
		t.Fatalf("Registry() error: %v", err)
	}
	name, err := reg.Resolve("cheap")
	if err != nil {
		t.Fatalf("Resolve(\"cheap\") error: %v", err)
	}
	if name != "deepseek" {
		t.Errorf("Resolve(\"cheap\") = %q, want %q", name, "deepseek")
	}
	if got, want := reg.Names(), []string{"claude-haiku", "claude-sonnet", "deepseek"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Registry().Names() = %v, want %v", got, want)
	}
}

func TestReplaceBuiltin(t *testing.T) {
	cfg, err := Parse("cfg.toml", "[providers.claude-haiku]\ntype = \"fake\"\n")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if want := []string{"claude-haiku", "claude-sonnet"}; !reflect.DeepEqual(providerNames(cfg.Providers), want) {
		t.Fatalf("provider names = %v, want %v", providerNames(cfg.Providers), want)
	}
	want := provider.Config{Name: "claude-haiku", Type: provider.TypeFake}
	if !reflect.DeepEqual(cfg.Providers[0], want) {
		t.Errorf("Providers[0] = %+v, want the table from the file wholesale: %+v", cfg.Providers[0], want)
	}
	if cfg.Providers[1].Name != "claude-sonnet" {
		t.Errorf("Providers[1].Name = %q, want %q", cfg.Providers[1].Name, "claude-sonnet")
	}
}

func TestFileOrderOfNewProviders(t *testing.T) {
	const text = `[providers.zeta]
type = "fake"

[providers.alpha]
type = "fake"

[providers.mid]
type = "fake"
`
	cfg, err := Parse("cfg.toml", text)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	want := []string{"claude-haiku", "claude-sonnet", "zeta", "alpha", "mid"}
	if got := providerNames(cfg.Providers); !reflect.DeepEqual(got, want) {
		t.Errorf("provider names = %v, want file order %v", got, want)
	}
}

func TestUnknownKey(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "top-level",
			text: `colour = "x"`,
			want: `cfg.toml: unknown key "colour"`,
		},
		{
			name: "in provider table",
			text: "[providers.p]\ntype = \"fake\"\nbogus = 1\n",
			want: `cfg.toml: unknown key "providers.p.bogus"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse("cfg.toml", tt.text)
			if err == nil {
				t.Fatalf("Parse error = nil, want %q", tt.want)
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("Parse error = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIKeyInConfigRejected(t *testing.T) {
	const text = `[providers.p]
type = "openai"
base_url = "http://x"
model = "m"
api_key = "sk-123"
`
	_, err := Parse("cfg.toml", text)
	if err == nil {
		t.Fatal("Parse error = nil, want an API key rejection")
	}
	got := err.Error()
	if !strings.Contains(got, "providers.p.api_key: API keys are never stored in the config") {
		t.Errorf("Parse error = %q, want it to mention providers.p.api_key", got)
	}
	if strings.Contains(got, "sk-123") {
		t.Errorf("Parse error = %q, want it to not echo the key", got)
	}
}

func TestSyntaxError(t *testing.T) {
	_, err := Parse("cfg.toml", "ceiling = = 3")
	if err == nil {
		t.Fatal("Parse error = nil, want a syntax error")
	}
	got := err.Error()
	if !strings.HasPrefix(got, "cfg.toml: ") {
		t.Errorf("Parse error = %q, want it to start with %q", got, "cfg.toml: ")
	}
	if !strings.Contains(got, "line 1") {
		t.Errorf("Parse error = %q, want it to name the line", got)
	}
}

func TestTypeError(t *testing.T) {
	_, err := Parse("cfg.toml", `ceiling = "big"`)
	if err == nil {
		t.Fatal("Parse error = nil, want a type error")
	}
	if got := err.Error(); !strings.HasPrefix(got, "cfg.toml: ") {
		t.Errorf("Parse error = %q, want it to start with %q", got, "cfg.toml: ")
	}
}

func TestValidateErrors(t *testing.T) {
	const prefix = "cfg.toml: "
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "ceiling negative",
			text: "ceiling = -1",
			want: "ceiling: must be >= 0",
		},
		{
			name: "budget too small",
			text: "budget = 10",
			want: "budget: must be >= 1000",
		},
		{
			name: "max_groups zero",
			text: "max_groups = 0",
			want: "max_groups: must be between 1 and 50",
		},
		{
			name: "max_groups too big",
			text: "max_groups = 51",
			want: "max_groups: must be between 1 and 50",
		},
		{
			name: "provider without type",
			text: "[providers.p]\nargv = [\"x\"]\n",
			want: "providers.p.type: required",
		},
		{
			name: "provider unknown type",
			text: "[providers.p]\ntype = \"grpc\"\n",
			want: `providers.p.type: must be one of command, openai, anthropic, fake (got "grpc")`,
		},
		{
			name: "command without argv",
			text: "[providers.p]\ntype = \"command\"\n",
			want: `providers.p.argv: required for type "command"`,
		},
		{
			name: "command unknown format",
			text: "[providers.p]\ntype = \"command\"\nargv = [\"x\"]\nformat = \"xml\"\n",
			want: `providers.p.format: must be one of raw, claude-json, opencode-json (got "xml")`,
		},
		{
			name: "openai without base_url",
			text: "[providers.p]\ntype = \"openai\"\nmodel = \"m\"\n",
			want: `providers.p.base_url: required for type "openai"`,
		},
		{
			name: "openai base_url without scheme",
			text: "[providers.p]\ntype = \"openai\"\nbase_url = \"ftp://x\"\nmodel = \"m\"\n",
			want: "providers.p.base_url: must start with http:// or https://",
		},
		{
			name: "openai without model",
			text: "[providers.p]\ntype = \"openai\"\nbase_url = \"http://x\"\n",
			want: `providers.p.model: required for type "openai"`,
		},
		{
			name: "anthropic without model",
			text: "[providers.p]\ntype = \"anthropic\"\n",
			want: `providers.p.model: required for type "anthropic"`,
		},
		{
			name: "api_key_env not a name",
			text: "[providers.p]\ntype = \"fake\"\napi_key_env = \"1BAD\"\n",
			want: "providers.p.api_key_env: must be an environment variable name",
		},
		{
			name: "input_price negative",
			text: "[providers.p]\ntype = \"fake\"\ninput_price = -1\n",
			want: "providers.p.input_price: must be >= 0",
		},
		{
			name: "bytes_per_token too small",
			text: "[providers.p]\ntype = \"fake\"\nbytes_per_token = 0.2\n",
			want: "providers.p.bytes_per_token: must be 0 (default) or between 0.5 and 10",
		},
		{
			name: "timeout negative",
			text: "[providers.p]\ntype = \"fake\"\ntimeout = -5\n",
			want: "providers.p.timeout: must be >= 0",
		},
		{
			name: "cheap alias unknown",
			text: `cheap = "nope"`,
			want: `cheap: alias "cheap": unknown provider "nope"`,
		},
		{
			name: "default alias unknown",
			text: `default = "nope"`,
			want: `default: alias "default": unknown provider "nope"`,
		},
		{
			name: "strong empty is allowed",
			text: `strong = ""`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse("cfg.toml", tt.text)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Parse error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Parse error = nil, want %q", prefix+tt.want)
			}
			if got, want := err.Error(), prefix+tt.want; got != want {
				t.Errorf("Parse error = %q, want %q", got, want)
			}
		})
	}
}

func TestPath(t *testing.T) {
	const home = "/home/tester"
	homeConfig := filepath.Join(home, ".config", "clump", "config.toml")

	tests := []struct {
		name  string
		clump string
		xdg   string
		want  string
	}{
		{
			name:  "CLUMP_CONFIG wins",
			clump: "/a/b.toml",
			xdg:   "/x",
			want:  "/a/b.toml",
		},
		{
			name: "XDG_CONFIG_HOME",
			xdg:  "/x",
			want: filepath.Join("/x", "clump", "config.toml"),
		},
		{
			name: "relative XDG_CONFIG_HOME is ignored",
			xdg:  "relative/config",
			want: homeConfig,
		},
		{
			name: "neither is set",
			want: homeConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv(pathEnv, tt.clump)
			t.Setenv(pathXDG, tt.xdg)

			got, err := Path()
			if err != nil {
				t.Fatalf("Path() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error: %v", path, err)
	}
	if want := Default(); !reflect.DeepEqual(got, want) {
		t.Errorf("Load(missing) = %+v, want Default() %+v", got, want)
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("max_groups = 7\n"), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error: %v", path, err)
	}
	if cfg.MaxGroups != 7 {
		t.Errorf("MaxGroups = %d, want 7", cfg.MaxGroups)
	}

	invalid := filepath.Join(dir, "invalid.toml")
	if err := os.WriteFile(invalid, []byte("max_groups = 0\n"), 0o600); err != nil {
		t.Fatalf("write %q: %v", invalid, err)
	}
	_, err = Load(invalid)
	if err == nil {
		t.Fatal("Load(invalid) error = nil, want a validation error")
	}
	if !strings.HasPrefix(err.Error(), invalid+": ") {
		t.Errorf("Load(invalid) error = %q, want it to start with %q", err.Error(), invalid+": ")
	}
}

func TestAliases(t *testing.T) {
	got := Default().Aliases()
	want := map[string]string{"cheap": "claude-haiku", "strong": "claude-sonnet", "default": "cheap"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Default().Aliases() = %v, want %v", got, want)
	}

	cfg := Default()
	cfg.Strong = ""
	got = cfg.Aliases()
	want = map[string]string{"cheap": "claude-haiku", "default": "cheap"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Aliases() with Strong empty = %v, want %v (no strong key)", got, want)
	}
	if _, ok := got["strong"]; ok {
		t.Errorf("Aliases() with Strong empty = %v, want no %q key", got, "strong")
	}
}

func TestProviderLookup(t *testing.T) {
	cfg := Default()

	got, ok := cfg.Provider("claude-sonnet")
	if !ok {
		t.Fatal("Provider(\"claude-sonnet\") not found")
	}
	if got.Model != "sonnet" {
		t.Errorf("Provider(\"claude-sonnet\").Model = %q, want %q", got.Model, "sonnet")
	}

	if p, ok := cfg.Provider("cheap"); ok {
		t.Errorf("Provider(\"cheap\") = %+v, true; want not found (an alias is not a name)", p)
	}
	if p, ok := cfg.Provider("nope"); ok {
		t.Errorf("Provider(\"nope\") = %+v, true; want not found", p)
	}
}
