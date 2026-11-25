# clump

`clump` reads a git diff and asks an LLM to split its hunks into logical changes, each with a title
and an explanation, then shows that grouping in a terminal UI next to the raw diff so you can review
it, edit it and copy it out. Instead of reading a commit file by file and guessing what the author
was doing, you read a short list of groups — "retry policy in the client", "tests for the timeout",
"the changelog entry" — and check each against its hunks. It runs one-shot on a commit, a range or a
patch on stdin, and as a watch pane beside lazygit that follows lazygit's selection.

```
clump · range 7cad34e2a144^..2a438fc11199 · → claude-haiku · auto on
▸ Append log lines to docs/log.txt      +3 −0 1h│1. Append log lines to docs/log.txt
  Add func f6 to src/fill.go            +1 −0 1h│1 file · 1 hunks · +3 −0
                                                │Adds three new lines (line 6 through line 8)
                                                │to the end of the log document.
                                                │Documentation-only change with no code impact.
                                                │
                                                │── docs/log.txt (M +3 −0) ──────────────────────
                                                │  @@ -3,3 +3,6 @@ line 2
                                                │   line 5
                                                │  +line 6
                                                │  +line 7
                                                │  +line 8
? keys · a all · / search · x mark · M move · u undo · q quit
```

(A real watch pane from the e2e run.) The list is on the left — title, `+added −deleted`, hunk count —
and the selected group's explanation and hunks, grouped by file, on the right: side by side at 100
columns or wider, stacked below that.

## Install

```
go install github.com/k22i/clump/cmd/clump@latest
```

Go 1.27 or newer. Or build from a clone:

```
git clone https://github.com/k22i/clump && cd clump
CGO_ENABLED=0 go build -o clump ./cmd/clump
```

One static binary, no cgo, no runtime dependencies of its own — but the default providers run the
`claude` command, so by default you need the Claude Code CLI installed and logged in. To call an
API directly or run a local model, configure another provider (below).

## Quick start

```
clump                     # HEAD
clump <rev>               # one commit
clump main...feature      # merge base to feature, like a PR
clump A..B                # two-dot range
clump --branch            # this branch against its default base
git diff | clump          # a patch on stdin
clump --pick              # choose a commit or comparison first
```

With a terminal on stdout `clump` opens the TUI; with stdout redirected (`clump > notes.md`) it prints
the Markdown summary instead. `A...B` and `--branch` show a range (the
commits' messages are the model's context), `A..B` a two-dot range, a patch on stdin is read as
given, and `--pick` shows clump's picker first: "This branch vs `<base>`" when you are not on the
base branch, then the 50 most recent commits; type to filter, Enter to open. The TUI opens
immediately: while the model works the list shows the diff's files and you can read their hunks;
the groups replace them when the answer arrives.

Flags:

| flag | effect |
|---|---|
| `--json` | print the groups as JSON instead of opening the TUI |
| `--plain` | print a Markdown summary (the text `y` puts on the clipboard) |
| `--dry-run` | print exactly what would be sent to the provider, and the estimate; send nothing |
| `--no-cache` | ignore cached results, still write them |
| `--yes` | do not ask before sending a prompt estimated above `ceiling` |
| `--provider NAME` | use a provider name or alias from the config (default: the `default` alias) |
| `--repo DIR` | work on DIR instead of the current directory |
| `--branch` | compare the current branch with its default base (`origin/HEAD`, else `main`, else `master`) |

`--json`, `--plain` and `--dry-run` print and exit instead of opening the TUI; they are what you use
in a script. An empty diff is an error, not an empty screen.

## Configuration

`clump` reads `~/.config/clump/config.toml`. `CLUMP_CONFIG` overrides the whole path and
`XDG_CONFIG_HOME` the directory it is looked for in; the file is optional.

`clump config init` prints the commented default (embedded in the binary) and `clump config path` the
path it would read. A file overlays the defaults: a top-level key it sets replaces the default value,
and a `[providers.<name>]` table it defines replaces the built-in provider of that name, or adds one.

### Top-level keys

