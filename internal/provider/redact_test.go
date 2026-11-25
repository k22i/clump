package provider

import (
	"strings"
	"sync"
	"testing"
)

func TestRedact(t *testing.T) {
	t.Run("registered env value", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "supersecretvalue123")
		got := Redact("key=supersecretvalue123 done")
		if got != "key=[REDACTED] done" {
			t.Fatalf("Redact = %q, want the value masked", got)
		}
	})

	t.Run("short env value is ignored", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "short")
		if got := Redact("a short value"); got != "a short value" {
			t.Fatalf("Redact = %q, want it unchanged", got)
		}
	})

	t.Run("value read at call time", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "")
		if got := Redact("later-visible-secret"); got != "later-visible-secret" {
			t.Fatalf("Redact = %q, want it unchanged while the var is unset", got)
		}
		t.Setenv("OPENROUTER_API_KEY", "later-visible-secret")
		if got := Redact("x later-visible-secret y"); got != "x [REDACTED] y" {
			t.Fatalf("Redact = %q, want the newly set value masked", got)
		}
	})

	t.Run("sk key", func(t *testing.T) {
		got := Redact("using sk-abcdefghijklmnop now")
		if got != "using [REDACTED] now" {
			t.Fatalf("Redact = %q", got)
		}
	})

	t.Run("bearer token", func(t *testing.T) {
		got := Redact("Authorization: Bearer abcdefghijklmnop")
		if strings.Contains(got, "abcdefghijklmnop") || !strings.Contains(got, redactedMarker) {
			t.Fatalf("Redact = %q, want the token masked", got)
		}
	})

	t.Run("x-api-key header", func(t *testing.T) {
		got := Redact("x-api-key: secret123 rest")
		if got != "[REDACTED] rest" {
			t.Fatalf("Redact = %q", got)
		}
	})

	t.Run("nothing to mask", func(t *testing.T) {
		const clean = "context canceled: exit status 3"
		if got := Redact(clean); got != clean {
			t.Fatalf("Redact = %q, want it unchanged", got)
		}
		if got := Redact(""); got != "" {
			t.Fatalf("Redact(\"\") = %q", got)
		}
	})
}

func TestRegisterSecretEnv(t *testing.T) {
	t.Run("unregistered name is not masked", func(t *testing.T) {
		const value = "never-registered-value"
		t.Setenv("CLUMP_UNREGISTERED_KEY", value)
		if got := Redact(value); got != value {
			t.Fatalf("Redact = %q, want it unchanged", got)
		}
	})

	t.Run("registered name is masked", func(t *testing.T) {
		const value = "my-own-secret-value"
		t.Setenv("MY_KEY", value)
		RegisterSecretEnv("MY_KEY")
		if got := Redact("token=" + value); got != "token=[REDACTED]" {
			t.Fatalf("Redact after RegisterSecretEnv = %q, want the value masked", got)
		}
	})

	t.Run("empty name is ignored", func(t *testing.T) {
		const value = "empty-name-value"
		RegisterSecretEnv("")
		if got := Redact(value); got != value {
			t.Fatalf("Redact = %q, want it unchanged", got)
		}
	})
}

func TestRegisterSecretEnvConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			RegisterSecretEnv("CONCURRENT_TEST_KEY")
			_ = Redact("sk-abcdefghijklmnop")
		}()
	}
	wg.Wait()
	if got := Redact("sk-abcdefghijklmnop"); got != redactedMarker {
		t.Fatalf("Redact = %q", got)
	}
}

func TestNewRegistersAPIKeyEnv(t *testing.T) {
	const value = "registered-via-config"
	t.Setenv("CLUMP_TEST_KEY_ENV", value)

	if _, err := New(Config{Name: "f", Type: TypeFake, APIKeyEnv: "CLUMP_TEST_KEY_ENV"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := Redact(value); got != redactedMarker {
		t.Fatalf("Redact = %q, want the config key env masked", got)
	}
}
