# clump design

`clump` groups the hunks of a git diff into logical changes with an LLM, shows them in a TUI
next to the raw diff, and lets you fix the groups by hand. It runs one-shot (`clump <rev>`,
`clump A...B`, `clump --branch`, a patch on stdin) or as a **watch** pane beside lazygit that
follows lazygit's selection. It is specialised for Rex and works in any terminal.

The requirements are `../DESIGN.md` (the task spec). The spike that settled the open questions
is `spike/` (`spike/README.md`, `A-lazygit.md`, `B-grouping.md`, `C-rex.md`). This file records
the decisions and contracts the build follows. Section "Decisions made during build" lists
choices made after the spike.

## 1. Packages

```
cmd/clump            CLI: flags, subcommands (watch, push, config init, rex install, lazygit install)
internal/gitdiff     unified-diff parser, canonical form + hash, git loaders
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
internal/chromalexers  vendored chroma/v2/lexers with a lazily built registry (see §14)
rex/                 init.lua (Rex actions) + embed.go (package rexlua)
scripts/             mkfixture.sh (scratch test repo), mkgolden.sh (parser golden fixtures)
```

Import graph (arrows point at dependencies; no cycles):

```
cmd/clump -> tui, engine, config, cache, provider, gitdiff, ipc, lazygit, rex, rexlua
tui       -> engine, group, gitdiff, cache, config, lazygit, ipc, report, tui/*
tui/diffview -> chromalexers, chroma/v2 (core), view;  report -> gitdiff, group, cache
engine    -> group, catalog, gitdiff, provider, cache, config
group     -> catalog, gitdiff
catalog   -> gitdiff
cache     -> group, provider
config    -> provider
gitdiff, provider, ipc, lazygit, rex -> stdlib (+ golang.org/x/sys for ipc, rex)
```

Dependencies (the whole list): `charm.land/bubbletea/v2` v2.1.2, `charm.land/bubbles/v2` v2.2.2,
`charm.land/lipgloss/v2` v2.1.1 (the current stable major of each), `github.com/alecthomas/chroma/v2`,
`github.com/BurntSushi/toml`, `golang.org/x/sys`. The Anthropic and OpenAI-compatible APIs are
spoken over raw `net/http`. `CGO_ENABLED=0`; one static binary; `go install
github.com/k22i/clump/cmd/clump@latest`.

## 2. Diff input and parsing

### Loaders (`gitdiff`)

Every git call is `git -C <repo> <GitArgs> <subcommand> <DiffFlags> ...` where
`GitArgs = -c core.quotepath=off -c diff.noprefix=false -c diff.mnemonicPrefix=false
-c diff.srcPrefix=a/ -c diff.dstPrefix=b/ -c color.ui=never` pins the output against user
config and `DiffFlags = --no-color --no-ext-diff --unified=3 --find-renames --submodule`
matches what lazygit asks for.

| input | git | context for the prompt |
|---|---|---|
| `clump <rev>` | `show --format= <DiffFlags> --diff-merges=first-parent <rev>` | `COMMIT MESSAGE:` + `git log -1` message |
| `clump A...B` | `diff <DiffFlags> A...B` (merge base, like a PR) | `COMMITS IN THIS RANGE (newest first):` + up to 60 messages of `git log --no-merges A..B` |
| `clump --branch` | `A...B` with A = `origin/HEAD`'s target, else `main`, else `master`; B = `HEAD` | as above |
| `clump --pr 123` | `gh pr diff 123 --color=never` in the repo root | `PULL REQUEST: <title>` + body + `COMMITS IN THIS PULL REQUEST (oldest first):` + the `messageHeadline`s of `gh pr view --json number,title,body,commits` |
| `clump --pr OWNER/REPO#123` / URL | the same calls with `--repo OWNER/REPO`, no clone needed | as above |
| stdin | the patch as given | its `commit` header, if any |
| watch range / diffing mode | `diff <DiffFlags> X Y` (two-dot, what lazygit shows) | messages of `X..Y` |

Context is capped at 6000 bytes (`[... truncated]`). **Merge commits** are always diffed
against the first parent. The parser never accepts `diff --cc`: it returns an error wrapping
`gitdiff.ErrCombined`, and watch re-loads such a commit with `LoadRev`. (A clean merge's plain
`git show` has a `--stat` block but no patch at all, verified on the fixture repo; watch treats a
`git show` of a commit with a `Merge:` header and no files the same way.)

