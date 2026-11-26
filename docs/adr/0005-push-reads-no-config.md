# `clump push` reads no config

`push` is lazygit's `diffRenderers` stdin filter: it runs on every quit and on a startup probe, so
it does no config load, no provider setup, no cache lookups. Its only flags are `--then` and the
implicit no-op behaviour of unknown flags, and a diff renderer for `--then` comes from
`clump push --then '<cmd>'`, which only `clump lazygit install` writes (`then` in `config.toml`).

Consequence: `push` must stay a few milliseconds of work over `cat` (ADR-0011), which ruled out
loading config.toml on this path altogether.
