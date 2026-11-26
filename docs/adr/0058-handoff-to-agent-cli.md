# Handing review comments off to a coding agent CLI

`clump handoff` (and `H` in the TUI) sends the current diff's review comments to a coding agent CLI —
`claude`, `codex`, `opencode`, or any `[agents.<name>]` entry — as the agent's initial prompt, and
clump then gets out of the way. clump never owns the agent process: no headless `-p` mode, no output
capture, no waiting, no "run and report back".

## The decisions

**Command name: `handoff`.** `push` is taken by the lazygit renderer (ADR-0055), `submit` posts to
GitHub, `send` collides with the notion of sending a prompt to a *provider*; `handoff` names the
act and shares no noun with either. TUI key: `H`, free, clear of ADR-0010's shared keys (`n`/`N`,
`g`) — it enters `HelpSections`, the footer hints and docs/keys.md in the same change, as
ADR-0054 requires.

**Two launch shapes plus a print fallback, all fed the same prompt string:**

- `replace` — clump lets Bubble Tea restore the terminal (`tea.Quit`), then the *cmd* layer calls
  `syscall.Exec` with the agent's argv, so clump's process image becomes the agent in the same
  terminal and there is nothing left to supervise. The exec is deliberately after the TUI exits:
  Bubble Tea owns the terminal until `p.Run()` returns, and restoring it by hand would race the
  renderer. `syscall.Exec` (not `golang.org/x/sys/unix`) is enough — releases are darwin and linux
  only (ADR-0046). A model needs a TTY for its UI, so `replace` refuses a stdin that is not a
  terminal (a piped `git show | clump handoff` would hand the agent a dead pipe); tests stub the
  call and assert the argv instead of execing (ADR-0057).
- `pane` — a new split in the multiplexer clump is already inside: `rex split -b $REX_BLOCK
  --cwd <repo> --label "handoff: <agent>" -- <agent argv>` (the exact `rex.session.new_split` shape
  the Rex module's `clump_watch` uses, rex/init.lua), else `tmux split-window -h -c <repo>` when
  `$TMUX` is set. The launcher is invoked and its exit code reported; clump ran-time never attaches
  to the agent's pane. Neither multiplexer present is an error, not a silent no-op.
- `print` — the prompt on stdout, nothing launched. This is how any agent we do not know about
  works (configure its argv, then pipe the prompt wherever), and it keeps handoff scriptable.

The prompt travels in **argv, not stdin**: the agent inherits the TTY and its stdin is the agent's
UI. Verified prompt forms — `claude "<prompt>"`, `codex "<prompt>"`,
`opencode --prompt "<prompt>"` — all reduce to "the prompt is the last argv element", so an agent
entry is just `argv` (the base argv; the prompt is appended) and `where`. A few KB of prompt is far
inside ARG_MAX.

**Config: `[agents.<name>]`, deliberately separate from `[providers.<name>]`.** The isolation rules
are opposites: a provider runs with the prompt on stdin in `os.TempDir()` so it cannot see the repo
(ADR-0027), while a handoff agent must run in the repository, inheriting environment and login.
Reusing provider config would half-silently give the agent provider flags (`-p`, `--tools ""`,
`--setting-sources ""`) that suppress exactly the context a coding agent needs. Entries: `argv`
(required, the prompt appended as its last element), `where` (`replace` by default, else `pane`,
`print`). The top-level `handoff = "<name>"` picks the default agent; empty (the default) picks the first
builtin (`claude`, `codex`, `opencode`) whose argv[0] is on PATH. Validation rejects an empty argv,
an unknown `where`, a `handoff` name that is not in `[agents]`, and unknown keys — so provider
secrets rules keep applying unchanged to the providers table.

**Scope: the whole review.** A handoff carries every comment anchored to the current diff and its
grouping — never just the group or file under the cursor. An agent needs the full review to act
coherently, partial handoffs would make the "these went out" stamp ambiguous, and the picker is
already the scope selector: comments the user has not written are not in the store at all.

**Stamping: the comments that went out get a `handed_off` timestamp on the comment record**
(ADR-0017's store; `omitempty`, no version bump — shards written before this change read back with
a zero stamp and shards written after it are ignored by older binaries without harm). The stamp is
written *before* a `replace` exec (syscall.Exec never returns, so writing after is writing never),
after the pane launcher exits 0 (the agent is out of clump's hands; the stamp means "delivered",
not "resolved"), and after the prompt is written in `print` mode. Consequences:

- `clump comments --open` lists only comments not yet handed off; `clump comments --json` carries
  `handed_off`; the plain list marks them `(handed off)`.
- There is no `--clear` and no re-open: a handoff is a fact, not a state machine. Handing off again
  re-sends all comments and re-stamps the ones that were open — the prompt always carries the whole
  review, so an agent never has to guess what earlier handoffs covered.
- The stamp never changes what posts to GitHub (`submit` sends comments, stamped or not) and never
  changes what the TUI shows — rendering is untouched (ADR-0043).

**Default `where` is `replace`** — the purest "get out of the way": any terminal, no multiplexer,
nothing left running. In **watch mode the TUI maps `replace` to `pane` and hides the `print`
item**: replacing would kill the pane lazygit pushes to (the IPC socket dies with it), and quitting
watch to print a prompt tears down the session for a scroll of text. The CLI honours `--where`
literally; it is expected to run where the user can see the consequence.

**Fail loud, degrade to print.** The binary is resolved with `LookPath` before anything is
launched; if the exec cannot start — missing binary, no multiplexer, launcher error — clump prints
the prompt *and* the exact command ("$ /path/to/agent 'prompt…'") on stdout and exits non-zero
instead of dying quietly. `clump doctor` lists every configured agent, its `where`, the resolved
path and a `--version` smoke result (3 s timeout) — a `claude` that exits 1 with no output under a
broken Node is reported as "this agent may be broken", because a handoff that silently does nothing
is worse than one that says the binary is broken.

**New package `internal/handoff`** (prompt, argv, pane wrapping, shell quoting, the request that
the TUI returns to cmd/clump), imported by `cmd/clump`, `tui` and `config` and importing only
`comment` — the edges ADR-0045's graph now lists. The agent launches live in `cmd/clump` (exec) and
`tui/sys` (the pane side effect), not in render packages, which stay pure (ADR-0007).

## Prompt

The prompt is the numbered comments, each with the target an agent needs — `path:line (side)`,
`path (file)`, `group "<title>" (clump group <id>)` — under a one-line header naming the ref
(the resolved sha) and the subject, and a two-line legend of what line/file/group comments are. It
does not embed the diff or the grouping: the agent runs in the repo and can ask clump itself
(`clump --json`, `clump comments --json`) or git. This is the contract `TestPrompt*` in
internal/handoff pins.
