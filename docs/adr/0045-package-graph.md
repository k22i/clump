# Package layout and the import graph

clump is one module with one binary; the packages are split by responsibility, and the import graph
is acyclic. The direction of every edge is load-bearing: `gitdiff`, `provider`, `ipc`, `lazygit` and
`rex` may only import the standard library (plus `golang.org/x/sys` for `ipc` and `rex`), which is
what keeps `clump push` free of config, provider and TUI setup (ADR-0005), and `internal/tui/*`
render packages stay pure so they can be golden-tested without a terminal (ADR-0007).

```
cmd/clump            CLI: flags, subcommands (watch, push, comments, submit, config init, rex install, lazygit install)
internal/gitdiff     unified-diff parser, canonical form + hash, git loaders
internal/comment    review-comment anchors: kinds, content keys, placement
internal/forge      post a review to GitHub: comment -> review payload, `gh api` submit
internal/catalog     catalog lines, hunk blocks, near-identical hunk collapsing
internal/group       prompt Build (tier 1/2), Validate/repair, Check, edit ops, History
internal/provider    Provider interface; command, OpenAI-compatible, Anthropic, fake; Registry; Redact
internal/cache       results, summaries, overrides, calibration on disk
internal/config      config.toml load/validate, embedded default.toml
internal/engine      the pipeline: cache -> Build -> estimate -> calls -> Validate -> cache
internal/ipc         push <-> watch protocol, socket path, listener, client, parent argv
internal/lazygit     classify lazygit's git argv into selections; lazygit install
internal/rex         OSC 7501 status, theme query, rex install
internal/tui         Bubble Tea UI (one-shot and watch): state, message flow, keymap, layouts
internal/tui/view    shared render types (Theme, Line, ListRow, HelpSection), file headers
internal/tui/{diffview,rawview,listview,helpview,sys}  pure render helpers, clipboard/editor glue
internal/report      Markdown (y, --plain) and JSON (--json) output of a grouped diff
internal/chromalexers  vendored chroma/v2/lexers with a lazily built registry (ADR-0011)
internal/update      latest release lookup, checksummed download, in-place binary replace (clump update)
rex/                 init.lua (Rex actions) + embed.go (package rexlua)
scripts/             mkfixture.sh (scratch test repo), mkgolden.sh (parser golden fixtures)
```

Import graph (arrows point at dependencies; no cycles):

```
cmd/clump -> tui, engine, config, cache, provider, gitdiff, ipc, lazygit, rex, rexlua, comment, forge, update
tui       -> engine, group, gitdiff, cache, config, lazygit, ipc, report, tui/*
tui/diffview -> chromalexers, chroma/v2 (core), view;  report -> gitdiff, group, cache
engine    -> group, catalog, gitdiff, provider, cache, config
group     -> catalog, gitdiff
catalog   -> gitdiff
comment   -> gitdiff, group
cache     -> group, provider, comment
config    -> provider
forge     -> comment, gitdiff
update    -> stdlib
gitdiff, provider, ipc, lazygit, rex -> stdlib (+ golang.org/x/sys for ipc, rex)
```
