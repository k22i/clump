# Rex module configuration

`rex/init.lua` is configured through two globals set before `dofile`, not through the dofile call:
`CLUMP_BIN` (the binary to run) and `CLUMP_NO_BINDINGS` (skip the default bindings).

Globals were chosen because Rex's `dofile` takes a path only, and because the module must also work
when copied into place by hand. Every action additionally takes `block`/`repo` arguments, which the
`rex do --action` tests use, where `ctx` has no `block_id`.

Default bindings: `ctrl+alt+g` pick, `ctrl+alt+w` watch, `ctrl+alt+q` close.

Action names cannot contain dots (`clump.pick` is not expressible), hence `clump_pick`,
`clump_watch`, `clump_watch_close`.

## What the module does

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