`--pr` is the one loader that does not run git: `runGh` (github.go) shells out to the `gh` CLI
for both the patch and the metadata, so clump never handles a GitHub token itself — gh's login
is the credential, `doctor` reports whether the binary is on PATH, and a missing gh is an error
naming the install URL. The spec grammar accepts `123`, `#123`, `OWNER/REPO#123` and pull request
URLs (optionally with `.diff`/`.patch` suffix); a number-only spec resolves against the
repository gh is run in, so it needs a clone — the `--repo`-named forms do not. gh lists a pull
request's commits oldest first, so the context keeps that order (a review reads a PR top to
bottom; the git range loaders stay newest first).

### Parser rules

Ported from `spike/groupcli/parse.go` (which parsed the 200 latest lazygit commits in both
plain and coloured form):

- Strip SGR (`ESC\[[0-9;]*m`) first. With lazygit's default `colorArg: always` that is the only
  escape sequence in the input (A-lazygit.md), and the stripped text is byte-identical to
  `colorArg: never`.
- A preamble before the first `diff --git` (commit header, `Merge:`, Author/Date, indented
  message, `---`, the `--stat` block) is read for `commit <sha>` and the message only.
- Inside a hunk the `@@` counts decide what a line is, so a body line `--- old rule` is never a
  file header. `\ No newline at end of file` belongs to the hunk.
- Paths: `diff --git` line split (quoted, equal halves for spaces, else ` b/`), overridden by
  `rename from/to`, `copy from/to`, `---`/`+++` (trailing TAB trimmed; git adds one after paths
  with spaces). Quoted paths are C-unquoted (`"caf\303\251.txt"` -> `café.txt`).
- Status: `A` new file, `D` deleted, `R` rename, `C` copy, `T` mode only, else `M`.
- Binary: `Binary files ... differ` or `GIT binary patch` (payload skipped).
- Files with no text hunk (binary, pure rename/copy, mode-only, empty new/deleted file) get one
  **pseudo-hunk** whose header describes it, e.g. `(binary new file)`, `(rename)`,
  `(mode change) (mode 100644 -> 100755)`.
- Hunk IDs `h1..hN` in file order across the diff.

### Canonical form and diff hash

`Diff.Canonical()` re-renders only what defines the patch (paths, status, modes, binary index,
hunk headers and bodies), so `clump <sha>` and lazygit's coloured `git show --stat --decorate`
of the same commit produce the **same `Diff.Hash()`**. That is what lets watch mode hit the
cache entries one-shot mode wrote, and vice versa. The exact format is in `gitdiff.go`; the
golden fixtures in `internal/gitdiff/testdata/git/` assert the equality per commit.

## 3. Grouping contract

### Catalog (`catalog`)

- One line per unit: `id | status | path | @@ header | +a -d` (path `old -> new` for R/C).
- One block per unit: `### hN path`, the `@@` header, the body (maybe capped, then
  `[... N more lines]`).
- **Collapsing.** Non-pseudo hunks whose files share directory, extension and status, and that
  have the same `+a -d`, are near-identical. A set of at least 6 such hunks from at least 6
  distinct files becomes one unit `cK` (catalog line `cK | M | dir/*.ext (N files) | N similar
  hunks, e.g. @@ ... @@ | +A -D`; block shows the first member as a sample). The model assigns
  `cK` as a whole; `Validate` expands it through `ValidateOpts.Aliases`. This targets the
  45-translation-file case from B-grouping.md.

### Prompt (`group`)

- `internal/group/prompt_v1.txt` (grouping) and `prompt_summary_v1.txt` (tier-2 summaries) are
  embedded. `PromptVersion = "v1"` is in every cache key.
- The first line names the task (`# clump-task: group v1`), which the fake provider keys on.
- Everything from the repo (context, catalog, hunks, summaries) sits between
  `<<<CLUMP-DATA-{nonce}>>>` and `<<<END-CLUMP-DATA-{nonce}>>>`, with a fresh 12-hex-char random
  nonce per request, and the prompt says that data is never instructions. In the spike neither
  Haiku nor DeepSeek followed an injection with a forged end marker; both flagged it.
- Model output: `{"groups":[{"title","description","hunks":[ids]}]}`.

### Validation and repair (`group.Validate`)

