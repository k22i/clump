package provider

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const redactedMarker = "[REDACTED]"

const minSecretLen = 8

var (
	secretsMu sync.RWMutex
	secretEnv = map[string]struct{}{
		"ANTHROPIC_API_KEY":  {},
		"OPENAI_API_KEY":     {},
		"DEEPSEEK_API_KEY":   {},
		"OPENROUTER_API_KEY": {},
	}
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)sk-[a-z0-9_\-]{12,}`),
	regexp.MustCompile(`(?i)bearer\s+[a-z0-9_\-\.]{12,}`),
	regexp.MustCompile(`(?i)x-api-key:\s*\S+`),
}

func RegisterSecretEnv(name string) {
	if name == "" {
		return
	}
	secretsMu.Lock()
	defer secretsMu.Unlock()
	secretEnv[name] = struct{}{}
}

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

func Redact(s string) string {
	if s == "" {
		return s
	}
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
