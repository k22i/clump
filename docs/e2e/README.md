# End-to-end run in Rex (Process step 4)

Run on 2026-11-25 with Rex server 0.1.0-preview.21 / app 0.1.0 (2061), lazygit 0.66.3, git 2.54,
macOS, clump at the commit that added this file. Every capture here is `rex capture` of a real
pane: `*.txt` is text, `*.html` is `--format=html` (keeps colour). `watch.log` is the watch
pane's `--log` output for steps 3–10. `$SCRATCH` stands for the scratch directory used.

## Setup

- **Repo**: a scratch fixture built by `scripts/mkfixture.sh` (51 commits on `main`: rename,
  copy, binary, mode change, unicode and spaced paths, delete, a merge, 40 filler commits; a
  `feature2` branch; one stash entry). Plus the clump repository itself (step 11).
- **lazygit**: `lazygit --use-config-file lazygit.yml` (this directory). Its only renderer is
  `clump push` through a wrapper that points clump at an isolated config, cache
  (`$SCRATCH/e2e/cache-real`) and socket dir, and adds `--log` to `watch`. The user's own lazygit,
  Rex and clump configs were not read or written.
- **Rex module**: `rex/init.lua` loaded by a two-line wrapper that sets `CLUMP_BIN` to the wrapper
  and `CLUMP_NO_BINDINGS`, run with `rex do <wrapper> --action <name> block=<lazygit block>`.
- **Provider**: the built-in `cheap` (claude-haiku via `claude -p`), with `argv[0]` set to an
  absolute path. Reason: inside Rex panes the login shell's `PATH` resolves `claude` to an old
  `/usr/local/bin/claude` install that crashes on Node 26 (the first attempt at steps 3–4 failed
  for that reason; `clump doctor` now prints which binary a command provider runs).

## Steps and evidence

| # | step | result | evidence |
|---|---|---|---|
| 1 | lazygit in a new Rex tab with `clump push` as its renderer | lazygit starts normally (startup probe: push exits at once on a TTY) | `01-lazygit.*` |
| 2 | `clump_watch` from the lazygit pane | split created **to the right**: lazygit `rect [0,0,0.5,1]`, watch `[0.5,0,0.5,1]`; a second `clump_watch` focused the same pane (`created: false`) | `02-layout.json`, `02-watch-opened.*`, `02-lazygit-with-split.*` |
| 3–4 | move the lazygit cursor through 7 commits (`k` ×6 from "Filler commit 35") | the watch pane followed each one: header shows subject, short hash, `→ claude-haiku`; each new commit grouped by Haiku in 2.5–2.9 s; lazygit's re-sends of the same diff were deduped | `03-commit-0..6.*`, `03-lazygit.*`; `watch.log` 17:45:52–17:46:40 |
| 5 | back to two commits already seen | shown **`cached`** ~16 ms after the debounce; `call start` count unchanged (6 → 6) | `05-revisit-cached.*`; `watch.log` 17:47:04–05 (`show … cached`) |
| 6 | hold `j`: 30 presses at 33 ms (`spike/rex-experiments/holdj.lua`) over unseen commits | **31 pushes → 1 model call** (the debounce fires once, on the last selection) | `06-after-hold-j.*`, `06-lazygit-after-hold-j.*`; `watch.log` 17:47:13–19 |
| 7 | stash entry (panel 5) and a range (`v`, `j`, `j` in the commits panel) | both **followed**: stash resolved to its sha and titled `stash@{0}: On main: fixture stash`; range `7cad34e2a144^..2a438fc11199` grouped into 2 groups | `07-stash.*`, `07-range.*`, `07-lazygit-*.*` |
| 7b | the stash's first answer from Haiku had no usable hunk IDs → validator fell back to one group per file (`per-file fallback` in the header); `r` regrouped it cleanly | the repair path worked as designed; `watch` now also logs the raw answer whenever it repairs | `07-stash.*`, `07-stash-regrouped.*` |
| 8 | branches panel (3), then the working tree (2) after editing a file | both **dimmed** (`not followed` in the header, faint text in the HTML, `opacity: 0.5`); `kind=log action=dim`, `kind=worktree action=dim`, including lazygit's periodic worktree re-sends; 0 model calls | `08-branch-dimmed.*`, `08-worktree-dimmed.*`, `08-lazygit-*.*` |
| 9 | in the watch pane: `n` "Reviewed by hand", then `k x M 2` (move group 1's hunk into it); close the pane with `clump_watch_close`, reopen with `clump_watch`, return to the commit | after the restart the commit shows **`edited`** with the hunk in "Reviewed by hand", restored from the override file with no model call | `09-edited.*`, `09-after-restart.*`; `watch.log` 17:49:37 (`show … edited`) |
| 10 | `kill -9` the watch process (leaves a stale socket), keep using lazygit; then reopen and close normally | lazygit kept rendering every diff **in colour** (commit header palette 3, `+` lines palette 2, `@@` palette 6, `diff --git` bold) with no error text; a new `clump_watch` removed the stale socket and started; a normal close removed its socket | `10-lazygit-after-kill.*`, `10-lazygit-after-close.*` |
| 11 | one-shot TUI on a clump commit (`6512b9b`) | 4 groups in reading order (constants/helpers → fallback → Listen check → tests), each explained; `a` shows the all view with group labels | `11-clump-repo-oneshot.*`, `11-clump-repo-all-view.*` |

Also verified in Rex during the build (no captures kept): program status records (`working` with
the estimate, then `done`, both `app=clump`) read back with the terminal's `program_status`
method; the theme query answered `dark` in < 1 ms (`clump doctor`); `clump_pick` with
`rex.ui.pick` → `unsupported` on this app build, so it opened `clump --pick` in a floating layer,
whose filter and Enter opened the chosen commit.

## Cost

Real model calls in this run, all on `cheap` (Claude Haiku 5.5 via `claude -p`, cost as reported
by the CLI): 12 successful watch calls ($0.00446), 1 one-shot call (~$0.00096), and 1 manual
replay of the stash prompt to inspect the model's answer ($0.00035). Seven earlier attempts
failed inside the broken local `claude` install before reaching the API. Total ≈ **$0.0058**.

## Not covered here

- A merge commit selected in lazygit (the first-parent reload is covered by unit tests only).
- Diffing mode (`W`) in lazygit (classification is covered by the INDEX.tsv captures).
- `rex.ui.pick` rendering a picker (the app build returns `unsupported`).
- Unsolicited theme-change notifications; Linux.