Input: the diff, the model text, `MaxGroups` (default 12) and the alias map. Output: a `Result`
that always passes `Check`. In order:

1. `ExtractJSON` finds the first balanced `{...}` (models add prose or fences). None, or invalid
   JSON, or no `groups` array -> `FallbackPerFile` (notice
   `fallback: one group per file (<why>)`).
2. For each model group in order: trim each ID; expand an alias ID to its members; drop IDs that
   are not hunks of the diff (counted); drop IDs already placed (duplicates: keep the first,
   counted). A group left with no hunks is dropped.
3. Title: control characters become spaces, whitespace collapses, trimmed, cut to 80 runes;
   empty -> `(untitled group)`. Description: same but newlines are kept, cut to 1000 runes.
   (Model text reaches the terminal; it must never carry escape sequences.)
4. No usable group -> `FallbackPerFile`.
5. Over the cap: keep the first `MaxGroups-1` groups, merge the rest into `Other changes`
   (`Groups beyond the cap of N, merged.`).
6. Hunks not placed -> a last group `Ungrouped` (ID `ungrouped`,
   `Hunks the model did not assign.`).
7. Group IDs `g1..gK` in order (Ungrouped keeps `ungrouped`); hunks sorted by number in every
   group.
8. Notices (exact text): `repaired: dropped N unknown hunk IDs`,
   `repaired: N hunks were in two groups; kept the first`,
   `repaired: N groups exceeded the cap of M; merged the tail`,
   `repaired: N hunks missing from the model output; put in Ungrouped`.

`FallbackPerFile`: one group per file, ID `g1..`, title = path, description
`All changes to <path>.`, `Fallback: true`.

### Invariants (`group.Check`)

1. group IDs non-empty and unique; 2. titles non-empty; 3. every hunk of the diff in exactly one
group; 4. no unknown IDs; 5. hunks sorted ascending in each group; 6. `ungrouped` never empty.
The validator, every edit op and every undo/redo keep them.

### Generated files (`generated`)

`Engine.MarkGenerated` runs once per loaded diff (one-shot, TUI `open`, watch) and sets
`File.Generated` to a reason, or "". Precedence: config `review` (never) > `.gitattributes`
`linguist-generated` / `linguist-vendored` via one `git check-attr -z --stdin` (set: generated;
unset/false: never) > config `patterns` > `generated.Builtin` > a comment marker in the first 10
lines visible in the diff (`Code generated … DO NOT EDIT`, `@generated`, `<auto-generated`,
"auto-generated", "generated … do not edit"), drizzle's `--> statement-breakpoint` anywhere
in a `.sql` file, or a `.json` file whose first 10 lines carry `"dialect"` and `"prevId(s)"`
(a drizzle-kit snapshot outside the built-in paths). Patterns are gitignore-style globs (`generated.Compile`) and are validated with
the config.

Generated hunks are never sent: `catalog.Units` and tier 2 skip them, and the prompt context
lists their paths (at most 40). `Validate` does not count them as missing or known. It and
`FallbackPerFile` append a last group `generated` ("Generated files"). When every hunk is generated,
`AllGenerated` is the result, with no call and no cache write. After that the group is ordinary and
can be edited.

## 4. Large diffs (`group.Build`)

Token estimates use the provider's `bytes_per_token` (calibrated, see §7) — never bytes/4,
which under-counts Claude by ~2× (B-grouping.md).

- **Tier 0**: the whole prompt fits `budget` -> send it untruncated.
- **Tier 1**: binary-search the largest per-hunk body cap (lines) that fits `budget`; cut bodies
  end with `[... N more lines]`. The catalog and every `@@` header are never cut.
- **Tier 2** when the best cap would be below 4 lines, or the catalog alone (cap 0) exceeds the
  budget:
  1. Per-file summary calls. Files are packed in diff order into chunks that fit the budget
     (each file's block rendered with the largest cap that fits a chunk on its own; a chunk
     holds whole files). Each answer is `{"files":[{"path","summary"}]}`, cached **per file**
     under `SummaryKey(file hash, provider, model)`, so re-grouping a branch after one more commit
     only re-summarises the files that changed.
  2. One grouping call over the full catalog plus one summary per file (bodies omitted). If that
     still exceeds the budget it is sent anyway and the estimate says so.
