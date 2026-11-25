### 1. Add squash-merging working tree state  (+28 -5, 12 hunks)

Introduce a new working tree state for a squash merge in progress, detected via the presence of SQUASH_MSG, and wire it into the state model, display titles, operation names and status detection. This is the foundation the rest of the change builds on.

- `pkg/commands/models/working_tree_state.go` (h7 h8 h9 h10 h11 h12 h13 h14 h15 h16)
- `pkg/commands/git_commands/status.go` (h5 h6)

### 2. Add SquashMerge command writing SQUASH_MSG  (+19 -0, 2 hunks)

Add a branch command that runs the squash merge and, if git started it, writes the configured commit message into SQUASH_MSG so that continuing the merge uses it.

- `pkg/commands/git_commands/branch.go` (h1 h2)

### 3. Continue and abort squash merges with commit and reset  (+21 -1, 2 hunks)

Map continue/abort for a squash merge onto `git commit` and `git reset --merge`, since `git merge --continue/--abort` refuse without MERGE_HEAD. Note the panic path for unsupported commands.

- `pkg/commands/git_commands/rebase.go` (h3 h4)

### 4. Use squash-merge-aware commands in merge helper  (+34 -27, 5 hunks)

Update the merge helper to treat squash merging like merging for commit selection and subprocess handling, route the uncommitted variant through the new command, and select the new commit after squash merge and commit. Also documents why squash merges are not offered with staged changes.

- `pkg/gui/controllers/helpers/merge_and_rebase_helper.go` (h17 h18 h19 h20 h21)

### 5. Add integration tests for squash merge continue/abort  (+202 -0, 5 hunks)

Cover aborting and continuing a conflicted squash merge, and continuing an uncommitted squash merge from the merge options menu, plus register the new tests and assert the merging status in the existing squash merge test.

- `pkg/integration/tests/branch/squash_merge.go` (h22)
- `pkg/integration/tests/branch/squash_merge_abort_after_conflicts.go` (h23)
- `pkg/integration/tests/branch/squash_merge_continue_after_conflicts.go` (h24)
- `pkg/integration/tests/branch/squash_merge_uncommitted_with_conflicts.go` (h25)
- `pkg/integration/tests/test_list.go` (h26)

