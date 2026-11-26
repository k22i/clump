# Watch mode with lazygit

Watch mode turns lazygit's diff panel into the trigger: whatever diff lazygit renders is also sent to
a clump pane, which groups it and follows as you move the cursor.

## How it works

lazygit runs a `git.diffRenderers` filter for every diff it renders; `clump push` is that filter:

1. If stdin is a TTY — lazygit's startup probe — it exits at once.
2. It writes the diff to stdout unchanged (or pipes it to `--then '<cmd>'`, e.g. delta), so lazygit's
   panel keeps working and keeps its colour.
3. It strips the ANSI sequences from the copy it forwards, finds the repo root, reads the parent git
   command line, and sends one JSON line to `clump watch` over a private Unix socket for this
   repository, then exits 0 — silently, so with no watch running nothing happens.

`push` reads no config and never blocks the panel: it adds 4.7 ms p50 over `cat` (measured by
`TestPushLatency`), and with no watch running the dial fails at once.

## Setup

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

## What is followed

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
