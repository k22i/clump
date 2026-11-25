### 1. Add squash-merging working tree state  (+28 -5, 12 hunks)

Introduces a SquashMerging state in the working tree model, mapped to the existing merge titles and names, and detects it via the SQUASH_MSG file in status commands. This is the foundation for showing a merge in progress after a squash merge.

- `pkg/commands/git_commands/status.go` (h5 h6)
- `pkg/commands/models/working_tree_state.go` (h7 h8 h9 h10 h11 h12 h13 h14 h15 h16)

### 2. Write configured message to SQUASH_MSG on squash merge  (+19 -0, 2 hunks)

Adds BranchCommands.SquashMerge, which runs the squash merge and then overwrites SQUASH_MSG with the configured message, but only if git created the file. This lets continuing use the configured message and keeps both squash variants consistent.

- `pkg/commands/git_commands/branch.go` (h1 h2)

### 3. Continue and abort squash merges with commit and reset  (+21 -1, 2 hunks)

Maps continue to 'git commit' and abort to 'git reset --merge' for squash merges, since MERGE_HEAD doesn't exist. The new state is also added to the exhaustive switch in the generic action.

- `pkg/commands/git_commands/rebase.go` (h3 h4)

### 4. Wire squash merge continue/abort into the merge helper  (+34 -27, 5 hunks)

Treats the squash-merging state like merging in the helper, so it selects the new commit and honours manual commit. Both squash variants now use SquashMerge, the commit variant continues via the generic action, and the uncommitted variant disables the continue offer. Also updates a related comment.

- `pkg/gui/controllers/helpers/merge_and_rebase_helper.go` (h17 h18 h19 h20 h21)

### 5. Add integration tests for squash merge conflicts  (+202 -0, 5 hunks)

Adds tests for aborting, continuing, and leaving uncommitted after squash merge conflicts, extends the existing squash merge test to check the status, and registers the new tests.

- `pkg/integration/tests/branch/squash_merge.go` (h22)
- `pkg/integration/tests/branch/squash_merge_abort_after_conflicts.go` (h23)
- `pkg/integration/tests/branch/squash_merge_continue_after_conflicts.go` (h24)
- `pkg/integration/tests/branch/squash_merge_uncommitted_with_conflicts.go` (h25)
- `pkg/integration/tests/test_list.go` (h26)

