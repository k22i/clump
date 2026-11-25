# Spike A — what lazygit pipes to a diff filter

Environment: lazygit 0.66.3 (Homebrew), git 2.54.0 (Apple Git-157), macOS, inside a Rex pane.
Everything here was observed with `lgcapture` (`lgcapture/main.go`), a stdinFilter that logs
argv, cwd, env, the parent process's command line, timing and signals, saves stdin, and
passes stdin through unchanged. lazygit was driven with `rex send-key` / `rex block call … write`.

Raw evidence:
- `samples/lazygit/*.txt` are captured stdin files, one per kind of selection. `INDEX.tsv` maps each one to the git command lazygit ran.
- `samples/capture-logs/*.jsonl` are the per-invocation logs: `always`/`never` for colour, `hold` for key repeat, `slow` for a 400 ms filter.
- `samples/lazygit-configs/*.yml` are the lazygit configs used, loaded with `lazygit --use-config-file`. Your own config was never touched.

## The hypothesis holds, with one correction

`git.diffRenderers` (`type: stdinFilter`, the default) works as hoped. lazygit puts the
command in `GIT_PAGER` and git runs it **for every diff lazygit renders in the main view, on every
selection change**, with the raw diff on stdin and stdout feeding lazygit's panel.
(The docs: `lazygit-docs/lazygit-Custom_DiffRenderers-v0.66.3.md`.)

The correction is that the filter is spawned **by git**, not by lazygit. Its parent process is the `git show` / `git diff`
command, so the filter can read *exactly what lazygit asked for* from its parent's argv. That
turns out to be the best way to tell what is selected (below).

## What stdin contains, per selection

| lazygit selection | parent git command (from `ps`/sysctl) | stdin starts with | identifiable from stdin alone? |
|---|---|---|---|
| Commit (commits panel) | `git show … --stat --decorate -p <full sha> --` | `commit <full sha> (decorations)`, Author, Date, indented message, `---`, `--stat` block, then the patch | **yes**, via the `commit <sha>` header |
| Range selection (`v` + `j/k` in commits) | `git diff … <oldest>^ <newest> --stat -p --` | the `--stat` block, then the patch, **no header** | no; the parent argv has it |
| File inside a commit (`enter` on a commit) | `git diff … <sha>^ <sha> -- <path>` | the patch for that path only | no; the parent argv has it |
| Stash entry | `git stash show … -p --stat -u refs/stash@{N}` | the `--stat` block, then the patch, **no header** | no; the parent argv has it |
| Branches panel | `git log --graph --color=always … refs/heads/<b> --` | a **graph log, not a diff** | n/a (not a diff) |
| Diffing mode (`W`, "diff main against …") | `git diff … --stat -p <a> <b> --` (**two-dot**, not merge-base) | the stat block, then the patch | no; the parent argv has it |
| Working-tree files panel | `git diff … -- <path or dir>` (`--cached` for staged) | the patch | no; the parent argv has it |
| Remotes / tags tabs | (no call) | n/a | n/a |
| **lazygit startup** | parent is **lazygit itself** | **stdin is a TTY, 0 bytes** | n/a. This is a probe; `push` must not read a TTY |

Notes:
- **The working-tree diff is re-sent periodically** (about every 10 s and on file events) even when
  nothing is selected, and so is the commit diff when you re-enter the panel. `watch` must dedupe by content hash.
- **cwd is the repo root** even when lazygit was started in `src/`. git runs with `-C <root>`, and
  `GIT_PREFIX` is set to empty.
- **The filter inherits lazygit's env, including `REX_SESSION` and `REX_BLOCK` (the lazygit pane).**
  So `push` knows which pane it came from with no lookup.
- With `colorArg: always`, git's `--color` doesn't reach `git log --graph`, which uses `--color=always` anyway.

### Rules proposed for what `watch` follows

`push` sends the parent's argv along with the diff. `watch` classifies:

| kind | rule | default |
|---|---|---|
| `git show <sha>` | follow; identity is the sha | on |
| `git diff A^ B --stat -p` (range) | follow; identity is `A^..B` | on |
| `git stash show refs/stash@{N}` | follow; resolve to the stash commit's sha (`git rev-parse`) in `watch`, not `push` | on |
| `git diff X Y` (diffing mode) | follow as "X vs Y". Two-dot, exactly what lazygit shows. | on |
| `git diff A^ A -- <path>` (file inside a commit) | don't regroup; keep the commit's groups and scroll to that file | (cursor only) |
| `git diff [--cached] -- <path>` (worktree) | ignore; show the last result dimmed | off (opt-in) |
| `git log --graph … refs/heads/<b>` | not a diff; dim the last result. Possible extra: offer "press b to group `<b>` vs `<default>`" | dim |
| stdin is a TTY, empty, or argv unreadable and no `commit` header | ignore | — |

