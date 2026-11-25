### 1. Detect and model an in-progress squash merge  (+28 -5, 12 hunks)

Adds a SquashMerging state derived from the presence of SQUASH_MSG, wires it into the working tree state model (effective state, titles, operation name, command name). This makes lazygit show a merge in progress after a squash merge stops or leaves changes uncommitted.

- `pkg/commands/git_commands/status.go` (h5 h6)
- `pkg/commands/models/working_tree_state.go` (h7 h8 h9 h10 h11 h12 h13 h14 h15 h16)

### 2. Write SQUASH_MSG and wrap squash merge in branch command  (+19 -0, 2 hunks)

Adds BranchCommands.SquashMerge, which runs the squash merge and then writes the configured message into SQUASH_MSG only if the merge actually started. Requires a filepath import. Risk: the existence check is the only guard that the merge started.

- `pkg/commands/git_commands/branch.go` (h1 h2)

### 3. Continue and abort squash merges with git commit and reset  (+21 -1, 2 hunks)

Routes continue and abort for the squash-merging state to 'git commit' and 'git reset --merge' instead of 'git merge --continue/--abort', which fail without MERGE_HEAD. Panics on any other command, so callers must only pass continue or abort.

- `pkg/commands/git_commands/rebase.go` (h3 h4)

### 4. Use squash merge state in merge and conflict flows  (+34 -27, 5 hunks)

Treats squash merging like a regular merge for subprocess and selection behaviour, lets the conflicts menu abort squash merges, and changes the Squash merge flows: the committed variant now continues via commit and selects the new commit, while the uncommitted variant stops offering to continue once conflicts are resolved. Note the uncommitted path also clears the MergeOrRebaseStartedInLazygit flag, and the abort-discard comment is updated for squash merges.

- `pkg/gui/controllers/helpers/merge_and_rebase_helper.go` (h17 h18 h19 h20 h21)

### 5. Add integration tests for squash merge conflicts  (+202 -0, 5 hunks)

Adds tests for aborting, continuing, and leaving uncommitted squash merges after conflicts, and checks the status line in the existing squash merge test. These cover the new states and should be read after the code they exercise.

- `pkg/integration/tests/branch/squash_merge.go` (h22)
- `pkg/integration/tests/branch/squash_merge_abort_after_conflicts.go` (h23)
- `pkg/integration/tests/branch/squash_merge_continue_after_conflicts.go` (h24)
- `pkg/integration/tests/branch/squash_merge_uncommitted_with_conflicts.go` (h25)
- `pkg/integration/tests/test_list.go` (h26)

