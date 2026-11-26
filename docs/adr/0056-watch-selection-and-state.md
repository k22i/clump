# Watch: selection rules and the state machine

Watch classifies what lazygit is showing from the git command that ran `push` (its parent argv),
falling back to the commit header of the diff text when the argv cannot be read. The classification
decides whether a selection is followed, only moves the cursor, or is dimmed — the table below is
the contract `lazygit.Classify` and `Decide` implement, pinned by tests over argv lines captured
from lazygit 0.66.

The state machine is single-threaded by construction: one call in flight, one queued, stale results
discarded (ADR-0035).

## Selection rules (`lazygit.Classify` + `Decide`)

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

## Watch state machine

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

- Out-of-order messages: drop `sent_ns` older than the newest seen (ADR-0035).
- Dedupe by diff hash: lazygit re-sends the same diff every ~10 s and on re-entering a panel.
- In-flight calls are not cancelled on selection change: they finish and fill the cache, so
  with Haiku at ~5 s the waste is bounded to one call (ADR-0035).
- All TUI keys work in watch, including editing (overrides keyed by diff hash).
- `--log FILE` writes one line per event (msg, classify, cache hit, call start/end, stale
  discard) for the e2e evidence.
