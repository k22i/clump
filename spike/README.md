# clump spike: results

This is the spike from `../../DESIGN.md` (process step 1). Nothing outside `clump/spike/` was changed. Your lazygit and
Rex configs are untouched; every experiment used `--use-config-file` configs, scratch repos and temporary Rex tabs,
and those tabs are all closed again.

| part | report | one-line answer |
|---|---|---|
| A: lazygit filter | [A-lazygit.md](A-lazygit.md) | **The stdinFilter mechanism works.** Selections are identified precisely from the parent git argv. Keep `colorArg: always` and strip SGR. Debounce in `watch`. `push` costs about 3 ms. |
| B: grouping CLI | [B-grouping.md](B-grouping.md) | Haiku 5.5 groups well for about $0.001–0.013 per diff in 4–10 s. Sonnet 5.5 is best on big branches. The validator repaired a real dropped-hunk failure. bytes/4 under-estimates Claude tokens by about 2×. |
| C: Rex APIs | [C-rex.md](C-rex.md) | Split with a command at creation, floating layer, label lookup, pwd, OSC 7501 and `CSI ?996n` all verified. **`rex.ui.pick` returned `unsupported` with your app build.** |

## Directory map

```
A-lazygit.md B-grouping.md C-rex.md   reports
lgcapture/      capture filter (logs stdin/argv/env/parent cmd/timing; passes stdin through)
pushproto/      prototype of `clump push` + a minimal listener (pure Go, CGO_ENABLED=0)
groupcli/       throwaway grouping CLI: parser, catalog/prompt v1, providers, validator, tests
results/        Markdown output, logs and stats.jsonl for every model run; run.sh re-runs the matrix
samples/        captured lazygit stdin (lazygit/), per-call logs (capture-logs/), lazygit configs used
rex-experiments/  Lua and Python experiments and their *.result.json
rex-docs/       snapshot of the Rex docs and `rex api describe` output used
lazygit-docs/   lazygit 0.66.3 Config.md, Custom_DiffRenderers.md, and the default config
bin/            built spike binaries (lgcapture, pushproto, groupcli); delete freely
```

Rebuild: `cd groupcli && go test ./... && go build -o ../bin/groupcli .`, and the same for `lgcapture` and `pushproto`.
Built with Go 1.27.1. The modules declare `go 1.25`.

## Decisions I need from you before the full build

1. **Selection rules in `watch`** (A, table "Rules proposed"). Follow commits, range selections, stash entries and diffing-mode
   comparisons. Don't regroup on "file inside a commit" (only move the cursor). Ignore the worktree unless opted in. Dim on the
   branches panel. Optional extra: when a branch is selected, offer "b: group `<branch>` vs `<default>`".
2. **In-flight calls when the selection moves.** The task asks for both "cancel in-flight calls" and "stale responses are still
   cached". My proposal: let a request that has already been sent finish and write to the cache, but keep at most one in flight and one
   queued (latest wins), behind a 400 ms debounce. With Haiku at ~5 s, that bounds the waste to one call. The alternative is a hard cancel (context and
   process-group kill), which saves tokens but caches nothing.
3. **Default providers.** `cheap` = `claude -p --model haiku` (command provider, uses your existing login, flags as in B).
   `strong` = Sonnet. Also ship the direct Anthropic Messages provider, which costs about half because the `claude` CLI bills a 1 h cache write on every call.
   DeepSeek or OpenRouter go through the OpenAI-compatible provider once you set a key env var. Through opencode they failed or were slow.
4. **`clump.pick` fallback.** Because `rex.ui.pick` is `unsupported` here, the action falls back to a picker inside clump's TUI
   in a floating layer. If you'd rather update the Rex app and retest first, say so.

## Not verified yet (with reasons)

- `rex.ui.pick` rendering a picker, and `ctx.block_id` when an action is run from a key or the palette. Both need either a
  newer app build or edits to your `~/.config/rex/init.lua`, which I didn't make.
- Unsolicited theme-change notifications (`CSI ? 2031 h`). That needs a manual light/dark toggle.
- The OpenAI-compatible HTTP provider against a real endpoint. No API key in the environment; it's tested only against a local fake.
- The two-pass "per-file summaries" large-diff tier. It's designed from the measurements in B, not built.
- Merge commits: the spike parser rejects `diff --cc`. lazygit's commit view of a merge needs a rule (probably
  `git show --first-parent` diff from `watch`, or "not followed").
- Linux `/proc/<ppid>/cmdline` for the parent argv. The code path exists, but only macOS was run.
- The real end-to-end test (the `watch` TUI beside lazygit following and caching, with `rex capture` evidence) is in
  process step 4, after the build. The spike only proved the transport with the prototype listener.
