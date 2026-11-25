### 1. Document the new bisect-related keybindings in translations  (+45 -0, 45 hunks)

Adds one line to each localized Keybindings_*.md doc file, all generated from the pkg/i18n strings. These are documentation-only changes that should follow the i18n source change, so they belong after the code that defines them.

- `docs-master/keybindings/Keybindings_en.md` (h1 h2 h3 h4 h5)
- `docs-master/keybindings/Keybindings_ja.md` (h6 h7 h8 h9 h10)
- `docs-master/keybindings/Keybindings_ko.md` (h11 h12 h13 h14 h15)
- `docs-master/keybindings/Keybindings_nl.md` (h16 h17 h18 h19 h20)
- `docs-master/keybindings/Keybindings_pl.md` (h21 h22 h23 h24 h25)
- `docs-master/keybindings/Keybindings_pt.md` (h26 h27 h28 h29 h30)
- `docs-master/keybindings/Keybindings_ru.md` (h31 h32 h33 h34 h35)
- `docs-master/keybindings/Keybindings_zh-CN.md` (h36 h37 h38 h39 h40)
- `docs-master/keybindings/Keybindings_zh-TW.md` (h41 h42 h43 h44 h45)

### 2. Add bisect state helpers for candidate and current commits  (+85 -0, 6 hunks)

Extends BisectInfo with a current-hash helper and adds IsDone and ReachableFromStart logic in BisectCommands, so the app can tell when git has finished a bisect and which commits remain candidates. Mistakes here affect every later bisect-dependent change, so review carefully.

- `pkg/commands/git_commands/bisect.go` (h46 h47 h48)
- `pkg/commands/git_commands/bisect_info.go` (h49 h50 h51)

### 3. Load commits with rev-list when a bisect is active  (+188 -33, 20 hunks)

Changes the commit loader and refresh path so that during a bisect, the candidate commits are fetched using git rev-list against the good and bad commits, then trimmed to the loaded commits. This also adds handling for scrolling to a newly conflicted commit after a background refresh. The rev-list path is a behaviour change to how commits are loaded and is a likely place for performance or correctness regressions.

- `pkg/commands/git_commands/commit_loader.go` (h52 h53)
- `pkg/gui/context/local_commits_context.go` (h54 h55)
- `pkg/gui/context/sub_commits_context.go` (h56 h57)
- `pkg/gui/controllers/helpers/refresh_helper.go` (h65 h66 h67 h68 h69 h70 h71 h72 h73 h74 h75)
- `pkg/gui/controllers/helpers/sub_commits_helper.go` (h76 h77)
- `pkg/integration/tests/ui/background_refresh_scrolls_to_conflicted_commit.go` (h113)

### 4. Rework bisect controller and mark-then-select flow  (+229 -165, 10 hunks)

Reworks the bisect controller so that once git has nothing left to test it shows the "Bisect complete" dialog instead of the mark menu, and selects the first bad commit after marking. It also removes the old afterMark helper and reworks the controller wiring. This changes user-visible behaviour, and the removed afterMark code needs checking to make sure no selection logic was lost.

- `pkg/gui/controllers.go` (h58 h59)
- `pkg/gui/controllers/bisect_controller.go` (h60 h61 h62 h63 h64)
- `pkg/gui/controllers/local_commits_controller.go` (h78 h79)
- `pkg/gui/controllers/sub_commits_controller.go` (h80)

### 5. Focus commits panel at startup during a bisect  (+6 -2, 3 hunks)

Makes lazygit start in the commits panel when a bisect is in progress, by changing the initial context selection and the startup argument parsing. Filter paths and status/branch/log/stash arguments still take precedence.

- `pkg/gui/gui.go` (h81 h82 h83)

### 6. Show bisect candidate marks in the commits view  (+8 -40, 7 hunks)

Changes the commits presentation so that the "?" candidate mark is derived from the new bisect state, and the "<-- current" label is no longer shown for a finished bisect. It also removes the old column-width reservation code for the removed label. Check that the column width calculation still matches the rendered strings.

- `pkg/gui/presentation/commits.go` (h84 h85 h86 h87 h88 h89 h90)

### 7. Add bisect translation strings for the new dialogs  (+14 -2, 3 hunks)

Adds the new English strings for the bisect complete and reset dialogs, and updates the Bisect translation struct to hold them. Other locales are not touched here.

- `pkg/i18n/english.go` (h91 h92 h93)

### 8. Add shell helper for stash messages in tests  (+2 -2, 1 hunks)

Changes the integration test shell's Stash helper signature used by the new bisect tests. This is test-only plumbing.

- `pkg/integration/components/shell.go` (h94)

### 9. Add integration tests for bisect candidate marks and selection  (+637 -4, 18 hunks)

Adds new integration tests covering candidate marks during merge-base testing, selecting the first bad commit, showing the result again, resetting when the merge base is bad, and starting bisect from various refs and panels. Also adjusts the existing from_other_branch and skip tests and registers the new tests in the test list. These tests document the intended behaviour for the groups above.

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

