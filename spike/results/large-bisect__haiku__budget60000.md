### 1. Add bisect menu key to keybinding docs  (+45 -0, 45 hunks)

Adds the 'b View bisect options' row to the keybinding reference tables in every localized Keybindings file, across the commits, branches, tags and reflog-like sections. Docs only; the translated labels should be checked by a native speaker.

- `docs-master/keybindings/Keybindings_en.md` (h1 h2 h3 h4 h5)
- `docs-master/keybindings/Keybindings_ja.md` (h6 h7 h8 h9 h10)
- `docs-master/keybindings/Keybindings_ko.md` (h11 h12 h13 h14 h15)
- `docs-master/keybindings/Keybindings_nl.md` (h16 h17 h18 h19 h20)
- `docs-master/keybindings/Keybindings_pl.md` (h21 h22 h23 h24 h25)
- `docs-master/keybindings/Keybindings_pt.md` (h26 h27 h28 h29 h30)
- `docs-master/keybindings/Keybindings_ru.md` (h31 h32 h33 h34 h35)
- `docs-master/keybindings/Keybindings_zh-CN.md` (h36 h37 h38 h39 h40)
- `docs-master/keybindings/Keybindings_zh-TW.md` (h41 h42 h43 h44 h45)

### 2. Add bisect candidate query and commit counting to git layer  (+85 -0, 6 hunks)

Adds LoadCandidates, which asks git rev-list which loaded commits are not yet ruled out, and CountCommitsAbove, used to size the commit load. It also adds candidate and current-test state helpers to BisectInfo. Failures are only logged, so the UI falls back to the old behaviour, but the new git calls run on every commits refresh during a bisect.

- `pkg/commands/git_commands/bisect.go` (h46 h47 h48)
- `pkg/commands/git_commands/bisect_info.go` (h49 h50 h51)

### 3. Make commit loading take a count instead of a bool  (+29 -5, 8 hunks)

Changes GetCommitsOptions.Limit from bool to int, so callers can request a specific number of commits, with 0 meaning all. The commits and sub-commits views track the limit they loaded with, so the view can load everything when the selection nears the end.

- `pkg/commands/git_commands/commit_loader.go` (h52 h53)
- `pkg/gui/context/local_commits_context.go` (h54 h55)
- `pkg/gui/context/sub_commits_context.go` (h56 h57)
- `pkg/gui/controllers/helpers/sub_commits_helper.go` (h76 h77)

### 4. Rework bisect controller to act on any ref  (+207 -153, 7 hunks)

Splits the bisect controller into one instance per commit-like context (commits, sub-commits, reflog, branches, remote branches, tags). Menus now act on the selected ref, show the result of a finished bisect, offer reset when git stops early, and only offer the full menu in the commits panel. Removes the old selectCurrentBisectCommit logic. Behaviour change: the menu shown depends on bisect state, which the integration tests cover.

- `pkg/gui/controllers.go` (h58 h59)
- `pkg/gui/controllers/bisect_controller.go` (h60 h61 h62 h63 h64)

### 5. Select the commit to test or first bad commit on refresh  (+122 -28, 11 hunks)

Refresh now picks the bisect commit to select (the commit to test, or the first bad commit once finished) and scrolls it into view. It loads a window of commits sized to reach that commit, and selection moves only when that commit changes, not on every refresh. The commits view render is also rerendered explicitly instead of via refreshView. This touches the core refresh path and is the riskiest part of the change.

- `pkg/gui/controllers/helpers/refresh_helper.go` (h65 h66 h67 h68 h69 h70 h71 h72 h73 h74 h75)

### 6. Load remaining commits near the end of the list  (+22 -12, 3 hunks)

Replaces the hard-coded 200-commit threshold with a shared helper that loads all commits when the selection gets within a margin of the loaded limit. Used by both the commits and sub-commits views.

- `pkg/gui/controllers/local_commits_controller.go` (h78 h79)
- `pkg/gui/controllers/sub_commits_controller.go` (h80)

### 7. Start in commits panel during an active bisect  (+6 -2, 3 hunks)

When a bisect is in progress at startup, focus the commits panel unless a filter path or an explicit startup argument decides otherwise.

- `pkg/gui/gui.go` (h81 h82 h83)

### 8. Show bisect status from git candidate data  (+8 -40, 7 hunks)

Removes the index-based bisect bounds and instead marks commits as candidates using the git-provided candidate set. Also tightens the current-commit check so it applies only when git has a commit to test.

- `pkg/gui/presentation/commits.go` (h84 h85 h86 h87 h88 h89 h90)

### 9. Update bisect translations for ref-based menus  (+14 -2, 3 hunks)

Replaces MarkStart with ref-aware templates and adds strings for describing commits, branches and tags, skip labels, and the 'use commits panel' message. Translation keys are renamed, so any non-English locale files that used the old keys need checking.

- `pkg/i18n/english.go` (h91 h92 h93)

### 10. Swap start bisect shell helper argument order  (+2 -2, 1 hunks)

Changes the integration shell helper StartBisect to take bad before good, matching the order git uses in its command.

- `pkg/integration/components/shell.go` (h94)

### 11. Add integration tests for bisect behaviour  (+637 -4, 18 hunks)

Adds new integration tests covering candidate marking, merge-base cases, result display, selection of the current and first bad commit, starting from branches, tags, sub-commits and reflog, and startup in the commits panel. Also updates two existing bisect tests to match the new menu and selection behaviour, and registers them in the test list.

- `pkg/integration/tests/bisect/check_out_branch_before_bisecting.go` (h95)
- `pkg/integration/tests/bisect/from_other_branch.go` (h96 h97)
- `pkg/integration/tests/bisect/mark_candidates_while_testing_merge_base.go` (h98)
- `pkg/integration/tests/bisect/offer_reset_when_merge_base_is_bad.go` (h99)
- `pkg/integration/tests/bisect/select_current_commit.go` (h100)
- `pkg/integration/tests/bisect/select_current_commit_in_long_range.go` (h101)
- `pkg/integration/tests/bisect/select_first_bad_commit.go` (h102)
- `pkg/integration/tests/bisect/select_first_bad_commit_on_startup.go` (h103)
- `pkg/integration/tests/bisect/show_result_again.go` (h104)
- `pkg/integration/tests/bisect/show_result_when_found_right_away.go` (h105)
- `pkg/integration/tests/bisect/skip.go` (h106)
- `pkg/integration/tests/bisect/start_from_branches.go` (h107)
- `pkg/integration/tests/bisect/start_from_sub_commits_and_reflog.go` (h108)
- `pkg/integration/tests/bisect/start_from_tags_and_remote_branches.go` (h109)
- `pkg/integration/tests/bisect/start_in_commits_panel.go` (h110)
- `pkg/integration/tests/test_list.go` (h111 h112)

### 12. Add integration test for scrolling to conflicted commit  (+37 -0, 1 hunks)

Adds a UI test that checks a background refresh scrolls a newly conflicted commit into view when it lies below the visible area.

- `pkg/integration/tests/ui/background_refresh_scrolls_to_conflicted_commit.go` (h113)

