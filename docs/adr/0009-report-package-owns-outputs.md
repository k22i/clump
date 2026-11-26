# `internal/report` owns the Markdown and JSON outputs

`report` renders the grouped diff as Markdown (`y`, `--plain`) and JSON (`--json`) for every
caller: the TUI, one-shot mode and scripts.

It existed before the TUI (Phase 1 needed `--plain`), so the outputs are not a TUI concern and an
agent-facing `--json` shape cannot drift from what `y` copies. The clipboard part of `y` lives in
`tui/sys`, which is the only place that touches the outside world from the UI.