| key | default | meaning |
|---|---|---|
| `default` | `"cheap"` | provider or alias one-shot mode uses when `--provider` is not given |
| `cheap` | `"claude-haiku"` | alias: the cheap provider. Watch mode auto-groups with it |
| `strong` | `"claude-sonnet"` | alias: the stronger provider, for big branches |
| `ceiling` | `40000` | estimated tokens above which one-shot asks first and watch never auto-sends |
| `budget` | `60000` | token budget for one prompt; larger diffs are truncated or summarised |
| `max_groups` | `12` | most groups the model may return (1–50) |
| `auto_group` | `true` | watch mode groups every followed selection without asking |
| `follow_worktree` | `false` | watch mode also follows lazygit's working-tree/staged diffs |
| `then` | `""` | diff renderer `clump lazygit install` keeps behind `clump push`, e.g. `"delta --paging=never"` |

### Generated files

Lock files, codegen output and vendored code are not worth reading line by line. Their changes
follow from a change somewhere else, and that change is what you review. clump does not send their
hunks to the model, so you pay for fewer tokens and the groups have less noise. The model is told
only their paths ("generated files also changed: …"), so it can still title the schema change that
a migration came from. In the TUI they sit in a **Generated files** group at the end of the list.

A file is generated when, in this order:

1. it matches `review`: never generated, whatever else matches;
2. `.gitattributes` sets `linguist-generated` or `linguist-vendored` on it, as GitHub uses to collapse
   files in pull requests; `-linguist-generated` makes it reviewed;
