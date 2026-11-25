# Spike C — Rex APIs, verified by experiment

Server: Rex `0.1.0-preview.21`, control API revision 18. Mac app: Rex `0.1.0 (2061)`.

Docs read (snapshots in `rex-docs/`, fetched 2026-11-25): `llms.txt`, `reference/lua.md`, `customize/actions.md`,
`customize/custom-ui.md`, `automate/lua-scripts.md`, `build/program-status.md`. Method shapes come from
`rex api describe … --json` (saved as `rex-docs/api_*.json`), and terminal block options and methods come from `rex block types --json`
and `rex block inspect` (`rex-docs/block_types.json`, `rex-docs/my_block.json`).

The experiment scripts are in `rex-experiments/`, and each one's output is saved next to it as `*.result.json`.

## Results

| question | verified result | evidence |
|---|---|---|
| Can a split be created **with its command at creation**? | **Yes.** `rex.session.new_split{anchor_id=<lazygit block>, direction="horizontal", side="after", ratio=0.5, focus=false, layout=rex.layout.block{flavor="com.superlogical.terminal.shell", label=…, options={cwd=root, command={...}}}}`. No need to write to the block afterwards. The command ran in `cwd` with its arguments intact. `session inspect` shows the new block in the `after` half (`rect [0.5,0,0.5,1]`), i.e. to the **right** of the anchor. | `split.lua`, `split.result.json` |
| Terminal options | `command` (argv list), `shell` (`none` runs `command` directly; otherwise via a login shell, which gets the user's `PATH`), `cwd`, `exit={on_completion=false}`, `initial_input`, `path_prepend`, `interactive`, `size`, `theme`. **There is no `env` option**, so identity has to go through argv or the label. | `rex-docs/block_types.json` |
| Finding an existing watch pane | Label it at creation (`label="clump-watch:<repo root>"`), then find it with `rex.session.list_blocks{}`. **The label survives the program setting an OSC 2 title.** The title changed to "some program title" and the label didn't. `process` also reports the foreground process's `argv0`/`cwd` as a second check. | `split.result.json`, `layer.result.json` |
| Focus / close | `session.focus_block`, `block.close` (`keep_last`), `session.close_layer`; listed by `rex api list` | `rex api list` |
| Floating layer for the one-shot TUI | **Works.** `rex.session.new_layer{bounds={0.1,0.1,0.8,0.8}, window_id=…, layout=rex.layout.block{…command…}}` starts the command in the floating layer, and it keeps its label. | `layer.lua`, `layer.result.json` |
| `pwd` of the focused terminal | `rex.block.call("com.superlogical.terminal","pwd",{block_id=…})` returns **`{pwd="file://<host>/<path>", owner={pid,name}}`**, a `file://` URL (URL-decode it, drop the host). It's **`{}` until the shell reports OSC 7**: a freshly opened pane returned `{}` once. Fallback: `process().foreground.cwd`, which is the real cwd and matched. | in-session `rex block call … pwd` |
| `ctx` in an action | When run from the CLI (`origin=api`), `ctx` has only `session_id` (no `block_id`, no `client_id`). Key and palette runs should include `ctx.block_id`. **Not yet verified**, because I didn't edit your `init.lua`. | `pick.result.json` |
| `rex.ui.pick` | Called correctly inside an action run with `rex do pick.lua --action …` and at a script's top level. **It returned `nil, "unsupported"` straight away both times.** Per the docs that means "no connected app can show prompts, usually because they're older than the server". Your Mac app is build 2061 against server preview.21. **The picker UI is unverified.** `clump.pick` must handle `unsupported`/`no_client`/`busy`/`timeout` by opening clump's own in-TUI picker in the floating layer. | `pick.lua`, `pick.result.json` |
| Program status (OSC 7501) | **Supported.** The feature query `ESC]7501;?ESC\` replies `…;?:states=idle,working,done,blocked,error:kinds=permission,question,auth`. `working` → `blocked` (`kind=question`) → `done` → `error` reports were each read back with `program_status` and the **msg decoded by the server**. `error` stayed after the program went idle, as the spec says. | `probe.py`, `probe.result.json`, polling output in the session log |
| Light/dark: `CSI ? 996 n` | **Rex answers** `ESC[?997;1n` (1 = dark) in **0.2 ms**, before the DA1 sentinel. It also answers OSC 11 (`rgb:0d0d/1111/1717`). Not verified: unsolicited updates when the theme changes (`CSI ? 2031 h`), which needs a manual theme toggle. | `probe.result.json` |
| Running inside Rex | `REX_SESSION`, `REX_BLOCK`, `REX_SERVER`, `TERM_PROGRAM=rex` are set in panes. **They're inherited through lazygit → git → the pager**, so `push` sees the lazygit pane's `REX_BLOCK`. | `probe.result.json`, `samples/capture-logs/*.jsonl` |

## Consequences for `rex/init.lua`

- `clump.watch`:
  1. `ctx.block_id` gives the lazygit pane.
  2. Get the repo root from its `pwd`, falling back to `process().foreground.cwd`, then `rex.exec{"git","-C",dir,"rev-parse","--show-toplevel"}`.
  3. Search `list_blocks` for the label `clump-watch:<root>`. If it exists, `focus_block`.
  4. Otherwise `new_split` as above with `command={"clump","watch","--repo",root}`, `focus=false`, so lazygit keeps focus.
- `clump.watch_close`: find the pane by label, then `block.close`.
- `clump.pick`: `rex.ui.pick` with items for "this branch vs default" (icon `branch`) and recent commits (icon `tag`, `detail="+a -d"`). If it returns `unsupported` etc., open `clump --pick` in a `new_layer` and let the TUI do the picking. Actions get 30 s; `rex.exec` of `git log` fits easily.
- Program status: emit only when `REX_SESSION`/`REX_BLOCK` are set. Always include `app=clump`, because each report replaces the record.
  `working` while grouping, `blocked kind=question` for "over the ceiling, press g", `done` with the group count, `error` with a redacted message.
- Theme: query `CSI ? 996 n` with a ~100 ms timeout, using DA1 as a sentinel, before Bubble Tea takes the terminal. `CLUMP_THEME=light|dark` overrides it.
