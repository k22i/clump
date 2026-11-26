# CLI reference

## One-shot

```
clump                     # HEAD
clump <rev>               # one commit
clump main...feature      # merge base to feature, like a PR
clump A..B                # two-dot range
clump --branch            # this branch against its default base
clump --pr 123            # GitHub PR of this repo (via the gh CLI)
clump --pr k22i/clump#123 # any GitHub PR: OWNER/REPO#N or its URL
git diff | clump          # a patch on stdin
clump --pick              # choose a commit or comparison first
```

With a terminal on stdout `clump` opens the TUI; with stdout redirected (`clump > notes.md`) it prints
the Markdown summary instead. `A...B` and `--branch` show a range (the
commits' messages are the model's context), `A..B` a two-dot range, a patch on stdin is read as
given, and `--pick` shows clump's picker first: "This branch vs `<base>`" when you are not on the
base branch, then the 50 most recent commits; type to filter, Enter to open. `--pr` fetches the
patch with `gh pr diff` and gives the model the pull request's title, description and commit
subjects as context; a plain number is looked up in the repository you run it in, `OWNER/REPO#N`
and pull request URLs work from anywhere. The TUI opens
immediately: while the model works the list shows the diff's files and you can read their hunks;
the groups replace them when the answer arrives.

## Flags

| flag | effect |
|---|---|
| `--json` | print the groups as JSON instead of opening the TUI |
| `--plain` | print a Markdown summary (the text `y` puts on the clipboard) |
| `--dry-run` | print exactly what would be sent to the provider, and the estimate; send nothing |
| `--no-cache` | ignore cached results, still write them |
| `--yes` | do not ask before sending a prompt estimated above `ceiling` |
| `--provider NAME` | use a provider name or alias from the config (default: the `default` alias) |
| `--repo DIR` | work on DIR instead of the current directory |
| `--branch` | compare the current branch with its default base (`origin/HEAD`, else `main`, else `master`) |
| `--pr SPEC` | a GitHub pull request: `123`, `#123`, `OWNER/REPO#123` or its URL (the `gh` CLI fetches it) |

`--json`, `--plain` and `--dry-run` print and exit instead of opening the TUI; they are what you use
in a script. An empty diff is an error, not an empty screen.

## Comments, handoff and submit

```
clump handoff [REV | A..B | --branch | --pr SPEC] [--agent NAME]
              [--where replace|pane|print] [--yes] [--dry-run]
```

`clump handoff` sends the diff's stored review comments to a coding agent CLI — `claude`,
`codex`, `opencode` or an `[agents.<name>]` entry — as the agent's initial prompt, and gets out of
the way: it does not own the agent process. `--agent NAME` picks the config entry (the `handoff`
config value, else the first of `claude`, `codex`, `opencode` on PATH); `--where` picks the launch
shape — `replace` turns the clump process into the agent in this terminal (stdin must be a
terminal), `pane` opens a split for it (`rex split` inside Rex, else `tmux split-window`), `print`
writes the prompt and launches nothing; the default is the agent's configured `where`, else
`replace`. The comments that went out are stamped (`clump comments --open` lists the rest);
`--dry-run` prints the exact command and the prompt, launching and stamping nothing; `--yes`
skips the confirm question. See [reviews.md](reviews.md#handing-off-to-a-coding-agent).

See [reviews.md](reviews.md) for `clump comments` (read and write review comments from scripts and
coding agents, no TUI, no provider call — `--open` lists the ones not yet handed off) and
`clump submit` (post a pull request's stored comments to GitHub as one review).

## Watch and installers

| command | what it does |
|---|---|
| `clump watch` | the watch pane beside lazygit; see [lazygit-watch.md](lazygit-watch.md) |
| `clump push` | lazygit's `diffRenderers` filter; run by lazygit, not by you |
| `clump lazygit install [--write] [--then CMD]` | add `clump push` to lazygit's `config.yml` |
| `clump rex install [--write]` | install the Rex module and its bindings; see [rex.md](rex.md) |
| `clump upgrade` (`update`) | replace the binary with the latest release, checksummed |
| `clump config init` / `config path` | print the commented default config / the path read |
| `clump doctor` | print what clump sees; send nothing; see [troubleshooting.md](troubleshooting.md) |
