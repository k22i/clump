<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/clump-horizontal-dark.svg">
    <img alt="clump" src="docs/brand/clump-horizontal.svg" width="280">
  </picture>
</p>

`clump` reads a git diff and asks an LLM to split its hunks into logical changes, each with a title
and an explanation, then shows that grouping in a terminal UI next to the raw diff so you can review
it, edit it and copy it out. Instead of reading a commit file by file and guessing what the author
was doing, you read a short list of groups — "retry policy in the client", "tests for the timeout",
"the changelog entry" — and check each against its hunks.

```
clump · range 7cad34e2a144^..2a438fc11199 · → claude-haiku · auto on
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
 ▸ Append log lines to docs/log.txt   +3 −0 1h │ 1. Append log lines to docs/log.txt
   Add func f6 to src/fill.go         +1 −0 1h │ 1 file · 1 hunk · +3 −0
                                               │
                                               │   Adds three new lines (line 6 through line 8) to the end of
                                               │   the log document. Documentation-only change with no code
                                               │   impact.
                                               │
                                               │ ── docs/log.txt (M +3 −0) ──────────────────────────────────
                                               │   @@ -3,3 +3,6 @@ line 2
                                               │    3  3  line 3
                                               │    4  4  line 4
                                               │    5  5  line 5
                                               │       6 +line 6
                                               │       7 +line 7
                                               │       8 +line 8
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
? keys · / search · c reviewed · x mark · M move · z fold · a all · s split · u undo · q quit
```

**Highlights**

- **Any diff**: a commit, a range, a GitHub PR (`--pr`), a patch on stdin — or a watch pane beside
  [lazygit](https://github.com/jesseduffield/lazygit) that follows lazygit's selection.
- **Cheap and cached**: about $0.001–$0.013 per diff on Haiku, 4–10 s; results are cached, so
  revisiting a commit never calls the model. Thousands of tokens are skipped by not sending lock
  files, migrations and codegen at all.
- **Yours to edit**: move, rename, merge, split groups; your edits are saved per diff and survive a
  restart. Mark hunks reviewed and they stay checked across a whole branch.
- **Comments and PR reviews**: attach comments to lines, files or groups (`C`), then post them to
  GitHub as one review (`clump submit`, or `P` in the TUI).
- **Sane defaults, private by design**: works out of the box with the `claude` CLI; every provider is
  a config table away; diffs are untrusted data in the prompt; nothing above the token ceiling is
  sent without asking.

## Install

A prebuilt binary — macOS and Linux, arm64 and amd64 — no Go toolchain needed:

```
curl -fsSL https://raw.githubusercontent.com/k22i/clump/main/scripts/install.sh | sh
```

From then on `clump upgrade` fetches the next release over the binary itself. With Go 1.27 or newer:
`go install github.com/k22i/clump/cmd/clump@latest`. The default providers run the `claude` command,
so by default you need the Claude Code CLI installed and logged in; any OpenAI-compatible or
Anthropic API works too ([configuration](docs/configuration.md#providers)).

## Quick start

```
clump                     # HEAD
clump <rev>               # one commit
clump main...feature      # merge base to feature, like a PR
clump --branch            # this branch against its default base
clump --pr 123            # a GitHub PR of this repo (via the gh CLI)
git diff | clump          # a patch on stdin
clump --pick              # choose a commit or comparison first
```

With a terminal on stdout `clump` opens the TUI; with stdout redirected (`clump > notes.md`) it prints
the Markdown summary instead. `--provider NAME` picks a provider or alias, `--repo DIR` another
repository, `--json` / `--plain` print instead of opening the TUI, `--dry-run` prints exactly what
would be sent (and sends nothing), `--no-cache` bypasses reads, `--yes` skips the over-ceiling
prompt. An empty diff is an error, not an empty
screen. The full CLI — ranges, `--pr` spec formats, flags, subcommands — is in [docs/cli.md](docs/cli.md).

In watch mode the pane opens beside lazygit and groups whatever diff lazygit shows — a 400 ms
debounce holds the model calls down to one per stop:

```
clump lazygit install --write   # add clump push to lazygit's diffRenderers
clump watch                     # the pane; auto-groups with the cheap provider
```

Anything is editable in the TUI: press `?` for the key map, or read [docs/keys.md](docs/keys.md).
Groups are the model's first draft — `M` moves hunks between groups, `R` renames, `u` undoes.

## Privacy

Whichever provider you configure receives the diff, the commit messages used as context and the file
paths in it. In watch mode with auto-group on (the default) every followed selection goes to the
`cheap` provider; `A` turns that off, and nothing above the ceiling is sent without asking. The diff
is wrapped in per-request markers and the instructions say it is never an instruction — the model
treats your repository's content as data. Keys live in environment variables, never in the config;
every error clump prints goes through a secret redactor.

## Documentation

| doc | what is in it |
|---|---|
| [docs/cli.md](docs/cli.md) | every command and flag: ranges, `--pr SPEC` formats, `--pick`, `--dry-run`, subcommands |
| [docs/configuration.md](docs/configuration.md) | the config file, every key, providers and aliases, API keys, costs, limits and the large-diff tiers |
| [docs/reviews.md](docs/reviews.md) | caching, your edits, reviewed marks, `clump comments` and posting a PR review |
| [docs/keys.md](docs/keys.md) | every key in the TUI |
| [docs/lazygit-watch.md](docs/lazygit-watch.md) | watch mode: how `clump push` works, setup, what lazygit selections are followed |
| [docs/rex.md](docs/rex.md) | the Rex module: actions, bindings, program status |
| [docs/troubleshooting.md](docs/troubleshooting.md) | `clump doctor` and the common failures |
| [docs/e2e/README.md](docs/e2e/README.md) | the end-to-end run and its captures |
| [docs/adr/README.md](docs/adr/README.md) | a decision record per contract; there is no separate design document |

If something does not work, start with `clump doctor` — it prints what clump sees and sends nothing
to any provider. [docs/troubleshooting.md](docs/troubleshooting.md) covers the common failures,
including the watch-pane `PATH` trap.

## Status

Verified end to end in Rex on 2026-11-25 (Rex server 0.1.0-preview.21, lazygit 0.66.3, macOS):
the split beside lazygit, seven commits followed and grouped, cache hits, a 31-press hold collapsing
to one call, edits surviving a pane restart — 14 real calls on Haiku for about $0.006. Steps and
captures in [docs/e2e/README.md](docs/e2e/README.md). Linux builds but only macOS was run;
`rex.ui.pick` falls back to clump's own picker on the tested build.

## Development

```
go build ./... && go vet ./...
go test -race ./...   # property tests, golden fixtures, provider fakes; no network, no model
```

Design notes and the reasoning behind every contract (cache keys, prompt formats, the key map, the
lazygit selection rules) live in [docs/adr/](docs/adr/README.md), one decision per file. `spike/`
holds the measurements this build rests on; `go run ./scripts/lint-comments` enforces the comment
ban in clump's own packages.

## Brand

The logo, wordmark, favicons and colour guidelines live in [docs/brand/](docs/brand/README.md).

## License

License: not chosen yet. The vendored syntax lexers under `internal/chromalexers` are MIT-licensed
Chroma code, Copyright (C) 2017 Alec Thomas; the text is at `internal/chromalexers/LICENSE`.
