# Rex

Rex is the terminal multiplexer this was built for; it also works anywhere else.

## Install

`clump rex install` writes clump's Lua module to `~/.local/share/clump/rex/init.lua` (or
`$XDG_DATA_HOME/clump/rex/init.lua`) and prints the line to add to `~/.config/rex/init.lua`:

```lua
dofile("/Users/you/.local/share/clump/rex/init.lua") -- clump: actions clump_pick, clump_watch, clump_watch_close
```

`clump rex install --write` asks `[y/N]`, backs `~/.config/rex/init.lua` up as
`init.lua.clump-backup-<timestamp>` and appends the line; a file that already loads the module is
left alone.

## Actions

Rex action names cannot contain dots, so the task's `clump.pick`, `clump.watch` and
`clump.watch_close` are spelled `clump_pick`, `clump_watch`, `clump_watch_close`.

| action | what it does |
|---|---|
| `clump_watch` | opens `clump watch --repo <root>` in a split to the right of the focused pane (lazygit), labelled `clump-watch:<root>`; if a pane with that label exists it focuses it instead and reports `created: false` |
| `clump_watch_close` | finds the pane by that label and closes it |
| `clump_pick` | shows a picker ("This branch vs <base>", recent commits) and opens clump's one-shot TUI in a floating layer over the window |

`clump_pick` asks Rex for a picker with `rex.ui.pick`; when Rex cannot show one (`unsupported`,
`no_client`, `busy`, `timeout`, `failed`) it opens `clump --pick` in a floating layer instead. On
the Rex app build tested `rex.ui.pick` returned `unsupported`, so that fallback is what runs.

The repository comes from the acting pane: its OSC 7 working directory, else its foreground
process's cwd, then `git rev-parse --show-toplevel`. Actions take `block` and `repo` args for
scripting (`rex do <file> --action clump_watch repo=/path/to/repo`).

## Bindings

Default bindings, added when the module loads:

| keys | action |
|---|---|
| `ctrl+alt+g` | `clump_pick` |
| `ctrl+alt+w` | `clump_watch` |
| `ctrl+alt+q` | `clump_watch_close` |

Set `CLUMP_NO_BINDINGS = true` before `dofile` to skip them and bind your own with `rex.bind`. Set
`CLUMP_BIN` when `clump` is not on the login shell's `PATH`:

```lua
CLUMP_BIN = "/Users/you/go/bin/clump"
CLUMP_NO_BINDINGS = true
dofile("/Users/you/.local/share/clump/rex/init.lua")
rex.bind("alt+g", "clump_pick")
```

## Program status and theme

With `REX_SESSION` and `REX_BLOCK` set, clump reports its state to Rex as `app=clump`: `working`
while it groups, `blocked kind=question` over the ceiling, `done` with the group count, `error` with
the redacted message — Rex's tab status, working / waiting for you / done / error.

The colour theme follows the terminal's light/dark setting, asked with a terminal query before the
UI starts (Rex answers in about 0.2 ms); `CLUMP_THEME=light` or `dark` overrides the query.
