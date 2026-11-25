package provider

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// redactedMarker replaces every secret Redact finds.
const redactedMarker = "[REDACTED]"

// minSecretLen is the shortest env value Redact masks. Shorter values are too
// likely to be an ordinary word that happens to appear in error text.
const minSecretLen = 8

var (
	secretsMu sync.RWMutex
	// secretEnv is the set of env var names whose values are masked. It starts
	// with the providers' default key variables and grows via RegisterSecretEnv.
	secretEnv = map[string]struct{}{
		"ANTHROPIC_API_KEY":  {},
		"OPENAI_API_KEY":     {},
		"DEEPSEEK_API_KEY":   {},
		"OPENROUTER_API_KEY": {},
	}
)

// secretPatterns match credentials that appear in text without a registered
// env var holding them (pasted into a URL, echoed by a CLI, ...).
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)sk-[a-z0-9_\-]{12,}`),
	regexp.MustCompile(`(?i)bearer\s+[a-z0-9_\-\.]{12,}`),
	regexp.MustCompile(`(?i)x-api-key:\s*\S+`),
}

// RegisterSecretEnv adds an env var name whose value Redact must mask. Safe
// for concurrent use. Names registered by default: ANTHROPIC_API_KEY,
// OPENAI_API_KEY, DEEPSEEK_API_KEY, OPENROUTER_API_KEY.
func RegisterSecretEnv(name string) {
	if name == "" {
		return
	}
	secretsMu.Lock()
	defer secretsMu.Unlock()
	secretEnv[name] = struct{}{}
}

// secretNames returns the registered names in sorted order, so Redact's output
// does not depend on map iteration order.
func secretNames() []string {
	secretsMu.RLock()
	names := make([]string, 0, len(secretEnv))
	for name := range secretEnv {
		names = append(names, name)
	}
	secretsMu.RUnlock()
	sort.Strings(names)
	return names
}

// Redact masks secrets in s with "[REDACTED]": the current value of every
// registered secret env var (values shorter than 8 bytes are ignored), and
// anything matching (case-insensitive) `sk-[a-z0-9_\-]{12,}`,
// `bearer\s+[a-z0-9_\-\.]{12,}` or `x-api-key:\s*\S+`.
func Redact(s string) string {
	if s == "" {
		return s
	}
	// Env values are read at call time: a key exported after the provider was
	// built must still be masked.
	for _, name := range secretNames() {
		if v := os.Getenv(name); len(v) >= minSecretLen {
			s = strings.ReplaceAll(s, v, redactedMarker)
		}
	}
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, redactedMarker)
	}
	return s
}
