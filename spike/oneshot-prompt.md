/rex-orchestrate

# Build `clump` end to end: coordinator brief

You are the **coordinator** (Opus). You will build `clump`, a Rex-native semantic diff viewer in Go, using `/rex-orchestrate` workers that run the `vibe` CLI with **DeepSeek v4.1 flash**. Start every worker with `--agent vibe --agent-args "--auto-approve --trust"`. Check `vibe config` once. If vibe's default model isn't DeepSeek v4.1 flash, pass the model with `--model`.

**The workers are much weaker than you.** Plan for that throughout:

- You own architecture, every cross-package contract, the hard parts, all integration, and every review.
- Workers get small, closed, mechanical tasks. Each one comes with exact signatures, test tables, the files it may touch, and the command that proves it is done.
- Never accept a worker's work without reading the diff yourself.

Work autonomously until the app is finished and verified. Ask me only when a decision is genuinely mine and can't be settled from the material below.

## 0. Read first (yourself, not the workers)

1. **The task spec:** `/Users/kevinmallari/Developer/DESIGN.md`. It is named DESIGN.md but it is the *requirements*. Every "Hard requirement", the "Watch mode mechanism", "Engineering constraints" and "Process" steps 2–5 apply. The spike (step 1) is done.
2. **The spike results:** `/Users/kevinmallari/Developer/clump/spike/README.md`, `A-lazygit.md`, `B-grouping.md` and `C-rex.md`. These record verified facts with evidence. Don't re-derive them, and don't contradict them without a new experiment.
3. **Reusable spike code** (port it, don't import it):
   - `spike/groupcli/`: parser, prompt v1, validator, providers and tests.
   - `spike/pushproto/`: push/listen IPC and parent argv through sysctl.
   - `spike/samples/lazygit/`: real lazygit stdin captures. `INDEX.tsv` maps each file to the git command that produced it.
   - `spike/rex-experiments/`: verified Rex Lua call shapes.

## 1. Decisions already made (don't reopen them)

- **Repo and module.**
  - The repo is `/Users/kevinmallari/Developer/clump` (it contains only `spike/` now). Run `git init` on branch `main`.
  - Module path: `github.com/k22i/clump`.
  - Use `go 1.25` in go.mod. Go 1.27.1 is installed.
  - Keep `spike/` in the repo, but gitignore `spike/bin/`.
  - Single static binary: `CGO_ENABLED=0`. `go install github.com/k22i/clump/cmd/clump@latest` must work.
- **Dependencies, the whole list:**
  - Bubble Tea, Bubbles and Lip Gloss. Pick the current stable major and pin it.
  - `github.com/alecthomas/chroma/v2`.
  - `github.com/BurntSushi/toml`.
  - `golang.org/x/sys`.
  - Nothing else without a written reason in DESIGN.md. Talk to the Anthropic Messages API over raw `net/http`, so there is no SDK dependency.
- **Providers.**
  - **Defaults:**
    - `cheap` is a command provider. Its argv is:
      `claude -p --model haiku --output-format json --tools "" --setting-sources "" --strict-mcp-config --exclude-dynamic-system-prompt-sections --no-session-persistence --system-prompt "You are a JSON API. Follow the user message exactly and output only JSON."`
      - The prompt goes on stdin.
      - The working directory is `os.TempDir()`, so the CLI never picks up repo context.
      - The output format is `claude-json`.
    - `strong` is the same command with `--model sonnet`.
  - **Also implement:**
    - an OpenAI-compatible HTTP provider (`base_url`, `model`, `api_key_env`; `response_format: json_object`; temperature 0);
    - a direct Anthropic Messages provider (`api_key_env`, default `ANTHROPIC_API_KEY`). It is about half the cost of `claude -p`, because the CLI bills a 1-hour cache write on every call;
    - the command output formats `raw`, `claude-json` and `opencode-json`;
    - a fake provider for tests.
  - **Pricing and token estimates are per provider in config.** Defaults:
    - prices: Haiku 5.5 $0.10/$0.50 per MTok, Sonnet 5.5 $2/$10;
    - `bytes_per_token`: 2.0 for Claude and 3.3 otherwise. The spike measured ~2.1 for Claude and ~3.3 for DeepSeek, and bytes/4 under-counts Claude by about 2×;
    - refine `bytes_per_token` from the usage each provider reports, and store it next to the cache.
  - **Keys are never in config.** Read them from the env var the config names, and redact them from all error text.
- **Diff input.**
  - `clump <rev>` uses `git show --format= --no-color --no-ext-diff --find-renames <rev>` plus `git log -1` for the message.
  - `clump A...B` uses a merge-base diff, and adds `git log A..B` messages (capped) as context.
  - `clump --branch` compares the current branch with the default base: `origin/HEAD`, else `main`, else `master`.
  - A patch on stdin.
  - **Merge commits:** diff against the first parent (`--first-parent` / `-m`). Never feed `diff --cc` to the parser.
- **Grouping contract.**
  - Hunk IDs are `h1…hN` in file order. Binary, pure-rename and mode-only files get one pseudo-hunk.
  - Catalog line: `id | status | path | @@ header | +a -d`.
  - Hunk blocks are annotated `### hN path`.
  - All data goes between `<<<CLUMP-DATA-{nonce}>>>` markers, with a fresh random nonce per request.
  - Model output is `{"groups":[{"title","description","hunks":[ids]}]}`.
  - The prompt lives in `internal/group/prompt_v1.txt` (start from `spike/groupcli/prompt_v1.txt`). `PromptVersion` is part of the cache key.
- **Validation and repair** follow the spike's `Validate`:
  - drop unknown IDs;
  - on duplicates keep the first;
  - merge groups over the cap into "Other changes";
  - put missing hunks in "Ungrouped";
  - with no usable JSON, fall back to one group per file, with a notice.
  - Invariant: every hunk is in exactly one group.
- **Large diffs.**
  - Never truncate the catalog or the headers.
  - Tier 1: binary-search the per-hunk body cap that fits the budget, and mark each cut with `[... N more lines]`.
  - Tier 2: when the cap would fall below 4 lines, or the catalog alone exceeds the budget:
    1. per-file summary calls, chunked to fit the budget and cached per file-patch hash;
    2. one grouping call over the summaries and the catalog.
  - Collapse runs of near-identical hunks (same path pattern, same shape; e.g. 45 translation files) into one catalog unit that the model assigns as a whole.
  - Show the token and cost estimate before sending, and ask when it is above the configurable `ceiling`.
- **Watch: selection rules.** `push` sends the parent git argv. Classify it like this (table A in `spike/A-lazygit.md`):
  - **Follow:** `git show <sha>` (a commit), `git diff A^ B --stat -p` (a range), `git stash show refs/stash@{N}` (resolve it to a sha in `watch`), and diffing-mode `git diff X Y`. Diffing mode is a two-dot diff, exactly what lazygit shows.
  - **File inside a commit:** don't regroup; only move the cursor to that file.
  - **Ignore:** the worktree diff (`git diff [--cached] -- path`) unless `follow_worktree = true`.
  - **Dim the last result:** `git log --graph` (the branches panel) and anything unknown.
  - **Fallback** when argv can't be read: the stdin `commit <sha>` header.
- **Watch: concurrency.**
  - Trailing debounce of 400 ms.
  - Dedupe by diff hash: lazygit re-sends the same diff about every 10 s.
  - Drop messages whose `sent_ns` is older than the newest seen.
  - At most one model call in flight and one queued (latest wins). When the selection moves, an in-flight call is **not** cancelled: it finishes, its result is written to the cache, and it is discarded from the display as stale.
  - `q` cancels everything.
- **Watch: display.**
  - Auto-group is on by default with `cheap`, and `A` toggles it.
  - Never auto-send above the ceiling; show `press g to group (~N tokens, ~$X)` instead.
  - Header line: commit subject, short hash, the provider receiving diffs, and a `cached` / `edited` marker.
- **`push`.**
  - If stdin is a TTY, exit 0 at once (lazygit's startup probe). Otherwise read all of stdin.
  - Write the bytes to stdout unchanged **first**, or pipe them to `--then '<renderer cmd>'` when one is configured, e.g. delta.
  - Then strip SGR (`ESC\[[0-9;]*m`) for the payload.
  - Repo root: walk up from the cwd to `.git`.
  - Dial `$XDG_RUNTIME_DIR` (else `$TMPDIR`) `/clump/<sha256(root)[:16]>.sock`, with a 50 ms dial timeout and a 100 ms write deadline.
  - Send one JSON line: `{v, sent_ns, repo_root, git_argv, rex_block, diff}`.
  - Exit 0 always and silently.
  - Keep lazygit's `colorArg` at its default (`always`). `never` makes lazygit's own panel lose its colour.
- **Socket.** Mode 0600 in a 0700 directory. `watch` removes a stale socket file before it listens, and refuses to start if another live `watch` already owns that socket.
- **Rex.** These are verified; see `spike/C-rex.md`.
  - **Watch pane:**
    - Create it with `rex.session.new_split{anchor_id=<lazygit block>, direction="horizontal", side="after", focus=false, layout=rex.layout.block{flavor="com.superlogical.terminal.shell", label="clump-watch:"..root, options={cwd=root, command={"clump","watch","--repo",root}}}}`.
    - Find it again with `list_blocks`, by label. The label survives program title changes.
    - `focus_block` focuses it; `block.close` closes it.
  - **Working directory:** `pwd` returns a `file://host/path` URL. It is empty until the shell reports OSC 7, so fall back to `process().foreground.cwd`.
  - **`rex.ui.pick`** returned `unsupported` with my current app build. `clump.pick` must try it first, then on `unsupported`/`no_client`/`busy`/`timeout`/`failed` open `clump --pick` in a `new_layer`. That needs an in-TUI picker for "this branch vs default" and recent commits.
  - **Program status:** OSC 7501 with `app=clump` on every report:
    - `working` while grouping;
    - `blocked kind=question` when over the ceiling;
    - `done` with the group count;
    - `error` with a redacted message.
    - Only emit it when `REX_SESSION`/`REX_BLOCK` are set.
  - **Theme:** query `CSI ? 996 n` with DA1 as a sentinel and a ~100 ms timeout, before the TUI starts. Rex answers `ESC[?997;1n` for dark. `CLUMP_THEME=light|dark` overrides it.
- **Don't modify my real configs.**
  - `clump rex install` and `clump lazygit install` print the snippet by default, and write only with `--write` plus a y/N confirmation, after making a backup.
  - For testing, use `lazygit --use-config-file <tmp.yml>` and `rex do rex/init.lua --action <name>`. Never touch `~/.config/rex/init.lua` or lazygit's `config.yml`.

## 2. How to use the workers

**Concurrency.**

- At most 4 workers at once.
- Use `--worktree current`, with strictly disjoint file ownership: one package or directory per worker.
- Freeze the shared types before you fan out, so each worker can build and test its package alone.

**Workers run checks.** I am explicitly asking for this, which overrides the rexo default. Every worker spec must end with:
```
gofmt -l . (must print nothing for your files)
go vet ./internal/<pkg>/...
go test -race ./internal/<pkg>/...
```
Workers paste the full output into their `done` report.

**Every worker spec must be self-contained.** The worker never sees this conversation. Include:

1. **Goal.** One paragraph.
2. **Files.** The files it **owns**. Everything else is read-only. List the read-only files it needs to look at.
3. **API.** The exact exported API as a Go code block, with types, signatures and doc comments. The worker must not change it. If it thinks the API is wrong, it says so with `rexo ask` instead.
4. **Behaviour.** Numbered rules, including edge cases and error messages.
5. **Tests.** A table of the test cases it must write, with inputs and expected outputs. Point it at fixture paths.
6. **Ported code.** If it ports spike code, the source path, and what to change.
7. **Forbidden.**
   - new dependencies;
   - editing files outside its ownership;
   - `t.Skip`;
   - tests that assert nothing;
   - weakening or deleting existing tests;
   - placeholder or TODO implementations;
   - network calls in tests;
   - calling real LLM providers.
8. **Done.** The commands above all pass, plus a short report of what was done and anything left uncertain.

**Size.** Keep each task to roughly ≤ 400 changed lines and ≤ 3 hours of human work. Split anything bigger.

**Review every `worker_done` before accepting it.**

1. Read `git diff` for that worker's files.
2. Re-run its checks yourself, then `go build ./... && go vet ./...` for the whole module.
3. Look for:
   - edits outside its ownership;
   - changed frozen signatures;
   - fake or tautological tests;
   - swallowed errors;
   - races;
   - goroutine leaks;
   - logged secrets.
4. If anything fails, send the precise failure back with `rexo send`. Don't fix it yourself. After 2 failed rounds on the same task, take it over or split it smaller.
5. Commit each accepted task as one local commit with a plain message. **No AI attribution of any kind**: no `Co-Authored-By`, no "Generated with". Never push.

**Hard parts the workers don't get.** Do these yourself:

- the shared types and contracts;
- the root Bubble Tea model, its message flow and the keymap;
- the watch state machine;
- the `rex/init.lua` module;
- prompt v1 and its wording;
- the edit/undo data model design;
- integration and end-to-end verification;
- DESIGN.md.

Workers may implement leaf pieces under these, once you have specified them exactly.

## 3. Phases (gate: the whole module builds and all tests pass before moving on)

### Phase 0: foundation (you alone, no workers)

- `git init`, `go.mod` and `.gitignore`.
- Write `clump/DESIGN.md` (task spec, process step 2). It covers:
  - the decisions above;
  - the large-diff strategy;
  - the verified lazygit and Rex facts, citing the spike;
  - the edit and override data model;
  - the cache layout and keys;
  - the IPC protocol;
  - the watch state machine, with a state diagram in text;
  - the package graph.
- Create every package with **frozen exported types and interfaces**, stub bodies that compile, and doc comments:
  - `internal/gitdiff`: `Diff`, `File`, `Hunk`, `Commit`; `Parse`; git loaders for rev, range and branch.
  - `internal/catalog`.
  - `internal/provider`: the `Provider` interface with `Complete(ctx, prompt) (text string, Usage, error)`; `Usage`; the registry and config types.
  - `internal/group`: `Group`, `Result`, `Validate`, `Check`, `Build`; the edit model.
  - `internal/cache`.
  - `internal/ipc`.
  - `internal/tui`.
  - `internal/rex`.
  - `internal/lazygit`.
  - `internal/config`.
  - `cmd/clump`.
- Copy the fixtures into `internal/gitdiff/testdata/`:
  - `spike/samples/lazygit/*`, with `INDEX.tsv`;
  - the edge cases from the spike's `parse_test.go`;
  - a few real `git show` outputs from this repo, for golden tests.
- **The edit data model.** Pin this down precisely, because workers will implement it:
  - `Grouping{Groups []Group}`, where `Group{ID string /*stable, e.g. g1*/, Title, Description string, Hunks []string}`.
  - Pure functions that return a new `Grouping` or an error, and never mutate:
    - `Move(g, hunkIDs, toGroupID | NewGroup)`
    - `NewGroup(g, title)`
    - `Rename(g, id, title, desc)`
    - `Delete(g, id)`: its hunks go to "Ungrouped", which is created if needed.
    - `Reorder(g, id, delta)`
    - `Merge(g, srcID, dstID)`
  - Every op returns `Check(diff, result)`-clean output, or an error and an unchanged input.
  - Empty groups are allowed except "Ungrouped", which disappears when empty.
  - `History{past, future []Grouping}` gives unlimited snapshot-based undo and redo.
  - Overrides file, keyed by diff hash:
    ```
    {version, diff_hash, base_cache_key, edited_at, grouping, snapshots: [{name, at, grouping}]}
    ```
  - Overrides are stored separately from model cache entries.
- Commit.

### Phase 1: core packages (workers, in parallel; you review)

Suggested split, one worker each:

- **W-gitdiff.** Port and harden the parser. It must handle:
  - renames and copies;
  - binary files and `GIT binary patch`;
  - mode changes;
  - new and deleted files;
  - `\ No newline at end of file`;
  - quoted unicode paths;
  - paths with spaces;
  - `git show` headers, with and without lazygit's `--stat`;
  - SGR stripping;
  - first-parent merges.

  Write table tests over every fixture. Add a test that parses each `testdata/` file and re-checks that the hunk line counts match the `@@` header counts.
  It also owns the git loaders, which shell out to `git` with `-c core.quotepath=off`.
- **W-provider.** The command, OpenAI-compatible, Anthropic Messages and fake providers, plus redaction. Tests use `httptest` fakes and `cat`/`printf` scripts. Never a real network call.
- **W-cache.**
  - `$XDG_CACHE_HOME/clump` (default `~/.cache/clump`).
  - Key: `sha256(normalized diff + provider name + model + PromptVersion + max_groups)`.
  - Atomic write: temp file, then rename.
  - Model results and overrides go in separate directories.
  - Store the `bytes_per_token` calibration.
  - A hit is a pure file read. Test that a hit does no I/O beyond reading that file.
  - Honour `--no-cache`.
- **W-group-validate.** Port `Validate`/`Check`/`FallbackPerFile`. Write the repair table tests and a property test of at least 2000 random model outputs.
- **W-group-edit.** Implement the edit ops and `History`, exactly as you specified them in Phase 0.
  Property test: a random sequence of at least 5000 ops, undos and redos, checking `Check` after every step, with the seed logged.
  This task is subtle. Give the worker the full invariant list and an example sequence.
- **W-config.**
  - TOML at `~/.config/clump/config.toml`: named providers, `default`, `cheap`/`strong`, `ceiling`, `budget`, `max_groups`, `auto_group`, `follow_worktree`, prices, `bytes_per_token`, `then` renderer.
  - Ship a commented default config with `clump config init` (prints it).
  - Validation errors name the key at fault.

You write `internal/group` `Build`, covering truncation, the summaries tier and catalog collapsing. You can also give it to a worker, but only with a complete specification and test table. Port it from `spike/groupcli/prompt.go`.

**Gate:** you run `clump --dry-run` and `clump --plain` on a commit in this repo using the **fake** provider, plus **one** real `cheap` call on a small diff to prove the wiring.

### Phase 2: TUI with the raw toggle (you own the architecture; workers build pure components)

- **You:**
  - the root model and the focus states (group list / diff pane);
  - the async message flow: the UI opens immediately with a spinner, the raw file list is browsable while groups load, then the groups fill in;
  - the keymap from the task spec;
  - the wide (side-by-side) and narrow (inline) layouts;
  - the notices bar.
- **Workers** build pure render functions and helpers, with golden tests. Render to a string with colour off and compare against `testdata/*.golden`.
  - Group list rows: title, `+a −d`, hunk count, `edited`/`cached` markers.
  - Diff pane: hunks grouped by file, Chroma highlighting by file extension, word-level highlighting for paired −/+ lines (only if cheap, else skip and note it).
  - Raw "all" view: the whole diff in file order, each hunk labelled with its group, with group boundary markers.
  - Search: `/`, `n`/`N`.
  - Help overlay: `?`.
  - Markdown summary for `y`, copied to the clipboard through OSC 52 with a `pbcopy`/`wl-copy`/`xclip` fallback. The same text backs `--plain`.
  - `e`: open `$EDITOR` at file:line for the hunk under the cursor (`+N file` style). Suspend the TUI while it runs.
- **Then you:**
  - `r` regroup (bypasses the cache; confirm when there are edits; offer to keep the edits as a snapshot);
  - `m` provider switcher;
  - `--json` (groups, including edits).

### Phase 3: group editing

**You** wire the edit keys to the Phase 1 ops through `History`:

- `x` marks a hunk; visual mode marks a range; `M` moves the marks (picker or new group);
- `n` new group; `R` rename plus explanation, edited inline;
- `d` delete; `J`/`K` reorder; `g` merge;
- `u` undo, `ctrl+r` redo.

Overrides are saved on every edit and loaded on open, marked "edited". `--json` and `y` reflect the edits.

**Tests:** scripted Bubble Tea `Update` sequences that send key messages and assert on the model state. No real terminal.

### Phase 4: IPC, `push`, `watch`

- **W-ipc:**
  - the protocol encode/decode;
  - the socket path;
  - the listener, with 0600/0700 permissions and stale-socket handling;
  - the client, with timeouts;
  - parent argv: darwin `kern.procargs2` and linux `/proc`.

  Tests run a real listener in a temp dir.
- **W-lazygit:**
  - the argv classifier, with table tests built from `spike/samples/lazygit/INDEX.tsv` plus synthetic cases for `--cached`, `stash@{3}` and unknown commands;
  - `clump lazygit install`. It keeps an existing renderer by turning it into `clump push --then '<old command>'`.
- **You:**
  - the `watch` state machine and its integration with the TUI (all TUI keys work in watch);
  - `push` (port it from `pushproto`).
- **Benchmark:** `push` must add ≤ 5 ms p50 over `cat` with no listener running. Write a test or benchmark that pipes a fixture through the real binary.

### Phase 5: Rex

- **You** write `rex/init.lua`: the `clump_pick`, `clump_watch` and `clump_watch_close` actions, plus documented default bindings.
  - Rex custom action names can't contain dots. Use underscores, and say in the README that these are the task's `clump.pick` etc.
  - Re-run `rex api describe` for every method you use.
  - Test each action with `rex do rex/init.lua --action clump_watch` against a scratch repo.
- **A worker writes `internal/rex`:**
  - the OSC 7501 encoder: base64 msg, the spec's limits, no control characters;
  - the env gating;
  - the theme query parser, with table tests on reply bytes.

  You wire it into the TUI and `watch`.
- **A worker writes `clump rex install`:**
  - it prints the `dofile` line;
  - `--write` appends it with confirmation and a backup.

### Phase 6: real integration test in Rex (you; DESIGN Process step 4)

Use a scratch fixture repo (the spike used a 64-commit one: commits, rename, binary, mode change, stash, feature branch) plus the clump repo itself. **Not** my other private repos.

1. Open lazygit in a new Rex tab with a temp config whose `diffRenderers` is `clump push`.
2. Run `clump_watch` (through `rex do --action`) so the watch split opens to its right.
3. Move the lazygit cursor through at least 6 commits with `rex send-key`.
4. Check that the watch pane follows them.
5. Go back to a commit you've seen and check it shows `cached` instantly, with no network call (assert from logs or a counter).
6. Hold `j` (the Lua write loop at 33 ms, from `spike/rex-experiments/holdj.lua`) and check that the debounce gives ≤ 2 model calls.
7. Select a stash entry and a range, and check both are followed.
8. Select a branch and the worktree, and check the result is dimmed.
9. Make an edit in the watch pane, restart it, and check the edit survives.
10. Kill the watch pane and check lazygit's own diff panel still renders with colour, with no errors.

**Evidence:**

- Use `rex capture` (text, plus `--format=html` for colour) at every step.
- Save it under `docs/e2e/` with a short `docs/e2e/README.md` that pairs each step with its evidence.
- Keep real model calls few, all on `cheap`, and report their total cost.

### Phase 7: docs and wrap-up

- **README** (a worker drafts it, you edit it):
  - install;
  - config, with provider examples for Claude CLI, DeepSeek, OpenRouter and Ollama;
  - **privacy**: the diff goes to the configured provider, and in watch mode the header shows which one;
  - every key;
  - Rex and lazygit setup;
  - troubleshooting.
- **Final gate:** `go vet ./...`, `go test -race ./...`, `CGO_ENABLED=0 go build ./cmd/clump`, and `go install ./cmd/clump`, all green.
- **Final report to me, in this order:**
  1. what was built;
  2. the commit list;
  3. the tests and e2e steps actually run, with their results;
  4. what didn't get done or deviated from the spec, and why;
  5. what is still unverified (for example `rex.ui.pick` on a newer app build, Linux, theme-change notifications);
  6. worker stats: tasks, rejections, and tasks you took over.

## 4. Standing rules

- **Cost.** Tests never call real providers. Only Phase 1's gate and Phase 6 make real `cheap` calls. Note each call and its reported cost.
- **Contract changes.** If you change a frozen contract after Phase 0, update DESIGN.md and tell any affected live worker with `rexo send`.
- **Scope.** Stay inside the task spec's scope. Its "Out of scope" list stands.
- **Asking me.** Only for decisions that are genuinely mine. Otherwise choose the conservative default, record it in DESIGN.md under "Decisions made during build", and keep going.
