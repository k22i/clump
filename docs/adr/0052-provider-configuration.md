# Provider configuration, defaults and prices

A provider is one `[providers.<name>]` table: a command (prompt on stdin), an OpenAI-compatible
endpoint, the Anthropic Messages API, or the fake. The defaults below are what `clump config init`
prints and what `Default()` returns, and they must stay equal (a test asserts it).

Prices, `bytes_per_token` and `overhead_tokens` are here because they drive the estimate the user
sees before spending money (ADR-0025) and the calibration that keeps it honest (ADR-0031).

`Provider.Complete(ctx, prompt) (text, Usage, error)`. Implementations:

- **command**: argv with the prompt on stdin, cwd `os.TempDir()` (ADR-0027), stdout read as
  `raw`, `claude-json` (`result`, `is_error`, `total_cost_usd`,
  usage incl. cache tokens) or `opencode-json` (JSONL `text` / `step_finish` / `error` events).
- **openai**: POST `base_url/chat/completions`, `temperature: 0`,
  `response_format: {"type":"json_object"}`, Bearer key from `api_key_env` (optional, for
  Ollama).
- **anthropic**: POST `base_url/v1/messages` (`x-api-key`, `anthropic-version: 2023-06-01`),
  key from `api_key_env` (default `ANTHROPIC_API_KEY`). No `cache_control` (ADR-0026).
- **fake**: deterministic answers (`provider.AutoAnswer`: one group per top-level directory;
  summaries per file), optional delay. Used by tests and the e2e run.

Defaults (`internal/config/default.toml`): `cheap = "claude-haiku"`,
`strong = "claude-sonnet"`, `default = "cheap"`, both command providers running
`claude -p --model haiku|sonnet --output-format json --tools "" --setting-sources ""
--strict-mcp-config --exclude-dynamic-system-prompt-sections --no-session-persistence
--system-prompt "You are a JSON API. ..."` with `format = "claude-json"`. Prices per MTok: Haiku
5.5 $0.10/$0.50, Sonnet 5.5 $2/$10. `bytes_per_token` 2.0 for Claude, 3.3 otherwise;
`overhead_tokens` 650 for the claude CLI (measured 643).

Keys are never in config; errors pass through `provider.Redact` (registered env values,
`sk-...`, `Bearer ...`, `x-api-key: ...`) — ADR-0028.