3. it matches one of your `patterns`;
4. it matches a built-in pattern (unless `builtin = false`):
   - **lock files**: `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lock`, `Cargo.lock`,
     `go.sum`, `poetry.lock`, `uv.lock`, `Gemfile.lock`, `composer.lock`, `flake.lock`, `Podfile.lock`,
     `Package.resolved`, `.terraform.lock.hcl`, …
   - **ORM migrations and schema dumps**: drizzle (`drizzle/**/*.sql`, `meta/_journal.json`,
     `meta/*_snapshot.json`, and drizzle-kit v1's `migrations/<ts>_<name>/snapshot.json`), prisma (`prisma/migrations/**/migration.sql`), Rails `db/schema.rb`
     and `db/structure.sql`, Supabase `database.types.ts`;
   - **codegen**: `__generated__/`, `*.generated.*`, `*.gen.*` (`routeTree.gen.ts`), `*_gen.go`,
     `zz_generated*`, protobuf/gRPC (`*.pb.go`, `*_pb2.py`, `*_pb.ts`, …), GraphQL codegen's client
     preset (`gql/graphql.ts`, `gql/gql.ts`, `gql/fragment-masking.ts`) and gql.tada's
     `graphql-env.d.ts`, Dart build_runner (`*.g.dart`, `*.freezed.dart`), .NET designers;
   - **bundles and stubs**: `*.min.js`, `*.min.css`, source maps, `next-env.d.ts`, `.next/`,
     `.svelte-kit/`;
   - **vendored code**: `vendor/`, `node_modules/`, `Pods/`, Yarn's `.pnp.cjs` and `.yarn/releases/`;
5. a comment in its first 10 lines says it is generated: `Code generated … DO NOT EDIT.` (Go's
   convention, used by sqlc, gqlgen, ent, protoc-gen-go and stringer), `@generated` (Relay,
   Cargo), `<auto-generated>` (.NET), "auto-generated" or "do not edit". A `.sql` file containing
   drizzle-kit's `--> statement-breakpoint` also counts, which catches drizzle migrations in any
   folder, and so does a `.json` file that opens like a drizzle-kit snapshot (`"dialect"` and
   `"prevId"`/`"prevIds"` in its first 10 lines). A single-statement drizzle migration has no
   breakpoint and is reviewed, unless you add `**/migrations/*/migration.sql` to `patterns`.

Left out on purpose: Django migrations, Jest snapshots, `CHANGELOG.md` and `go.mod`. A tool writes
them too, but their diffs carry intent worth reading. Add them to `patterns` if you disagree.

```toml
[generated]
detect = true                         # false sends and reviews everything
builtin = true                        # false: only patterns, .gitattributes and markers
patterns = ["src/api/client/", "*.snap"]
review = ["db/structure.sql"]
```

Patterns are gitignore-style. A pattern with no `/` matches a name at any depth (`go.sum`, `*.pb.go`).
A `/` anchors it at the repo root (`db/schema.rb`); start it with `**/` to match at any depth. `*`
stays within one path segment and `**` spans directories. A pattern that matches a directory, such
as `vendor/` or `**/__generated__`, covers everything below it. If every file in a diff is generated,
nothing is sent. `--dry-run` lists the files it would leave out, each with its reason.

### Providers

Each provider is a `[providers.<name>]` table. `type` is one of:

| type | what it talks to |
|---|---|
| `command` | an argv with the prompt on stdin; stdout read as `format` |
| `openai` | any OpenAI-compatible `POST <base_url>/chat/completions` |
| `anthropic` | the Anthropic Messages API, `POST <base_url>/v1/messages` |
| `fake` | no model: deterministic groups by top-level directory, for trying clump out and for tests |

For `type = "command"`, `format` is `raw` (stdout is the answer), `claude-json` (the `claude -p
--output-format json` envelope: `result`, `is_error`, `total_cost_usd`, usage) or `opencode-json`
(the JSONL events of `opencode run --format json`). A command runs in the temp directory by default,
so an agent CLI never picks up the repository's `CLAUDE.md`.

Every provider key:

| key | default | meaning |
|---|---|---|
| `type` | — | `command`, `openai`, `anthropic` or `fake`; required |
| `argv` | — | the command, for `type = "command"`; required there |
| `format` | `"raw"` | how a command's stdout is read: `raw`, `claude-json`, `opencode-json` |
| `dir` | the temp directory | the command's working directory |
| `model` | — | model name; required for `openai` and `anthropic`, and part of the cache key |
| `base_url` | `https://api.anthropic.com` for `anthropic` | the API base; the OpenAI provider POSTs to `<base_url>/chat/completions` |
| `api_key_env` | `ANTHROPIC_API_KEY` for `anthropic` | name of the environment variable holding the key; empty means no `Authorization` header |
| `input_price` | `0` | USD per million input tokens, for estimates |
| `output_price` | `0` | USD per million output tokens, for estimates |
| `bytes_per_token` | `2.0` for Claude, `3.3` otherwise | prompt-size heuristic used before sending |
| `overhead_tokens` | `0` | fixed per-call input the tool adds; the `claude` CLI adds about 650 |
| `timeout` | `180` | seconds one call may take |
| `max_output_tokens` | `4096`, or `16000` for `anthropic` | `max_tokens` for the HTTP providers |
| `fake_delay_ms` | `0` | for `fake`: wait this long before answering |

### The Claude CLI (the default)

```
[providers.claude-haiku]
type = "command"
argv = ["claude", "-p", "--model", "haiku", "--output-format", "json", "--tools", "",
        "--setting-sources", "", "--strict-mcp-config",
        "--exclude-dynamic-system-prompt-sections", "--no-session-persistence",
        "--system-prompt", "You are a JSON API. Follow the user message exactly and output only JSON."]
format = "claude-json"
model = "haiku"
input_price = 0.10
output_price = 0.50
bytes_per_token = 2.0
overhead_tokens = 650
timeout = 180
```

`claude-sonnet` is the same table with `--model sonnet`, `model = "sonnet"`, `input_price = 2.0`,
`output_price = 10.0` and `timeout = 300`. Without the
`--tools`/`--setting-sources`/`--strict-mcp-config` trio every call carried about 18.4k tokens of
Claude Code harness; with them, about 643. `--bare` drops the OAuth login, so it cannot be used.

### The Anthropic API directly

About half the cost of `claude -p` for the same model: the CLI writes the prompt to the 1-hour
prompt cache on every call, billed at twice the input price, so its reported cost is roughly
`tokens × list price × 2`. A direct Messages call without `cache_control` does not pay that —
roughly $0.0035 (Haiku) and $0.069 (Sonnet) for a 29k-token branch in `spike/B-grouping.md`.

```
[providers.anthropic-haiku]
type = "anthropic"
model = "claude-haiku-5-5"
api_key_env = "ANTHROPIC_API_KEY"
input_price = 0.10
output_price = 0.50
bytes_per_token = 2.0
```

Then point an alias at it: `cheap = "anthropic-haiku"`.

### Other providers

```
[providers.deepseek]
type = "openai"
base_url = "https://api.deepseek.com/v1"
model = "deepseek-chat"
api_key_env = "DEEPSEEK_API_KEY"
input_price = 0.27
output_price = 1.10
bytes_per_token = 3.3

[providers.openrouter]
type = "openai"
base_url = "https://openrouter.ai/api/v1"
model = "deepseek/deepseek-chat"
api_key_env = "OPENROUTER_API_KEY"

# Local Ollama: no key, so no api_key_env.
[providers.ollama]
type = "openai"
base_url = "http://localhost:11434/v1"
model = "qwen3:8b"

# The opencode CLI as a command provider, reading its JSONL events.
[providers.opencode]
type = "command"
argv = ["opencode", "run", "-m", "opencode/deepseek-v4.1-flash-free", "--format", "json"]
format = "opencode-json"
model = "deepseek-v4.1-flash-free"
```

In the spike the free DeepSeek route through opencode was slow (20–108 s per diff) and needed
repairs, so treat it as a fallback rather than a default.

### Aliases and switching

`cheap`, `strong` and `default` are the three aliases: `default` picks the provider one-shot mode
uses, `cheap` is what watch mode auto-groups with, `strong` is there for big branches. All three
name a provider table, and `--provider NAME` accepts either an alias or a table name.

Press `m` in the TUI to switch provider at runtime: the menu lists every configured provider with
its aliases, and choosing one shows that provider's cached result if there is one (`provider is now
<name> (cached)`), otherwise it groups with it. With edits in place the switch is recorded, and `r`
regroups with the new provider.

### API keys are never in the config

`api_key_env` names an environment variable; the key itself stays in your environment. Keys named
`api_key`, `key`, `token` or `secret` in the config are rejected when it loads, with a pointer at
`api_key_env`. Every error clump prints goes through a redactor that strips the values of the
registered key variables and anything shaped like `sk-...`, `Bearer ...` or `x-api-key: ...`, so a
failing request cannot put a key in your terminal or a log.

## Privacy

Whichever provider you configure receives the diff, the commit messages used as context and the
file paths in it: the model cannot group hunks it has not seen. In watch mode with auto-group on
(the default), every selection you land on goes to the `cheap` provider, and the header shows which
provider answered (`→ claude-haiku`). `A` turns auto-group off; then nothing is sent until you press
`g`.

Nothing is sent above the ceiling without asking: one-shot mode prompts on the terminal, watch mode
shows `press g to group (~N tokens, ~$X)` and waits. `--dry-run` prints exactly the prompt or
prompts that would be sent, plus the estimate, and sends nothing.

The diff is untrusted data in the prompt: everything from the repository sits between
`<<<CLUMP-DATA-{nonce}>>>` and `<<<END-CLUMP-DATA-{nonce}>>>` with a fresh random nonce per request,
and the instructions say that text is never an instruction. In the spike neither Haiku nor DeepSeek
followed an injected "ignore previous instructions" inside a diff; both flagged it.

## Costs and limits

Before sending, clump estimates the call — calls, input tokens, cost — from `bytes_per_token`,
`overhead_tokens` and the provider's prices. That is what `--dry-run` prints, and what appears in
`press g to group (~12,400 tokens, ~$0.0033)`.

Three keys bound it: `ceiling` (ask above this), `budget` (one prompt must fit) and `max_groups`.
`bytes_per_token` is not a constant: after each real call clump compares the prompt size with the
input tokens the provider reported and stores a per-provider value in `calibration.json`, moving 30%
of the way to each new observation (observations outside 0.5–10 bytes/token, or too small to be
meaningful, are ignored). That matters because bytes/4 under-counts Claude by about 2×.

Large diffs go through three tiers:

1. **Fits `budget`**: sent whole, untruncated.
2. **Too big**: clump binary-searches the largest per-hunk body cap in lines that fits, and cuts
   bodies with `[... N more lines]`. The catalog (one line per hunk) and every `@@` header are
   never cut.
3. **Still too big** — the best cap would be under 4 lines, or the catalog alone exceeds the budget:
   per-file summary calls first (files packed into chunks that fit, each summary cached per file, so
   re-grouping a branch after one more commit only re-summarises the files that changed), then one
   grouping call over the full catalog plus one summary per file. Descriptions get vaguer here,
   which is why the threshold is low rather than zero.

Near-identical hunks are also collapsed before the prompt: at least 6 hunks with the same `+a −d`,
from at least 6 distinct files sharing directory, extension and status, become one catalog entry
`cK | M | dir/*.ext (N files) | N similar hunks`, which the model assigns as a whole and the
validator expands — the 45-translated-files case.

What it costs, from `spike/B-grouping.md` (Haiku 5.5 through `claude -p`, cost as reported by the
CLI): a small diff 4.3 s and $0.0009, a mixed one 5.1 s and $0.0033, a branch 6.6 s and $0.0064, a
large full diff 9.6 s and $0.0124 — Haiku was a good default at about $0.001–$0.013 per diff in
4–10 s, and Sonnet was noticeably better on large branches at about 20× the cost.

## Caching and your edits

Results live under `$XDG_CACHE_HOME/clump` (else `~/.cache/clump`):

```
results/<key[0:2]>/<key>.json      one cached grouping
summaries/<key[0:2]>/<key>.json    one cached per-file summary
overrides/<hash[0:2]>/<hash>.json  your edits for one diff
calibration.json                   bytes-per-token per provider
```

The key hashes the diff's canonical form, the provider, the model, the prompt version and
`max_groups`, so the same commit reached two ways — `clump <sha>` or lazygit's coloured `git show` —
hits the same entry. A hit is one file read and no provider call; in the e2e run a revisit showed
`cached` about 16 ms after the debounce. `--no-cache` and `r` make reads miss while writes still
happen, so a regroup refills the cache.

Your edits are saved as an override keyed by the diff hash, separate from model results: reopening
that diff shows them marked `edited` with no provider call, even without a cached result, and it
survives a restart (e2e: edit, close the pane, reopen, back to the commit, still `edited`). Undo is
per session; undoing back to the model's grouping deletes the override, so the next open shows the
cached result again.

`r` with edits asks first: `d` discards them and regroups, `s` keeps them as a snapshot (`before
regroup <time>`) and regroups, `esc` cancels; after a regroup only the snapshots remain. `--json`
and `--plain` print the effective grouping, override included.

## Keys

Press `?` for the same list inside the TUI.

**Navigate**

| key | action |
|---|---|
| `j/k ↓/↑` | next/previous group (lines in the diff pane) |
| `enter l →` | focus the diff pane |
| `esc h ←` | back to the group list |
| `]/[` | next/previous group from anywhere |
| `}/{` | next/previous hunk (diff pane) |
| `ctrl+d/ctrl+u` | half page down/up |
| `a` | toggle grouped / all hunks in file order |

**Search & tools**

| key | action |
|---|---|
| `/` | search the diff pane |
| `n/N` | next/previous match (while searching) |
| `e` | open `$EDITOR` at the hunk |
| `y` | copy a Markdown summary |
| `r` | regroup (bypass the cache) |
| `m` | switch provider |
| `?` | this help |
| `q ctrl+c` | quit |

**Edit groups**

| key | action |
|---|---|
| `x` | mark the hunk (the whole group in the list) |
| `v` | visual mode: mark a range of hunks |
| `M` | move marked hunks to a group or a new one |
| `n` | new empty group |
| `R` | rename group and edit its explanation |
| `d` | delete group (hunks go to Ungrouped) |
| `J/K` | move group down/up |
| `g` | merge this group into another |
| `u/ctrl+r` | undo/redo |

**Watch**

| key | action |
|---|---|
| `A` | toggle auto-group |
| `g` | group now (when not grouped yet) |

`n`/`N` are next/previous match only while a search query is active (`esc` clears it); otherwise `n`
is "new empty group". `g` is "group now" in watch mode when nothing is grouped yet, "merge"
otherwise. `pgup`/`pgdown` (also `space`, `ctrl+f`, `ctrl+b`), `home` and `end` move in the diff pane;
`1`–`9` pick a group by number in the target menus.

## Watch mode with lazygit

Watch mode turns lazygit's diff panel into the trigger: whatever diff lazygit renders is also sent to
a clump pane, which groups it and follows as you move the cursor.

### How it works

lazygit runs a `git.diffRenderers` filter for every diff it renders; `clump push` is that filter:

1. If stdin is a TTY — lazygit's startup probe — it exits at once.
2. It writes the diff to stdout unchanged (or pipes it to `--then '<cmd>'`, e.g. delta), so lazygit's
   panel keeps working and keeps its colour.
3. It strips the ANSI sequences from the copy it forwards, finds the repo root, reads the parent git
   command line, and sends one JSON line to `clump watch` over a private Unix socket for this
   repository, then exits 0 — silently, so with no watch running nothing happens.

`push` reads no config and never blocks the panel: it adds 4.7 ms p50 over `cat` (measured by
`TestPushLatency`), and with no watch running the dial fails at once.

### Setup

`clump lazygit install` prints the change it would make to lazygit's `config.yml` (found via
`LG_CONFIG_FILE`, `$XDG_CONFIG_HOME/lazygit/config.yml`, `~/Library/Application Support/lazygit/config.yml`
on macOS when it exists, or `~/.config/lazygit/config.yml`). `--write`
applies it after a `[y/N]` and a backup to `config.yml.clump-backup-<timestamp>`.

The rules it follows:

- No config file: the snippet becomes the file. No top-level `git:` key: the snippet is appended.
  `git:` without `diffRenderers:`: the clump entry is inserted as its first child.
- `diffRenderers:` in block style: an entry already containing `clump push` means "already
  configured" and nothing changes. Otherwise the first entry's `command:` becomes `clump push
  --then '<old command>'` when that entry is a stdin filter, so your existing renderer still runs;
  if it is not one, the clump entry goes ahead of it.
- Anything it cannot edit safely (flow-style lists, YAML anchors, the legacy `git.paging.pager`, a
  `colorArg: never` on the entry it would wrap) is printed as a snippet to paste by hand.

The renderer to keep can be given as `clump lazygit install --then 'delta --paging=never'`, or set as
`then` in clump's config. `colorArg` stays at lazygit's default `always`: with `never`, lazygit's own
panel loses all colour, because it does not re-colour pager output.

Then start the pane with `clump watch [--repo DIR] [--provider NAME] [--log FILE]`. `--repo` names
the repository (default: the current directory), `--provider` the provider to auto-group with
(default: `cheap`), `--log` writes one line per event (message, classification, cache hit, call, stale
discard) — how the e2e evidence was captured.

### What is followed

| lazygit selection | what happens |
|---|---|
| a commit | followed; the commit message is the header, and it is the model's context |
| a range (`v` then `j`/`k`) | followed as `<oldest>^..<newest>` |
| a stash entry | followed; the ref is resolved to its sha for the header |
| diffing mode (`W`, "diff against") | followed as a two-dot comparison, exactly what lazygit shows |
| a file inside a commit (`enter` on a commit) | cursor only: the pane scrolls to that file, same groups, no call |
| the working tree / staged files | dimmed unless `follow_worktree = true` |
| the branches panel (`log --graph`) | dimmed — not a diff |
| anything else | dimmed |

"Dimmed" means the last result stays on screen, faint, with `not followed` in the header; no call is
made. Merge commits are diffed against their first parent.

Between a selection change and the work there is a 400 ms trailing debounce, so holding `j` down
across 31 selections produced exactly one model call in the e2e run. Messages are deduped by diff
hash (lazygit re-sends the same diff every ~10 s) and out-of-order messages — an older `sent_ns`
than the newest seen — are dropped. A call already in flight is not cancelled when you move on: it
finishes and fills the cache, so the waste is one call and the answer is reused if you come back.
Cached selections show `cached` at once, and editing works in watch as in one-shot: overrides are
keyed by diff hash, so your edits come back with the commit.

## Rex

Rex is the terminal multiplexer this was built for; it also works anywhere else.

### Install

`clump rex install` writes clump's Lua module to `~/.local/share/clump/rex/init.lua` (or
`$XDG_DATA_HOME/clump/rex/init.lua`) and prints the line to add to `~/.config/rex/init.lua`:

```lua
dofile("/Users/you/.local/share/clump/rex/init.lua") -- clump: actions clump_pick, clump_watch, clump_watch_close
```

`clump rex install --write` asks `[y/N]`, backs `~/.config/rex/init.lua` up as
`init.lua.clump-backup-<timestamp>` and appends the line; a file that already loads the module is
left alone.

### Actions

Rex action names cannot contain dots, so the task's `clump.pick`, `clump.watch` and
`clump.watch_close` are spelled `clump_pick`, `clump_watch`, `clump_watch_close`.

| action | what it does |
|---|---|
| `clump_watch` | opens `clump watch --repo <root>` in a split to the right of the focused pane (lazygit), labelled `clump-watch:<root>`; if a pane with that label exists it focuses it instead and reports `created: false` |
| `clump_watch_close` | finds the pane by that label and closes it |
| `clump_pick` | shows a picker ("This branch vs <base>", recent commits) and opens clump's one-shot TUI in a floating layer over the window |

`clump_pick` asks Rex for a picker with `rex.ui.pick`; when Rex cannot show one (`unsupported`,
`no_client`, `busy`, `timeout`, `failed`) it opens `clump --pick` in a floating layer instead. On
the Rex app build tested `rex.ui.pick` returned `unsupported`, so that fallback is what runs.

The repository comes from the acting pane: its OSC 7 working directory, else its foreground
process's cwd, then `git rev-parse --show-toplevel`. Actions take `block` and `repo` args for
scripting (`rex do <file> --action clump_watch repo=/path/to/repo`).

### Bindings

Default bindings, added when the module loads:

| keys | action |
|---|---|
| `ctrl+alt+g` | `clump_pick` |
| `ctrl+alt+w` | `clump_watch` |
| `ctrl+alt+q` | `clump_watch_close` |

Set `CLUMP_NO_BINDINGS = true` before `dofile` to skip them and bind your own with `rex.bind`. Set
`CLUMP_BIN` when `clump` is not on the login shell's `PATH`:

```lua
CLUMP_BIN = "/Users/you/go/bin/clump"
CLUMP_NO_BINDINGS = true
dofile("/Users/you/.local/share/clump/rex/init.lua")
rex.bind("alt+g", "clump_pick")
```

### Program status and theme

With `REX_SESSION` and `REX_BLOCK` set, clump reports its state to Rex as `app=clump`: `working`
while it groups, `blocked kind=question` over the ceiling, `done` with the group count, `error` with
the redacted message — Rex's tab status, working / waiting for you / done / error.

The colour theme follows the terminal's light/dark setting, asked with a terminal query before the
UI starts (Rex answers in about 0.2 ms); `CLUMP_THEME=light` or `dark` overrides the query.

## Troubleshooting

**Start with `clump doctor`.** It prints what clump sees, and sends nothing to any provider:

```
config      /Users/you/.config/clump/config.toml
aliases     default=cheap cheap=claude-haiku strong=claude-sonnet
provider    claude-haiku     command   haiku (runs /usr/local/bin/claude)
limits      ceiling=40000 budget=60000 max_groups=12 auto_group=true
generated   detect=true builtin=true patterns=0 review=0
repo        /Users/you/src/project
socket      /var/folders/.../clump/1a2b3c4d5e6f7a8b.sock (no watch running)
rex         true (REX_SESSION="..." REX_BLOCK="...")
theme       dark (12ms)
```

For a command provider it shows which binary would run, or `(claude not on PATH)`; for an HTTP
provider, whether its `api_key_env` is set; in a repository, the socket and whether a watch is
listening on it.

**"Grouping works in my shell but fails in the watch pane."** The pane runs your login shell's
`PATH`, which can differ from your interactive shell's — the real failure the e2e run hit: inside
Rex panes `claude` resolved to an old install that crashed. Run `clump doctor` inside a Rex pane,
compare the `runs …` path, then give `argv[0]` an absolute path:

```toml
[providers.claude-haiku]
type = "command"
argv = ["/Users/you/.local/bin/claude", "-p", "--model", "haiku", ...]   # the one `which claude` shows in your shell
```

**"Another clump watch is already running for this repo."** One watch per repository, by design: the
socket is per repo root. Close the other pane (`clump_watch_close`, or `q` in it). A watch that died
without cleaning up is handled: the next one removes the stale socket, and `clump doctor` says
whether anything is actually listening.

**Nothing happens when I move in lazygit.** Either the renderer is not configured (run
`clump lazygit install`; lazygit's `config.yml` needs `clump push` under `diffRenderers:`) or the
selection is one clump does not follow (branches panel, working tree without `follow_worktree`,
a file inside a commit) — the pane then says `not followed`.

**"press g to group".** The estimate was over `ceiling` and watch never auto-sends above it, or
auto-group is off (`A`). Press `g` to send it anyway.

**"per-file fallback" in the header.** The model's answer had no usable hunk IDs, so clump showed
one group per file rather than nothing; `--log` records the raw answer. Press `r` to regroup, usually
after `m` to a stronger provider.

**Colours look wrong.** clump asks the terminal whether it is light or dark before drawing, and
assumes dark if it does not answer; set `CLUMP_THEME=light` or `dark`. Inside lazygit a washed-out
panel usually means `colorArg: never` on the renderer — leave it at the default.

**Where is the socket?** `$XDG_RUNTIME_DIR/clump/<first 16 hex of sha256(repo root)>.sock`, else
`$TMPDIR/clump/….sock`; when that path would break macOS's 104-byte limit the socket goes to
`/tmp/clump-<uid>/` instead. Directory 0700, socket 0600, and clump refuses a socket directory that
is a symlink or owned by another user.

## Status

Verified end to end in Rex on 2026-11-25 (Rex server 0.1.0-preview.21, lazygit 0.66.3, macOS) with
`rex capture` of every pane: the split to the right of lazygit, seven commits followed and grouped,
cache hits, a 31-press hold collapsing to one call, a stash entry and a range, the branches panel and
worktree dimmed with zero calls, edits surviving a pane restart, a `kill -9` leaving lazygit in
colour, a one-shot run on a clump commit. The whole run made 14 real calls on Haiku for about $0.006. Captures and
steps: [`docs/e2e/README.md`](docs/e2e/README.md).

Not covered yet:

- `rex.ui.pick` rendering a picker: the app build used returns `unsupported`, so `clump_pick` falls
  back to its own picker.
- Linux: it builds and the code paths exist (`/proc/<ppid>/cmdline`), but only macOS was run.
- Unsolicited theme-change notifications: the theme is read at startup only.
- A merge commit selected in lazygit, and diffing mode (`W`): unit tests and the recorded lazygit
  captures, not an e2e step.

## Development

The suite runs with `go test -race ./...` and has a property test over the validator, a random
sequence of edit operations with `Check` after every step, golden fixtures for the parser and the
renderers, and provider fakes (`printf`/`cat` scripts, `httptest` servers); no test makes a real
network call or needs a model.

Useful pieces:

- `scripts/mkfixture.sh DIR [N]` builds a scratch repository with a rename, a copy, a binary file, a
  mode change, a path without a trailing newline, a quoted unicode path, a path with spaces, a
  deletion, a merge, `N` filler commits, a feature branch and a stash entry.
- `CLUMP_PUSH_LATENCY=1` also asserts that `clump push` stays within its latency budget over `cat`,
  which is why it is off by default.
- `spike/` holds the measurements this build rests on: `spike/A-lazygit.md` (what lazygit pipes to a
  diff filter, and how a selection is identified from the parent git argv), `spike/B-grouping.md`
  (quality, cost and latency per provider per diff size, the bytes-per-token measurement, the
  injection test) and `spike/C-rex.md` (the Rex APIs used).
- `internal/chromalexers` is `chroma/v2/lexers` vendored so the registry is built on first use rather
  than in package `init`: the stock lexers cost 3.5 ms of start-up, and lazygit runs `clump push` on
  every diff it renders.

## License

License: not chosen yet. The vendored syntax lexers under `internal/chromalexers` are MIT-licensed
Chroma code, Copyright (C) 2017 Alec Thomas; the text is at `internal/chromalexers/LICENSE`.
