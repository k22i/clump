### 1. Add file-based AppState read/update with locking  (+312 -0, 3 hunks)

Introduces app_state.go with ReadAppState/UpdateAppState, shared/exclusive file locking via a lock file (unix and Windows), skipping unchanged writes, and atomic temp-file-plus-rename writes that preserve symlinks and permissions. Risk: new locking and write logic that is platform-specific and handles read-only and unsupported-lock cases silently.

- `pkg/config/app_state.go` (h19)
- `pkg/config/file_lock.go` (h23)
- `pkg/config/file_lock_windows.go` (h24)

### 2. Switch AppConfig to the new state API  (+5 -88, 9 hunks)

Replaces the in-memory appState field and GetAppState/SaveAppState with an appStatePath and the ReadAppState/UpdateAppState interface, removing the old load/save code and the AppState type moved to app_state.go. Dummy config is cleaned up accordingly.

- `pkg/config/app_config.go` (h12 h13 h14 h15 h16 h17 h18)
- `pkg/config/dummies.go` (h21 h22)

### 3. Remove AppState from Common and test helpers  (+16 -21, 13 hunks)

Drops the unused AppState field from Common and renames the dummy constructor to NewDummyCommonWithUserConfig. Test callers are updated to stop passing app state.

- `pkg/app/app.go` (h1)
- `pkg/common/common.go` (h10)
- `pkg/common/dummies.go` (h11)
- `pkg/commands/git_commands/commit_test.go` (h3)
- `pkg/commands/git_commands/deps_test.go` (h4 h5)
- `pkg/commands/git_commands/file_loader_test.go` (h6)
- `pkg/commands/git_commands/stash_test.go` (h7)
- `pkg/commands/git_commands/working_tree_test.go` (h8 h9)
- `pkg/gui/filetree/file_tree_test.go` (h31)
- `pkg/gui/filetree/file_tree_view_model_test.go` (h32 h33)

### 4. Expose ReadAppState/UpdateAppState through the GUI interface  (+12 -11, 2 hunks)

Replaces GetAppState, SaveAppState and SaveAppStateAndLogError on IGuiCommon with ReadAppState and UpdateAppState that log errors instead of returning them. Save failures no longer abort loading or show an error popup.

- `pkg/gui/types/common.go` (h43)
- `pkg/gui/gui_common.go` (h37)

### 5. Convert GUI callers to read and update state on demand  (+40 -41, 14 hunks)

Call sites now read state.yml when needed and express changes as updates applied to the current state (history, hide command log, startup popup, last version, recent repos). The recent repos menu now excludes the current repo by path rather than skipping the first entry.

- `pkg/gui/controllers/helpers/repos_helper.go` (h25)
- `pkg/gui/controllers/shell_command_action.go` (h26 h27 h28)
- `pkg/gui/extras_panel.go` (h29 h30)
- `pkg/gui/gui.go` (h34 h35 h36)
- `pkg/gui/layout.go` (h38 h39 h40)
- `pkg/gui/recent_repos_panel.go` (h41 h42)

### 6. Use new state API at startup and update checks  (+13 -4, 3 hunks)

openRecentRepo and the updater now read state via ReadAppState (logging errors) and record the last update check with UpdateAppState.

- `pkg/app/app.go` (h2)
- `pkg/updates/updates.go` (h56 h57)

### 7. Update integration tests to seed recent repos via UpdateAppState  (+38 -14, 12 hunks)

Integration test setup now sets RecentRepos through UpdateAppState instead of mutating the in-memory state. Some tests also rename the config parameter to cfg to avoid shadowing the package.

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

### 8. Add tests for app state reading, updating and locking  (+220 -0, 1 hunks)

Covers broken files, cross-instance visibility, preserved updates, skipping unchanged writes, concurrent updates, read-only folders, symlinks, file permissions and Windows open-file behaviour.

- `pkg/config/app_state_test.go` (h20)