- The **estimate** (calls, tokens, cost) is shown before sending; above `ceiling` one-shot asks
  and watch never auto-sends (it shows `press g to group (~N tokens, ~$X)`).
- Spike numbers: a 113-hunk branch is ~10k tokens with headers only, so a budget below that
  cannot be met by truncation; header-only grouping stays structurally fine but descriptions
  start to be invented, hence the cap-4 threshold for tier 2.

## 5. Providers

`Provider.Complete(ctx, prompt) (text, Usage, error)`. Implementations:

- **command**: argv with the prompt on stdin, cwd `os.TempDir()` (so an agent CLI never sees
  repo context), stdout read as `raw`, `claude-json` (`result`, `is_error`, `total_cost_usd`,
  usage incl. cache tokens) or `opencode-json` (JSONL `text` / `step_finish` / `error` events).
- **openai**: POST `base_url/chat/completions`, `temperature: 0`,
  `response_format: {"type":"json_object"}`, Bearer key from `api_key_env` (optional, for
  Ollama).
- **anthropic**: POST `base_url/v1/messages` (`x-api-key`, `anthropic-version: 2023-06-01`),
  key from `api_key_env` (default `ANTHROPIC_API_KEY`). No `cache_control`: about half the cost of
  `claude -p`, which bills a 1-hour cache write on every call (B-grouping.md).
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
`sk-...`, `Bearer ...`, `x-api-key: ...`).

## 6. Engine (`engine`)

One pipeline for every mode:

1. `diffHash = Diff.Hash()`; `key = cache.Key(diffHash, provider, modelID, maxGroups)`.
2. Override for `diffHash`? -> show it (`edited`), no call.
3. Cache hit (`key`) -> show it (`cached`), no call. A hit is one file read.
4. `group.Build` -> `Plan` and `Estimate{tier, calls, tokens, cost, over ceiling}`.
5. Over the ceiling -> one-shot asks (`--yes` skips), watch shows `press g`.
6. Run: tier 1 one call; tier 2 the uncached summary calls (sequentially, one in flight) then
   the grouping call. `Validate`, write the cache entry, calibrate bytes per token.

`--dry-run` prints exactly the prompt(s) that would be sent (tier 2: the summary prompts, then
the grouping prompt with `(summary pending)` placeholders) and the estimate, and sends nothing.

## 7. Cache

Root `$XDG_CACHE_HOME/clump` (else `~/.cache/clump`):

```
results/<key[0:2]>/<key>.json        Entry{version, key, diff_hash, provider, model, prompt_version, max_groups, created_at, result, usage}
summaries/<key[0:2]>/<key>.json      Summary{version, key, path, summary, created_at}
overrides/<hash[0:2]>/<hash>.json    Override (user edits; §8)
calibration.json                     {"<provider>": bytes_per_token}
reviewed/<key[0]>.json               {"version":1,"hunks":{"<ReviewKey>":"<time>"}} (16 shards)
```

- `Key = sha256(diffHash \0 provider \0 modelID \0 PromptVersion \0 max_groups)`, plus
  `\0 skip=<generated.Signature>` when the diff has generated files (`KeyWith`; without any
  the key is unchanged). `modelID` is `model`, else the argv, else `base_url`.
- Reviewed marks are keyed by `Hunk.ReviewKey() = sha256(DisplayPath \0 body lines)` (the
  header for a pseudo-hunk; no line numbers), so they follow a change across diffs and reset
  when its text changes. `SetReviewed` re-reads each shard before writing it and drops marks
  older than 180 days; a corrupt shard reads as empty.
- Writes: temp file in the same dir, then rename (atomic). Files 0600, dirs 0700.
- A hit is exactly one `ReadFile` (tested with a counting FS).
- `--no-cache` / `r`: reads miss, writes still happen. Overrides are never bypassed.
- Calibration: `obs = prompt bytes / (input tokens - overhead)`, ignored if the denominator
  < 100 or `obs` outside [0.5, 10]; stored `0.7*old + 0.3*obs`.

## 8. Edit model

```go
type Group    struct { ID, Title, Description string; Hunks []string }   // ID "g1".. or "ungrouped"
type Grouping struct { Groups []Group }
```

Pure operations; each returns a new `Grouping` or `(input, error)`, and never mutates its input:

