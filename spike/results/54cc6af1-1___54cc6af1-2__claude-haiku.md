### 1. Add lock-protected, atomic state.yml read/update layer  (+532 -0, 4 hunks)

Introduces pkg/config/app_state.go with ReadAppState and UpdateAppState, which take a lock on a separate .lock file, re-read the file on every call, and write via temp file + fsync + rename, preserving symlink targets, permissions, and read-only files. Also adds platform-specific file locking (flock on Unix, LockFileEx on Windows). Risks: locking may silently be skipped on unsupported filesystems or read-only folders; an unparseable file is silently replaced with defaults on the next update.

- `pkg/config/app_state.go` (h19)
- `pkg/config/app_state_test.go` (h20)
- `pkg/config/file_lock.go` (h23)
- `pkg/config/file_lock_windows.go` (h24)

### 2. Replace in-memory AppState with read/update API in config  (+5 -88, 9 hunks)

Removes the cached appState from AppConfig and its Dummy constructor, the old GetAppState/SaveAppState/loadAppState/SaveAppState code, and the AppState type's old home. AppConfig now stores only the state file path. The AppConfigurer interface switches to ReadAppState and UpdateAppState.

- `pkg/config/app_config.go` (h12 h13 h14 h15 h16 h17 h18)
- `pkg/config/dummies.go` (h21 h22)

### 3. Remove AppState from Common and update test helpers  (+12 -15, 12 hunks)

Drops the AppState field from common.Common and the dependency injection in git_commands test deps, replacing NewDummyCommonWithUserConfigAndAppState with NewDummyCommonWithUserConfig. Mechanical test updates follow the removed parameter.

- `pkg/common/common.go` (h10)
- `pkg/common/dummies.go` (h11)
- `pkg/commands/git_commands/commit_test.go` (h3)
- `pkg/commands/git_commands/deps_test.go` (h4 h5)
- `pkg/commands/git_commands/file_loader_test.go` (h6)
- `pkg/commands/git_commands/stash_test.go` (h7)
- `pkg/commands/git_commands/working_tree_test.go` (h8 h9)
- `pkg/gui/filetree/file_tree_test.go` (h31)
- `pkg/gui/filetree/file_tree_view_model_test.go` (h32 h33)

### 4. Route app and updater state access through new API  (+17 -10, 4 hunks)

Switches the app startup, update checker, and update recording code to ReadAppState/UpdateAppState. The update check now reads state on each call; a read error is logged and defaults are used. Read errors are logged rather than returned.

- `pkg/app/app.go` (h1 h2)
- `pkg/updates/updates.go` (h56 h57)

### 5. Convert GUI state callers to atomic update operations  (+46 -46, 14 hunks)

Replaces direct mutation of the cached state in the GUI (shell command history, command log visibility, intro popup version, last version, recent repos) with read-only reads and UpdateAppState closures that apply changes to the freshly read file. Save errors are now logged instead of returned; the intro popup no longer surfaces save failures. Shell history deletion now uses value-based removal, which removes all duplicates, and the update closure alone decides the outcome.

- `pkg/gui/controllers/shell_command_action.go` (h27 h28)
- `pkg/gui/extras_panel.go` (h29 h30)
- `pkg/gui/gui.go` (h34 h35 h36)
- `pkg/gui/gui_common.go` (h37)
- `pkg/gui/layout.go` (h38 h39 h40)
- `pkg/gui/recent_repos_panel.go` (h41 h42)
- `pkg/gui/types/common.go` (h43)

### 6. Exclude current repo from recent repos by path  (+6 -6, 2 hunks)

The recent repos menu previously skipped the first entry assuming it was the current repo. It now gets the working directory and removes that path from the list, since the first entry may no longer be the current repo once state is shared across instances.

- `pkg/gui/controllers/helpers/repos_helper.go` (h25)
- `pkg/gui/controllers/shell_command_action.go` (h26)

### 7. Update integration tests to use UpdateAppState  (+38 -14, 12 hunks)

Integration test setup functions that previously assigned RecentRepos or other fields directly now call UpdateAppState with a closure; the parameter is renamed from config to cfg in two files to avoid shadowing the package name.

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

