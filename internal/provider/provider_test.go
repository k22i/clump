package provider

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestModelID(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "model wins",
			cfg:  Config{Model: "m", Argv: []string{"a"}, BaseURL: "https://x/v1"},
			want: "m",
		},
		{
			name: "argv joined",
			cfg:  Config{Argv: []string{"a", "b", "c"}, BaseURL: "https://x/v1"},
			want: "a b c",
		},
		{
			name: "base url last",
			cfg:  Config{BaseURL: "https://x/v1"},
			want: "https://x/v1",
		},
		{
			name: "all empty",
			cfg:  Config{},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.ModelID(); got != tc.want {
				t.Fatalf("ModelID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		n    int
		bpt  float64
		want int
	}{
		{
			name: "explicit bpt with overhead",
			cfg:  Config{BytesPerToken: 4.0, OverheadTokens: 650},
			n:    1001,
			bpt:  2.0,
			want: 501 + 650,
		},
		{
			name: "falls back to the claude default",
			cfg:  Config{Model: "haiku"},
			n:    1001,
			bpt:  0,
			want: int(math.Ceil(1001 / 2.0)),
		},
		{
			name: "falls back to 3.3 for other models",
			cfg:  Config{Model: "deepseek-chat"},
			n:    1001,
			bpt:  0,
			want: int(math.Ceil(1001 / 3.3)),
		},
		{
			name: "configured bytes per token wins over the default",
			cfg:  Config{Model: "deepseek-chat", BytesPerToken: 2.5},
			n:    1001,
			bpt:  0,
			want: int(math.Ceil(1001 / 2.5)),
		},
		{
			name: "no bytes is only the overhead",
			cfg:  Config{OverheadTokens: 650},
			n:    0,
			bpt:  2.0,
			want: 650,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EstimateTokens(tc.n, tc.bpt); got != tc.want {
				t.Fatalf("EstimateTokens(%d, %v) = %d, want %d", tc.n, tc.bpt, got, tc.want)
			}
		})
	}
}

func TestCost(t *testing.T) {
	c := Config{InputPrice: 0.1, OutputPrice: 0.5}
	got := c.Cost(1_000_000, 2_000_000)
	if math.Abs(got-1.1) > 1e-9 {
		t.Fatalf("Cost = %v, want 1.1", got)
	}
	if got := (Config{}).Cost(1_000_000, 1_000_000); got != 0 {
		t.Fatalf("free provider Cost = %v, want 0", got)
	}
}

func TestDefaultBytesPerToken(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want float64
	}{
		{"anthropic type", Config{Type: TypeAnthropic}, 2.0},
		{"claude model", Config{Model: "claude-3-5-haiku"}, 2.0},
		{"model is case insensitive", Config{Model: "OPUS"}, 2.0},
		{"claude in argv", Config{Argv: []string{"claude", "-p"}}, 2.0},
		{"sonnet in argv", Config{Argv: []string{"/usr/bin/Sonnet-cli"}}, 2.0},
		{"other model", Config{Model: "deepseek-chat"}, 3.3},
		{"nothing set", Config{}, 3.3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DefaultBytesPerToken(tc.cfg); got != tc.want {
				t.Fatalf("DefaultBytesPerToken = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "empty name",
			cfg:  Config{Type: TypeFake},
			want: `invalid provider config: provider "": name is empty`,
		},
		{
			name: "command without argv",
			cfg:  Config{Name: "x", Type: TypeCommand},
			want: `invalid provider config: provider "x": argv is empty`,
		},
		{
			name: "command with empty argv0",
			cfg:  Config{Name: "x", Type: TypeCommand, Argv: []string{""}},
			want: `invalid provider config: provider "x": argv is empty`,
		},
		{
			name: "unknown format",
			cfg:  Config{Name: "x", Type: TypeCommand, Argv: []string{"cat"}, Format: "yaml"},
			want: `invalid provider config: provider "x": unknown format "yaml"`,
		},
		{
			name: "unknown type",
			cfg:  Config{Name: "x", Type: "bogus"},
			want: `invalid provider config: provider "x": unknown type "bogus" (want command, openai, anthropic or fake)`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.cfg)
			if err == nil {
				t.Fatalf("New(%+v) = %v, want an error", tc.cfg, p)
			}
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("error %v is not an ErrConfig", err)
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
		})
	}
}

func TestNewDispatch(t *testing.T) {
	t.Run("fake", func(t *testing.T) {
		p, err := New(Config{Name: "scripted", Type: TypeFake, FakeDelayMS: 25})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		f, ok := p.(*Fake)
		if !ok {
			t.Fatalf("New returned %T, want *Fake", p)
		}
		if got := p.Name(); got != "scripted" {
			t.Fatalf("Name() = %q, want \"scripted\"", got)
		}
		if f.Delay != 25_000_000 {
			t.Fatalf("Delay = %v, want 25ms", f.Delay)
		}
	})

	t.Run("command", func(t *testing.T) {
		p, err := New(Config{Name: "cat", Type: TypeCommand, Argv: []string{"cat"}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, ok := p.(*commandProvider); !ok {
			t.Fatalf("New returned %T, want *commandProvider", p)
		}
		if got := p.Name(); got != "cat" {
			t.Fatalf("Name() = %q, want \"cat\"", got)
		}
	})

	t.Run("empty format is raw", func(t *testing.T) {
		p, err := New(Config{Name: "cat", Type: TypeCommand, Argv: []string{"cat"}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		text, _, err := p.Complete(context.Background(), "hi")
		if err != nil || text != "hi" {
			t.Fatalf("Complete = %q, %v", text, err)
		}
	})

	t.Run("http types are not built yet", func(t *testing.T) {
		for _, typ := range []string{TypeOpenAI, TypeAnthropic} {
			_, err := New(Config{Name: "h", Type: typ, Model: "m"})
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("New(%s) error = %v, want an ErrConfig", typ, err)
			}
			if err == nil || !strings.Contains(err.Error(), "not built yet") {
				t.Fatalf("New(%s) error = %v, want the not-built-yet message", typ, err)
			}
		}
	})
}
