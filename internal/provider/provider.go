// Package provider talks to the LLMs that group hunks: a command provider
// (an argv with the prompt on stdin), an OpenAI-compatible HTTP provider, the
// Anthropic Messages API, and a fake for tests.
//
// API keys are never in config. A provider reads its key from the env var
// named by Config.APIKeyEnv, and every error it returns passes through Redact.
package provider

import (
	"context"
	"errors"
)

// Usage is what one Complete call consumed.
type Usage struct {
	// InputTokens includes cache-read and cache-write tokens.
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CacheTokens  int     `json:"cache_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd"`
	// CostSource is CostReported when the provider reported the cost, or
	// CostEstimated when it was computed from the token counts and Config prices.
	CostSource string `json:"cost_source"`
}

// Cost sources for Usage.CostSource.
const (
	CostReported  = "reported"
	CostEstimated = "estimated"
)

// Add returns the field-wise sum of u and v. CostSource is CostReported only
// if both are (an empty CostSource counts as neutral).
func (u Usage) Add(v Usage) Usage {
	out := Usage{
		InputTokens:  u.InputTokens + v.InputTokens,
		OutputTokens: u.OutputTokens + v.OutputTokens,
		CacheTokens:  u.CacheTokens + v.CacheTokens,
		CostUSD:      u.CostUSD + v.CostUSD,
	}
	switch {
	case u.CostSource == "":
		out.CostSource = v.CostSource
	case v.CostSource == "" || u.CostSource == v.CostSource:
		out.CostSource = u.CostSource
	default:
		out.CostSource = CostEstimated
	}
	return out
}

// Provider completes one prompt. Implementations are safe for concurrent use.
type Provider interface {
	// Name is the configured provider name (e.g. "claude-haiku").
	Name() string
	// Complete sends prompt and returns the model's text output. It honours
	// ctx cancellation. Errors never contain API keys.
	Complete(ctx context.Context, prompt string) (text string, u Usage, err error)
}

// Provider types for Config.Type.
const (
	TypeCommand   = "command"
	TypeOpenAI    = "openai"
	TypeAnthropic = "anthropic"
	TypeFake      = "fake"
)

// Output formats for a command provider (Config.Format).
const (
	FormatRaw          = "raw"           // stdout is the model text
	FormatClaudeJSON   = "claude-json"   // claude -p --output-format json
	FormatOpencodeJSON = "opencode-json" // opencode run --format json (JSONL events)
)

// Config is one [providers.<name>] table from config.toml.
type Config struct {
	// Name is the table name; filled in by the config loader, not from TOML.
	Name string `toml:"-" json:"name"`
	// Type is one of TypeCommand, TypeOpenAI, TypeAnthropic, TypeFake.
	Type string `toml:"type" json:"type"`

	// Argv is the command (TypeCommand). The prompt goes on stdin.
	Argv []string `toml:"argv" json:"argv,omitempty"`
	// Format is how a command's stdout is read (FormatRaw by default).
	Format string `toml:"format" json:"format,omitempty"`
	// Dir is the command's working directory. Default os.TempDir(), so an
	// agent CLI never picks up repo context (CLAUDE.md, AGENTS.md).
	Dir string `toml:"dir" json:"dir,omitempty"`

	// Model is the model name. Required for TypeOpenAI and TypeAnthropic;
	// informational for TypeCommand (it is part of the cache key).
	Model string `toml:"model" json:"model,omitempty"`
	// BaseURL is the OpenAI-compatible base, e.g. "https://api.deepseek.com/v1".
	// The provider POSTs to BaseURL + "/chat/completions". For TypeAnthropic it
	// defaults to "https://api.anthropic.com" and the provider POSTs to
	// BaseURL + "/v1/messages".
	BaseURL string `toml:"base_url" json:"base_url,omitempty"`
	// APIKeyEnv names the env var holding the key. TypeAnthropic defaults to
	// "ANTHROPIC_API_KEY". Empty for TypeOpenAI means no Authorization header
	// (e.g. local Ollama).
	APIKeyEnv string `toml:"api_key_env" json:"api_key_env,omitempty"`

	// InputPrice and OutputPrice are USD per million tokens, for estimates.
	InputPrice  float64 `toml:"input_price" json:"input_price"`
	OutputPrice float64 `toml:"output_price" json:"output_price"`
	// BytesPerToken is the prompt-size heuristic used before sending.
	// Default 2.0 for Claude models, 3.3 otherwise (see DefaultBytesPerToken).
	BytesPerToken float64 `toml:"bytes_per_token" json:"bytes_per_token"`
	// OverheadTokens is the fixed per-call input the tool adds (the claude CLI
	// adds ~650). Estimates add it; calibration subtracts it.
	OverheadTokens int `toml:"overhead_tokens" json:"overhead_tokens"`
	// TimeoutSec bounds one call. Default 180.
	TimeoutSec int `toml:"timeout" json:"timeout"`
	// MaxOutputTokens is the HTTP providers' max_tokens. Default 4096.
	MaxOutputTokens int `toml:"max_output_tokens" json:"max_output_tokens"`

	// FakeDelayMS makes a TypeFake provider wait before answering.
	FakeDelayMS int `toml:"fake_delay_ms" json:"fake_delay_ms,omitempty"`
}

// ModelID identifies the model for cache keys: Model if set, otherwise the
// command argv joined by spaces, otherwise BaseURL.
func (c Config) ModelID() string {
	panic("provider: ModelID not implemented")
}

// EstimateTokens estimates the input tokens of a prompt of promptBytes bytes:
// ceil(promptBytes / bpt) + OverheadTokens. bpt <= 0 means c.BytesPerToken,
// and if that is <= 0 too, DefaultBytesPerToken(c).
func (c Config) EstimateTokens(promptBytes int, bpt float64) int {
	panic("provider: EstimateTokens not implemented")
}

// Cost is (in*InputPrice + out*OutputPrice) / 1e6 USD.
func (c Config) Cost(in, out int) float64 {
	panic("provider: Cost not implemented")
}

// DefaultBytesPerToken is 2.0 when the provider is a Claude model (Type is
// TypeAnthropic, or Model or any Argv element contains "claude", "haiku",
// "sonnet" or "opus", case-insensitive), else 3.3.
func DefaultBytesPerToken(c Config) float64 {
	panic("provider: DefaultBytesPerToken not implemented")
}

// ErrConfig is wrapped by New for an unusable Config.
var ErrConfig = errors.New("invalid provider config")

// New builds a provider from c. It registers c.APIKeyEnv with Redact. It
// does not read the API key yet: a missing key is reported by Complete, so a
// config naming an unset env var still loads.
func New(c Config) (Provider, error) {
	panic("provider: New not implemented")
}
