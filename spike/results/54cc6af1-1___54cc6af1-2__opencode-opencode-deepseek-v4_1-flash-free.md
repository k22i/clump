### 1. Add locked, atomic app state persistence  (+312 -0, 3 hunks)

Introduces a dedicated app_state.go implementing ReadAppState and UpdateAppState with shared/exclusive lock files, atomic temp-file-and-rename writes, fsync, and fallback-to-defaults parsing. The platform-specific lock helpers (unix flock, Windows LockFileEx) back these calls.

- `pkg/config/app_state.go` (h19)
- `pkg/config/file_lock.go` (h23)
- `pkg/config/file_lock_windows.go` (h24)

### 2. Store only the state path in AppConfig  (+5 -88, 9 hunks)

Replaces the cached AppState field with an appStatePath and swaps GetAppState/SaveAppState for ReadAppState/UpdateAppState in the AppConfigurer interface. The old load/save code and AppState definition move out, and config dummies drop the now-unused field.

- `pkg/config/app_config.go` (h12 h13 h14 h15 h16 h17 h18)
- `pkg/config/dummies.go` (h21 h22)

### 3. Replace GUI common state accessors  (+12 -11, 2 hunks)

Updates IGuiCommon and guiCommon to expose ReadAppState/UpdateAppState, logging errors instead of returning them so state failures no longer abort normal operations.

- `pkg/gui/types/common.go` (h43)
- `pkg/gui/gui_common.go` (h37)

### 4. Read and update state during GUI startup  (+16 -9, 7 hunks)

Migrates startup paths (recent repo opening, command-log visibility, intro popup, breaking changes, last version) to read state on demand or apply changes through UpdateAppState. Behaviour changes: intro confirmation no longer surfaces a save error, and recent repo lookup reads the file live.

- `pkg/app/app.go` (h2)
- `pkg/gui/gui.go` (h34 h35 h36)
- `pkg/gui/layout.go` (h38 h39 h40)

### 5. Update shell command history and extras panel via callbacks  (+18 -24, 5 hunks)

Converts shell command history addition/deletion and the command-log toggle to UpdateAppState callbacks, removing the old read-modify-save sequence and related slices import.

- `pkg/gui/controllers/shell_command_action.go` (h26 h27 h28)
- `pkg/gui/extras_panel.go` (h29 h30)

### 6. Exclude current repo from recent repos by path  (+5 -5, 1 hunks)

Stops assuming the first recent-repos entry is the current repo; it instead filters the list by the actual working directory, so entries from other instances are handled correctly.

- `pkg/gui/controllers/helpers/repos_helper.go` (h25)

### 7. Record recent repos through UpdateAppState  (+6 -4, 2 hunks)

Rewrites updateRecentRepoList to derive the new list inside an UpdateAppState callback rather than mutating a cached slice and saving it.

- `pkg/gui/recent_repos_panel.go` (h41 h42)

### 8. Migrate updater state access  (+8 -3, 2 hunks)

Uses UpdateAppState to record the last update check and reads state on demand when deciding whether to skip a check, logging read errors.

- `pkg/updates/updates.go` (h56 h57)

### 9. Remove AppState from Common  (+7 -11, 3 hunks)

Drops the unused AppState field from common.Common, simplifies NewCommon, and renames the test dummy constructor to NewDummyCommonWithUserConfig.

- `pkg/app/app.go` (h1)
- `pkg/common/common.go` (h10)
- `pkg/common/dummies.go` (h11)

### 10. Update unit tests for the new dummy constructor  (+9 -10, 10 hunks)

Mechanical test updates removing the appState argument from git command dependencies and switching filetree tests to NewDummyCommonWithUserConfig.

- `pkg/commands/git_commands/commit_test.go` (h3)
- `pkg/commands/git_commands/deps_test.go` (h4 h5)
- `pkg/commands/git_commands/file_loader_test.go` (h6)
- `pkg/commands/git_commands/stash_test.go` (h7)
- `pkg/commands/git_commands/working_tree_test.go` (h8 h9)
- `pkg/gui/filetree/file_tree_test.go` (h31)
- `pkg/gui/filetree/file_tree_view_model_test.go` (h32 h33)

### 11. Add app state persistence tests  (+220 -0, 1 hunks)

Covers broken files, cross-instance visibility and update merging, skipped rewrites, concurrent instances, read-only folders, symlinks, permission preservation and Windows file-in-use errors.

- `pkg/config/app_state_test.go` (h20)

### 12. Seed integration test state via UpdateAppState  (+38 -14, 12 hunks)

Updates the setup callbacks to seed RecentRepos through UpdateAppState instead of mutating the removed in-memory state, renaming shadowed config parameters where needed.

- `pkg/integration/tests/config/custom_commands_in_per_repo_config.go` (h44)
- `pkg/integration/tests/config/side_panels_in_per_repo_config.go` (h45)
- `pkg/integration/tests/misc/direnv_approves_envrc.go` (h46)
- `pkg/integration/tests/misc/direnv_loaded_on_repo_switch.go` (h47)
- `pkg/integration/tests/misc/direnv_unloads_on_blocked_envrc.go` (h48)
- `pkg/integration/tests/misc/filter_recent_repos.go` (h49)
- `pkg/integration/tests/misc/recent_repos_branch_column.go` (h50)
- `pkg/integration/tests/misc/recent_repos_column_widths.go` (h51)
- `pkg/integration/tests/misc/recent_repos_reftable_repo.go` (h52)
- `pkg/integration/tests/misc/recent_repos_with_long_names.go` (h53)
- `pkg/integration/tests/ui/keybinding_suggestions_when_switching_repos.go` (h54)
- `pkg/integration/tests/ui/switch_repo_moves_the_selection.go` (h55)

