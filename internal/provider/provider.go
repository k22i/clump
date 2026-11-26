package provider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Usage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CacheTokens  int     `json:"cache_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd"`
	CostSource   string  `json:"cost_source"`
}

const (
	CostReported  = "reported"
	CostEstimated = "estimated"
)

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

type Provider interface {
	Name() string
	Complete(ctx context.Context, prompt string) (text string, u Usage, err error)
}

const (
	TypeCommand   = "command"
	TypeOpenAI    = "openai"
	TypeAnthropic = "anthropic"
	TypeFake      = "fake"
)

const (
	FormatRaw          = "raw"
	FormatClaudeJSON   = "claude-json"
	FormatOpencodeJSON = "opencode-json"
)

type Config struct {
	Name string `toml:"-" json:"name"`
	Type string `toml:"type" json:"type"`

	Argv   []string `toml:"argv" json:"argv,omitempty"`
	Format string   `toml:"format" json:"format,omitempty"`
	Dir    string   `toml:"dir" json:"dir,omitempty"`

	Model     string `toml:"model" json:"model,omitempty"`
	BaseURL   string `toml:"base_url" json:"base_url,omitempty"`
	APIKeyEnv string `toml:"api_key_env" json:"api_key_env,omitempty"`

	InputPrice      float64 `toml:"input_price" json:"input_price"`
	OutputPrice     float64 `toml:"output_price" json:"output_price"`
	BytesPerToken   float64 `toml:"bytes_per_token" json:"bytes_per_token"`
	OverheadTokens  int     `toml:"overhead_tokens" json:"overhead_tokens"`
	TimeoutSec      int     `toml:"timeout" json:"timeout"`
	MaxOutputTokens int     `toml:"max_output_tokens" json:"max_output_tokens"`

	FakeDelayMS int `toml:"fake_delay_ms" json:"fake_delay_ms,omitempty"`
}

func (c Config) ModelID() string {
	switch {
	case c.Model != "":
		return c.Model
	case len(c.Argv) > 0:
		return strings.Join(c.Argv, " ")
	default:
		return c.BaseURL
	}
}

func (c Config) EstimateTokens(promptBytes int, bpt float64) int {
	return c.ceilTokens(promptBytes, bpt) + c.OverheadTokens
}

func (c Config) bptOrDefault(bpt float64) float64 {
	if bpt <= 0 {
		bpt = c.BytesPerToken
	}
	if bpt <= 0 {
		bpt = DefaultBytesPerToken(c)
	}
	return bpt
}

func (c Config) ceilTokens(n int, bpt float64) int {
	if n <= 0 {
		return 0
	}
	return int(math.Ceil(float64(n) / c.bptOrDefault(bpt)))
}

func (c Config) Cost(in, out int) float64 {
	return (float64(in)*c.InputPrice + float64(out)*c.OutputPrice) / 1e6
}

var claudeModelHints = []string{"claude", "haiku", "sonnet", "opus"}

func DefaultBytesPerToken(c Config) float64 {
	if c.Type == TypeAnthropic {
		return 2.0
	}
	for _, s := range append([]string{c.Model}, c.Argv...) {
		s = strings.ToLower(s)
		for _, hint := range claudeModelHints {
			if strings.Contains(s, hint) {
				return 2.0
			}
		}
	}
	return 3.3
}

var ErrConfig = errors.New("invalid provider config")

func New(c Config) (Provider, error) {
	if c.Name == "" {
		return nil, fmt.Errorf("%w: provider %q: name is empty", ErrConfig, c.Name)
	}
	switch c.Type {
	case TypeCommand:
		if len(c.Argv) == 0 || c.Argv[0] == "" {
			return nil, fmt.Errorf("%w: provider %q: argv is empty", ErrConfig, c.Name)
		}
		switch c.Format {
		case "", FormatRaw, FormatClaudeJSON, FormatOpencodeJSON:
		default:
			return nil, fmt.Errorf("%w: provider %q: unknown format %q", ErrConfig, c.Name, c.Format)
		}
	case TypeOpenAI, TypeAnthropic, TypeFake:
	default:
		return nil, fmt.Errorf("%w: provider %q: unknown type %q (want %s, %s, %s or %s)",
			ErrConfig, c.Name, c.Type, TypeCommand, TypeOpenAI, TypeAnthropic, TypeFake)
	}
	if c.APIKeyEnv != "" {
		RegisterSecretEnv(c.APIKeyEnv)
	}
	switch c.Type {
	case TypeCommand:
		return newCommand(c), nil
	case TypeFake:
		return &Fake{ProviderName: c.Name, Delay: time.Duration(c.FakeDelayMS) * time.Millisecond}, nil
	case TypeOpenAI:
		return newOpenAI(c)
	case TypeAnthropic:
		return newAnthropic(c)
	}
	return nil, fmt.Errorf("%w: provider %q: unknown type %q", ErrConfig, c.Name, c.Type)
}
