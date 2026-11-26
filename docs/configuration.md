# Configuration

`clump` reads `~/.config/clump/config.toml`. `CLUMP_CONFIG` overrides the whole path and
`XDG_CONFIG_HOME` the directory it is looked for in; the file is optional.

`clump config init` prints the commented default (embedded in the binary) and `clump config path` the
path it would read. A file overlays the defaults: a top-level key it sets replaces the default value,
and a `[providers.<name>]` table it defines replaces the built-in provider of that name, or adds one.

## Top-level keys

| key | default | meaning |
|---|---|---|
| `default` | `"cheap"` | provider or alias one-shot mode uses when `--provider` is not given |
| `cheap` | `"claude-haiku"` | alias: the cheap provider. Watch mode auto-groups with it |
| `strong` | `"claude-sonnet"` | alias: the stronger provider, for big branches |
| `ceiling` | `125000` | estimated tokens above which one-shot asks first and watch never auto-sends (`0` disables the check) |
| `budget` | `120000` | token budget for one prompt; larger diffs are truncated or summarised |
| `max_groups` | `12` | most groups the model may return (1–50) |
| `auto_group` | `true` | watch mode groups every followed selection without asking |
| `follow_worktree` | `false` | watch mode also follows lazygit's working-tree/staged diffs |
| `then` | `""` | diff renderer `clump lazygit install` keeps behind `clump push`, e.g. `"delta --paging=never"` |
| `line_numbers` | `true` | show old/new line numbers beside the diff (`#` toggles them in the TUI) |
| `side_by_side` | `false` | draw the diff old-left / new-right (`s` toggles it in the TUI) |
| `handoff` | `""` | agent `H` (TUI) and `clump handoff` use; empty picks the first of `claude`, `codex`, `opencode` whose binary is on PATH |

## Coding agents (handoff)

`H` in the TUI and `clump handoff` send the review's comments to a coding agent CLI as its initial
prompt and then get out of the way. Agents live in `[agents.<name>]`, deliberately separate from
`[providers.<name>]`: a provider runs isolated in a temp dir so it cannot see the repository, while
a handoff agent must run in it, with your environment and your login. The built-ins are `claude`,
`codex` and `opencode`; a table replacing one of them must, like providers, set everything itself.

| key | required | meaning |
|---|---|---|
| `argv` | yes | the cli to launch: the prompt is appended as its **last** element (`opencode` wants `["opencode", "--prompt"]`) |
| `where` | no | `replace` (default: clump exits, its process becomes the agent in this terminal), `pane` (a new `rex split`, else `tmux split-window`; clump stays open — the only shape watch mode offers), or `print` (the prompt on stdout, nothing launched) |

```
[agents.aider]
argv = ["aider"]
where = "pane"
```

The comments that go out are stamped (`clump comments --open` lists the rest); nothing about the
agent is supervised, streamed or waited on. See
[ADR-0058](adr/0058-handoff-to-agent-cli.md) for the shape of the whole contract.

## Generated files

Lock files, codegen output and vendored code are not worth reading line by line. Their changes
follow from a change somewhere else, and that change is what you review. clump does not send their
hunks to the model, so you pay for fewer tokens and the groups have less noise. The model is told
only their paths ("generated files also changed: …"), so it can still title the schema change that
a migration came from. In the TUI they sit in a **Generated files** group at the end of the list.
They are folded, each header says why the file was skipped, and `z` unfolds one when you want to
look.

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

## Providers

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
