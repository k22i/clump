package provider

// RegisterSecretEnv adds an env var name whose value Redact must mask. Safe
// for concurrent use. Names registered by default: ANTHROPIC_API_KEY,
// OPENAI_API_KEY, DEEPSEEK_API_KEY, OPENROUTER_API_KEY.
func RegisterSecretEnv(name string) {
	panic("provider: RegisterSecretEnv not implemented")
}

// Redact masks secrets in s with "[REDACTED]": the current value of every
// registered secret env var (values shorter than 8 bytes are ignored), and
// anything matching (case-insensitive) `sk-[a-z0-9_\-]{12,}`,
// `bearer\s+[a-z0-9_\-\.]{12,}` or `x-api-key:\s*\S+`.
func Redact(s string) string {
	panic("provider: Redact not implemented")
}