| op | rule |
|---|---|
| `Move(g, hunks, to)` | `hunks` non-empty (`ErrNoHunks`), each in some group (`ErrNoHunk`), duplicates ignored; `to` exists (`ErrNoGroup`) or is `ungrouped` (created at the end if absent). Hunks leave their groups and join `to`, which is re-sorted. Source groups may become empty and stay, except `ungrouped`, which is removed when empty. Moving hunks already in `to` is a no-op. |
| `MoveToNew(g, hunks, title)` | like Move into a new group `NextGroupID(g)` appended at the end; trimmed title must be non-empty (`ErrEmptyTitle`). |
| `NewGroup(g, title)` | appends an empty group `NextGroupID(g)`; `ErrEmptyTitle`. |
| `Rename(g, id, title, desc)` | `ErrNoGroup`; `ErrEmptyTitle`; `ErrUngroupedOp` for `ungrouped`. Title/desc trimmed. |
| `Delete(g, id)` | `ErrNoGroup`; `ErrUngroupedOp` for `ungrouped`. Its hunks go to `ungrouped` (created at the end if absent; nothing is created for an empty group). |
| `Reorder(g, id, delta)` | `ErrNoGroup`; new index `clamp(i+delta, 0, len-1)`; others keep their order. |
| `Merge(g, src, dst)` | `ErrNoGroup` for either; `ErrSameGroup`; src's hunks join dst (sorted), src removed; dst keeps its title. Merging `ungrouped` away is allowed, and so is merging into it. |
| `NextGroupID(g)` | `g<max N + 1>` over IDs `g<digits>`. IDs are never reused while a group holds them and never renumbered. |

Empty groups are allowed (a fresh `n` group), except `ungrouped`.

`History{past, cur, future}` is snapshot-based and unlimited: `Do(next)` pushes `cur` (no-op
if `Equal`), clears `future`; `Undo`/`Redo` move one step; `Reset` (after a regroup) clears
both stacks. `Apply(op)` runs an op and `Do`es its result.

### Overrides

Saved after every edit, keyed by diff hash, separate from model results:

```json
{"version":1,"diff_hash":"…","base_cache_key":"…","edited_at":"…",
 "grouping":{"groups":[…]},"snapshots":[{"name":"…","at":"…","grouping":{…}}]}
```

- Opening a diff with an override shows the override, marked `edited`, without a model call
  (even when there is no cached result). An override that fails `Check` is ignored with a
  notice (it cannot happen for a matching hash, but the file is user-editable).
- The undo stack is per session. Undoing back to the model's grouping deletes the override
  (when no snapshots are kept), so the next open shows the cached result, not "edited".
- `r` regroup with edits asks: `d` discard edits, `s` keep them as a snapshot
  (`before regroup <time>`) and regroup, `esc` cancel. After a regroup the override's
  `grouping` is empty (no active edits) and only its `snapshots` remain.
- `--json` and `y`/`--plain` print the effective grouping (override if any).

## 9. TUI

Layout: group list left (title, `+a −d`, hunk count, markers), right pane = selected group's
title and explanation on top, its hunks below grouped by file. Side by side when the width is
at least 100 columns, stacked (inline) below. Header line: subject, short ref, provider, `cached`
/ `edited`; watch adds `auto` state and `dimmed`. Notices bar at the bottom.

Async flow: the UI opens at once with a spinner; the raw file list (the `a` view) is browsable
while the groups load; `groupsReadyMsg` fills them in. Provider calls run in `tea.Cmd`s with a
context the model can cancel.

Keys: `j/k` groups, `⏎` focus diff pane (`esc` back), `]`/`[` next/prev group anywhere, `a`
grouped/all, `/` search, `n`/`N` next/prev match, `e` open `$EDITOR` at `+line file`, `y` copy
Markdown, `r` regroup, `m` provider, `?` help, `q` quit. Editing: `x` mark hunk, `v` visual range,
`M` move marks, `n` new group (while a search query is active, `n`/`N` are next/previous match
instead; `esc` clears the query), `R` rename/explain, `d` delete, `J`/`K` reorder, `g` merge (one-shot) / group now
(watch, when nothing is grouped yet), `u` undo, `ctrl+r` redo. Watch: `A` auto-group toggle.

