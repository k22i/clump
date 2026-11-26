# Troubleshooting

**Start with `clump doctor`.** It prints what clump sees, and sends nothing to any provider:

```
config      /Users/you/.config/clump/config.toml
aliases     default=cheap cheap=claude-haiku strong=claude-sonnet
provider    claude-haiku     command   haiku (runs /usr/local/bin/claude)
limits      ceiling=125000 budget=120000 max_groups=12 auto_group=true
generated   detect=true builtin=true patterns=0 review=0
cache       /Users/you/.cache/clump
github      gh at /opt/homebrew/bin/gh (--pr fetches through it)
handoff     default=auto (claude)
agent       claude           replace  /usr/local/bin/claude (--version cli.js:386 (exited with exit status 1): this agent may be broken)
agent       codex            replace  /opt/homebrew/bin/codex --version ok: codex-cli 0.172.0
repo        /Users/you/src/project
socket      /var/folders/.../clump/1a2b3c4d5e6f7a8b.sock (no watch running)
rex         true (REX_SESSION="..." REX_BLOCK="...")
theme       dark (12ms)
```

For a command provider it shows which binary would run, or `(claude not on PATH)`; for an HTTP
provider, whether its `api_key_env` is set; for every coding agent (`H` / `clump handoff`), the
resolved path and a `--version` smoke test — `(--version …: this agent may be broken)` means the
binary on PATH is not healthy, and a handoff will still try but is more likely to surprise you.
Also shown: in a repository, the socket and whether a watch is listening on it.

**"Grouping works in my shell but fails in the watch pane."** The pane runs your login shell's
`PATH`, which can differ from your interactive shell's — the real failure the e2e run hit: inside
Rex panes `claude` resolved to an old install that crashed. Run `clump doctor` inside a Rex pane,
compare the `runs …` path, then give `argv[0]` an absolute path:

```toml
[providers.claude-haiku]
type = "command"
argv = ["/Users/you/.local/bin/claude", "-p", "--model", "haiku", ...]   # the one `which claude` shows in your shell
```

**"Another clump watch is already running for this repo."** One watch per repository, by design: the
socket is per repo root. Close the other pane (`clump_watch_close`, or `q` in it). A watch that died
without cleaning up is handled: the next one removes the stale socket, and `clump doctor` says
whether anything is actually listening.

**Nothing happens when I move in lazygit.** Either the renderer is not configured (run
`clump lazygit install`; lazygit's `config.yml` needs `clump push` under `diffRenderers:`) or the
selection is one clump does not follow (branches panel, working tree without `follow_worktree`,
a file inside a commit) — the pane then says `not followed`.

**"press g to group".** The estimate was over `ceiling` and watch never auto-sends above it, or
auto-group is off (`A`). Press `g` to send it anyway.

**"per-file fallback" in the header.** The model's answer had no usable hunk IDs, so clump showed
one group per file rather than nothing; `--log` records the raw answer. Press `r` to regroup, usually
after `m` to a stronger provider.

**Colours look wrong.** clump asks the terminal whether it is light or dark before drawing, and
assumes dark if it does not answer; set `CLUMP_THEME=light` or `dark`. Inside lazygit a washed-out
panel usually means `colorArg: never` on the renderer — leave it at the default.

**Where is the socket?** `$XDG_RUNTIME_DIR/clump/<first 16 hex of sha256(repo root)>.sock`, else
`$TMPDIR/clump/….sock`; when that path would break macOS's 104-byte limit the socket goes to
`/tmp/clump-<uid>/` instead. Directory 0700, socket 0600, and clump refuses a socket directory that
is a symlink or owned by another user.

**"`H` / `clump handoff` refuses to replace."** `replace` turns the clump process into the agent
and needs the agent's stdin to be a terminal — `git show | clump handoff --where replace` pipes a
dead stdin, so it is refused. Use `--where pane` (a new split keeps stdin's TTY) or `--where print`
to keep a script unattended. In watch mode `replace` never appears at all: the pane lazygit pushes
to would die with clump; watch hands off to a pane (Rex or tmux) only.
