# git and gh run pinned and never wait on a terminal

Every git call sets `GIT_OPTIONAL_LOCKS=0` and `GIT_TERMINAL_PROMPT=0`; every gh call sets
`NO_COLOR=1` and `GH_NO_UPDATE_NOTIFIER=1`.

- `GIT_OPTIONAL_LOCKS=0`: clump and lazygit run side by side, so an opportunistic index write could
  block the user's next git command for the sake of a `git status` refresh clump does not need.
- `GIT_TERMINAL_PROMPT=0` / no terminal handoff for gh: a missing credential must come back as an
  error clump can print, not as a password prompt inside a TUI that owns the terminal.
- `NO_COLOR`: the patch must stay plain text (`gitdiff.Parse` strips SGR, but only what git was
  asked to add).
- `GH_NO_UPDATE_NOTIFIER`: never spend a run on an update check.

Both runners also clip their error text and route it through `provider.Redact`, so a failed command
cannot leak a token into a notice.