Review: `c` marks the file block under the diff cursor (or the selected group, from the list)
reviewed. A finished group moves the cursor to the next group that is not reviewed, wrapping
around. The Generated files group and empty groups are not counted in the header's `N/M reviewed`.
A file block is folded by default when it is generated or fully reviewed. `z` (block) and `Z` (pane)
flip that, remembered per `(group ID, file)` (`""` in the all view) until the block's reviewed
state changes, and reset on a new grouping. A folded header carries `BlockHunks`, so `x`, `e`,
`}`/`{` act on it. `#` toggles the line-number gutter (`line_numbers`, default on). `Line.Gutter`
excludes the gutter from search.

Theme: `CLUMP_THEME=light|dark`, else the `CSI ? 996 n` query (DA1 sentinel, 100 ms) before
Bubble Tea starts; Rex answers `ESC[?997;1n` (dark) in 0.2 ms (C-rex.md).

Rex program status (only when `REX_SESSION` and `REX_BLOCK` are set), always `app=clump`:
`working` while grouping, `blocked kind=question` over the ceiling, `done` with the group count,
`error` with the redacted message. Emitted with `tea.Raw`.

## 10. push and watch

### `clump push` (lazygit `diffRenderers` stdinFilter)

1. stdin is a TTY -> exit 0 at once (lazygit's startup probe; A-lazygit.md).
2. Read all stdin. Write it to stdout unchanged **first** — or, with `--then '<cmd>'`, pipe it to
   `sh -c <cmd>` with stdout/stderr inherited (e.g. delta).
3. Strip SGR for the payload. Repo root: walk up from the cwd to `.git` (git runs the pager in
   the repo root). Parent argv via sysctl `kern.procargs2` (darwin) or `/proc/<ppid>/cmdline`.
4. Dial `SocketPath(root)` with a 50 ms timeout, write one JSON line with a 100 ms deadline,
   close. Exit 0 always, silently. No config is read.

Budget: ≤ 5 ms p50 over `cat` without a listener (spike: ~3 ms).

### IPC

Socket: `$XDG_RUNTIME_DIR/clump/<sha256(root)[:16 hex]>.sock`, else `$TMPDIR/clump/...`.
Directory 0700, socket 0600. Message: `{v:1, sent_ns, repo_root, git_argv, rex_block, diff}`,
one JSON line per connection, ≤ 64 MiB. `watch` removes a stale socket (dial fails) before
listening and refuses to start (`ErrInUse`) when a live watch answers.

### Selection rules (`lazygit.Classify` + `Decide`)

From the parent git argv (global options `-C`, `-c` skipped; `argv[0]` may be any path to git):

| argv | kind | action |
|---|---|---|
| `show ... <sha> --` | commit | follow |
| `diff ... A^ B ...` (no pathspec, or pathspec `.`) | range | follow (`A^..B`) |
| `diff ... A^ A -- <paths>` | file-in-commit | cursor only (move to that file if the commit is shown; else dim) |
| `stash show ... refs/stash@{N}` | stash | follow; watch resolves the ref to a sha for the header |
| `diff ... X Y ...` (X without `^`) | diffmode | follow (two-dot, as lazygit shows it) |
| `diff [--cached] -- <paths>`, `diff --no-index ...` | worktree | dim unless `follow_worktree = true` |
| `log ...` | log (branches panel) | dim |
| anything else | unknown | dim |
| argv unreadable | stdin `commit <sha>` header -> commit; else unknown | |

### Watch state machine

```
            push msg (sent_ns >= newest)
   ┌──────────────────────────────────────────┐
   │                                          v
 [Idle] ──msg──> [Debouncing] ──400 ms quiet──> classify
   ^               │  ^  msg: replace pending,   │
   │               └──┘  restart timer           │
   │                                             ├─ dim ──────────> mark shown result dimmed ──> [Idle]
   │                                             ├─ cursor only ──> move cursor (same commit) ─> [Idle]
   │                                             └─ follow
   │                                                  │ parse (ErrCombined/merge -> git LoadRev)
   │                                                  │ hash == shown hash -> undim ───────────> [Idle]
   │                                                  v
   │                                          [Resolve] git: context/title, stash sha
   │                                                  │ show diff (raw list browsable)
   │                                                  ├─ override  -> show "edited" ───────────> [Idle]
   │                                                  ├─ cache hit -> show "cached" ───────────> [Idle]
   │                                                  ├─ !auto or over ceiling -> "press g" ───> [Idle]
   │                                                  └─ need call -> request(job)
   │                                                                     │
   │       request: no call in flight -> start; else queued := job (latest wins)
   │       new selection that needs no call -> queued := nil
   │       call done: write cache; result for shown hash -> display, else discard (stale);
   │                  then start queued, if any
   └──────────────────────────────────────────────────────────────────────┘
   q: cancel the in-flight call's context, drop the queue, close the listener, quit.
```

- Out-of-order messages: drop `sent_ns` older than the newest seen (A-lazygit.md: filter
  processes can finish out of order).
- Dedupe by diff hash: lazygit re-sends the same diff every ~10 s and on re-entering a panel.
- In-flight calls are not cancelled on selection change: they finish and fill the cache, so
  with Haiku at ~5 s the waste is bounded to one call (spike README decision 2).
- All TUI keys work in watch, including editing (overrides keyed by diff hash).
- `--log FILE` writes one line per event (msg, classify, cache hit, call start/end, stale
  discard) for the e2e evidence.

## 11. Rex

Verified in C-rex.md (Rex server 0.1.0-preview.21, app 0.1.0 (2061)):

- `clump_watch`: `ctx.block_id` (the lazygit pane) -> its `pwd` (`file://host/path`, empty
  until OSC 7, fallback `process().foreground.cwd`) -> `git rev-parse --show-toplevel` -> find a
  block labelled `clump-watch:<root>` with `list_blocks` and `focus_block` it, else
  `rex.session.new_split{anchor_id=…, direction="horizontal", side="after", focus=false,
  layout=rex.layout.block{flavor="com.superlogical.terminal.shell", label="clump-watch:"..root,
  options={cwd=root, command={"clump","watch","--repo",root}}}}`. The label survives title
  changes; there is no `env` option.
- `clump_watch_close`: find by label, `block.close`.
- `clump_pick`: `rex.ui.pick` (items: "this branch vs default", recent commits); on
  `unsupported`/`no_client`/`busy`/`timeout`/`failed` open `clump --pick` in a floating
  `new_layer`, where clump shows its own picker. `rex.ui.pick` returned `unsupported` with the
  current app build.
- Action names cannot contain dots: the task's `clump.pick`, `clump.watch`, `clump.watch_close`
  are `clump_pick`, `clump_watch`, `clump_watch_close`.
- `clump rex install` writes the module to `~/.local/share/clump/rex/init.lua` (clump's own
  file) and prints the `dofile` line; only `--write` (with y/N and a backup) edits
  `~/.config/rex/init.lua`.

## 12. lazygit install

`clump lazygit install` prints the change; `--write` applies it after a y/N and a backup
(`config.yml.clump-backup-<ts>`). Rules (line-based; no YAML library):

1. No config file -> the snippet is the whole file.
2. No top-level `git:` key -> append the snippet.
3. `git:` without `diffRenderers:` -> insert `diffRenderers:` with the clump entry as the first
   child of `git:` (using the block's indent).
4. `diffRenderers:` in block style: if an entry's command contains `clump push`, report
   "already installed" and change nothing. Else rewrite the **first** entry's `command:` to
   `clump push --then '<old command>'` when it is a stdinFilter (no `type:` or
   `type: stdinFilter`); otherwise insert the clump entry first.
5. Anything else (flow-style lists, anchors, the legacy `git.paging.pager`, a `colorArg: never`
   on the entry it would wrap) -> print the snippet and instructions, never write.

`colorArg` stays at its default (`always`): with `never` lazygit's own panel loses colour
(A-lazygit.md).

## 13. Testing

- Table tests over every fixture in `internal/gitdiff/testdata` (lazygit captures from the
  spike, golden git fixtures from `scripts/mkfixture.sh`, edge cases); a test that every hunk's
  body matches its `@@` counts; `*.show.txt` and `*.lazygit.txt` of the same commit hash equal.
- Validator: repair table + a 2000-case property test. Edit ops: a ≥5000-step random sequence
  of ops, undos and redos with `Check` after every step and the seed logged.
- Providers against `httptest` fakes and `printf`/`cat` scripts; never a real network call or
  model in tests.
- TUI: pure render functions with golden files (colour off); scripted `Update` key sequences.
- IPC: a real listener in a temp dir. push: a benchmark through the real binary.
- E2E (Process step 4): `docs/e2e/README.md`.

## 14. Decisions made during build

- **Go 1.27** in go.mod (the user asked for 1.27 during Phase 0; the brief had said 1.25). This
  allows the latest charm v2 releases, which require Go ≥ 1.26.
- Bubble Tea v2 has built-in OSC 7501 support (`View.ProgramStatus`); clump still uses its own
  `internal/rex` encoder (gated on Rex, emitted with `tea.Raw`) so watch and non-TUI paths share
  one implementation and its limits are tested.
- Overrides live under the cache root (`overrides/`), as the brief specifies, separate from
  `results/`.
- Watch uses the diff pushed by lazygit (parsed) rather than re-running git, except for merges
  (`ErrCombined` / empty merge show) and for context (commit messages), which come from git.
- `push` reads no config; `then` in config.toml is only used by `clump lazygit install`.
- Copies are only detected when git reports them (the loaders do not pass `--find-copies`), to
  match lazygit's diffs and keep hashes equal.
- **TUI split into sub-packages** (`internal/tui/view`, `diffview`, `rawview`, `listview`,
  `helpview`, `sys`) so render workers could build and test in parallel without breaking each
  other's builds. The root model (`internal/tui`) owns all state.
- **No `bubbles/textinput`**: it pulls in `github.com/atotto/clipboard`, outside the agreed
  dependency set. `internal/tui/input.go` is a 100-line line editor. Bubbles is used for the
  spinner only.
- **`internal/report`** holds the Markdown and JSON renderers (needed by `--plain` in Phase 1,
  before the TUI existed); the clipboard part lives in `tui/sys`.
- **Key conflicts resolved**: `n`/`N` are next/previous match only while a search query is active
  (else `n` is "new group"); `g` is "group now" in watch when nothing is grouped yet, else merge.
- **`push` start-up time.** Every package `init` runs before `main`, and the binary links the
  TUI. `chroma/v2/styles` (3.6 ms) and `chroma/v2/lexers` (3.5 ms, plus three analyser `init`s
  that force the registry) made `push` 11.5 ms slower than `cat`. Fixes: the two syntax palettes
  are built in code (`diffview.palette`, GitHub light/dark colours), and `chroma/v2/lexers` is
  vendored as `internal/chromalexers` (MIT, LICENSE kept) with the registry built on first use,
  the analysers run then, and its Rule/Token literals keyed for `go vet`. No new module:
  `github.com/dlclark/regexp2/v2` (already chroma's dependency) becomes a direct requirement
  because the vendored code imports it. Result: push adds 4.7 ms p50 over `cat`
  (`TestPushLatency`; asserted with `CLUMP_PUSH_LATENCY=1`).
- **Socket path length.** Unix socket paths are limited to 104 bytes on macOS. When
  `SocketDir()` would exceed that, `SocketPath` uses `/tmp/clump-<uid>/`; `Listen` refuses a
  socket directory that is a symlink or owned by another user (the fallback dir is shared).
- **Watch resolves titles itself**: `gitdiff.CommitMessage`, `RangeContext`, `CommitContext` and
  `ShortRev` were added so watch can title and contextualise lazygit's pushed diff without
  re-running the diff.
- **`clump --pick`** shows clump's own picker (this branch vs default, 50 recent commits, type to
  filter); `clump_pick` falls back to it whenever `rex.ui.pick` cannot show a picker.
- **Rex module configuration**: `CLUMP_BIN` and `CLUMP_NO_BINDINGS` globals set before `dofile`;
  every action also takes `block`/`repo` args (used by `rex do --action` tests, where `ctx` has
  no `block_id`). Default bindings: `ctrl+alt+g` pick, `ctrl+alt+w` watch, `ctrl+alt+q` close.
- **Generated files are skipped, not hidden.** Their hunks stay in the grouping (the `Check`
  invariant still holds) in a folded `generated` group, so they can be read and moved; only the
  model never sees their bodies. The built-in list leaves out files a tool writes but people still
  read for intent (Django migrations, Jest snapshots, `go.mod`). A `generated/` directory pattern
  was rejected: it caught clump's own `internal/generated` package.
- **Reviewed is per hunk content, toggled per file or group.** A file or group is the unit you
  check off, as on GitHub. Storing marks per hunk by content lets them survive regrouping and
  carry over from a commit to the branch range that contains it, and makes them reset when the
  hunk changes. `c` was free (`v`, GitHub's "viewed" key, is visual mode here).
