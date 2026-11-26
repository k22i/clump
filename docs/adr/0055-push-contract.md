# `clump push`: the diff renderer contract

`push` is lazygit's `diffRenderers` stdin filter, so it must never fail, never print, never read
config (ADR-0005) and must return the diff before doing anything else. The socket side is
fire-and-forget: no listener, a stale socket or a busy one is not an error (ADR-0036).

## `clump push` (lazygit `diffRenderers` stdinFilter)

1. stdin is a TTY -> exit 0 at once (lazygit's startup probe; A-lazygit.md).
2. Read all stdin. Write it to stdout unchanged **first** — or, with `--then '<cmd>'`, pipe it to
   `sh -c <cmd>` with stdout/stderr inherited (e.g. delta).
3. Strip SGR for the payload. Repo root: walk up from the cwd to `.git` (git runs the pager in
   the repo root). Parent argv via sysctl `kern.procargs2` (darwin) or `/proc/<ppid>/cmdline`.
4. Dial `SocketPath(root)` with a 50 ms timeout, write one JSON line with a 100 ms deadline,
   close. Exit 0 always, silently. No config is read.

Budget: ≤ 5 ms p50 over `cat` without a listener (spike: ~3 ms).
