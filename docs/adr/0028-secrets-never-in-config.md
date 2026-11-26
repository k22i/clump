# Secrets never live in config, and errors are redacted

`config.toml` names the environment variable holding a key (`api_key_env`) and never the key
itself. A key named like a secret (`api_key`, `token`, …) is rejected at load time with a pointer at
`api_key_env`.

Keys are read at call time, not at provider construction, so a config naming an unset variable still
loads and a key exported later still works.

Every error passes through `provider.Redact`, which masks the current value of each registered
secret env var (values shorter than 8 bytes are ignored, since a short value is more likely to be an
ordinary word in an error message) plus anything matching `sk-…`, `Bearer …` or `x-api-key: …`. A
key is never put in a URL, a log or an error.