If the parent argv can't be read (another OS, a git wrapper), fall back to the stdin `commit <sha>` header, which
covers commits only.

Reading the parent argv **without CGo** works: `kern.procargs2` through `golang.org/x/sys/unix.SysctlRaw` on
macOS, `/proc/<ppid>/cmdline` on Linux. This is verified end to end in `pushproto/main.go` (the listener
printed `kind=show` / `kind=stash` / `kind=diff`).

## Colour and `{{width}}`

- **Default `colorArg: always`:** stdin carries ANSI **SGR sequences only** (`ESC[…m`). No other CSI appeared in any
  capture. Stripping `ESC\[[0-9;]*m` gives output **byte-identical** to `colorArg: never` for the same commit
  (checked with `diff`).
- **`colorArg: never` is not wanted.** With a plain pass-through, lazygit's own diff panel loses all colour:
  lazygit doesn't re-colour pager output. The VT capture of the panel had no SGR on `commit …` or `diff --git` lines.
  **Recommendation:** keep the default (`always`), strip SGR in `push`, and pass the coloured bytes through untouched.
  (Or pipe to `delta` when configured, as the task says.)
- **`{{width}}`** is only a template variable for the command line, and git gets `COLUMNS=<view width>`. Width affects only
  the `--stat` block (long paths are elided with `...`), which the parser ignores. Patch lines are never wrapped by git;
  lazygit wraps them in its view. `{{colorScheme}}` (`dark`/`light`) is available too, and lazygit asks the terminal for it.

## How often the filter runs, and debouncing

Measured on a 60-commit fixture (`samples/capture-logs/hold.jsonl`):

| input | filter calls |
|---|---|
| `j` held at ~30 Hz (30 presses, 33 ms apart) | **30 calls, one per keypress**, ~34 ms apart, each finished in ~6 ms |
| `j` at 12.5 Hz (80 ms apart) | 30 calls, ~80 ms apart |
| 30 `k`s written in one burst | **1 call** (lazygit handles the batch, then renders once) |

So lazygit **doesn't throttle**. **`watch` must debounce.** 300–400 ms trailing is enough, because key repeat is at most ~30 Hz.

With a deliberately slow filter (400 ms sleep, `samples/capture-logs/slow.jsonl`), lazygit **did not kill** older filter
processes when the selection moved. They ran **concurrently** and all finished, with no signal or write error. Consequences:
- `push` must stay fast. If it stalled, the processes would pile up, though lazygit stays responsive.
- Messages can arrive **out of order**. `push` stamps `sent_ns`, and `watch` drops anything older than the newest it has seen.

## `push` prototype (`pushproto/`)

`pushproto push` reads stdin, writes it to stdout unchanged **first**, then strips SGR, reads the parent argv, and
dials `$TMPDIR/clump/<sha256(root)[:8]>.sock` (50 ms dial timeout, 100 ms write deadline). It sends one JSON line
`{v, sent_ns, repo_root, git_argv, rex_block, diff}` and exits. Measured:

- In lazygit with a listener: push finished **0.3 ms** after process start, and the listener received each message **0.2 ms** after it was sent
  (`samples/capture-logs/pushproto.log`).
- With **no listener**: the stale socket file gives ECONNREFUSED at once, and push exits in **0.15 ms** after start.
- Whole process (Go start + read + exit, no listener): **p50 10.5 ms / p95 14.6 ms**, against **7.3 / 8.4 ms for `cat`**
  in the same harness. That's about 3 ms of overhead (`perl system()` spawn cost is included in both).
- After the listener was killed, lazygit kept rendering and colouring diffs normally.
- The socket was created mode `0600` in a `0700` dir (`listening … sock=-rw------- dir=-rwx------`).

The full end-to-end integration test (the real `watch` TUI beside lazygit, cache hits) is part of the build phase, not the spike.

## Fallbacks if this mechanism had failed (not needed)

1. A lazygit `customCommands` key (e.g. `ctrl+g`) running `clump push --commit {{.SelectedLocalCommit.Hash}}`
   is reliable and explicit, but you have to press a key.
2. Read lazygit's `state.yml` / screen via `rex.block.call(term, "format")`. Screen-scraping is fragile and wasn't built.

The stdinFilter path works, so neither is proposed. Option 1 could still be offered as a "regroup now" key.
