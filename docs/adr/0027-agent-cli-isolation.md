# Agent CLIs run isolated from the repo

A command provider runs its argv with the prompt on stdin, `cmd.Env` left nil (so the CLI inherits
PATH and the user's API keys) and cwd `os.TempDir()`.

The working directory is the decisions that matters: run in the repo, an agent CLI picks up
`CLAUDE.md`, `AGENTS.md` and whatever else it finds, and the grouping silently changes with the
project. The built-in `claude -p` argv goes further (`--setting-sources ""`,
`--strict-mcp-config`, `--tools ""`, `--no-session-persistence`) so the answer is a bare JSON object
and nothing else.

The CLI is run in its own process group so cancelling the context kills the whole tree, including a
shell wrapper's children; if the group is already gone, the direct child is signalled instead so the
call still ends. `killWaitDelay` bounds the wait for the output pipes, so a stuck grandchild cannot
hang the call.
