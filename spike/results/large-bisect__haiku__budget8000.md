### 1. Document the commits panel's bisect-candidate marks  (+45 -0, 45 hunks)

Adds a line to each localized Keybindings doc, auto-generated from pkg/i18n, for the new bisect-related keybinding or behaviour. Changes are docs only; the wording should be checked against the English source in pkg/i18n.

- `docs-master/keybindings/Keybindings_en.md` (h1 h2 h3 h4 h5)
- `docs-master/keybindings/Keybindings_ja.md` (h6 h7 h8 h9 h10)
- `docs-master/keybindings/Keybindings_ko.md` (h11 h12 h13 h14 h15)
- `docs-master/keybindings/Keybindings_nl.md` (h16 h17 h18 h19 h20)
- `docs-master/keybindings/Keybindings_pl.md` (h21 h22 h23 h24 h25)
- `docs-master/keybindings/Keybindings_pt.md` (h26 h27 h28 h29 h30)
- `docs-master/keybindings/Keybindings_ru.md` (h31 h32 h33 h34 h35)
- `docs-master/keybindings/Keybindings_zh-CN.md` (h36 h37 h38 h39 h40)
- `docs-master/keybindings/Keybindings_zh-TW.md` (h41 h42 h43 h44 h45)

### 2. Add translation strings for bisect status and messages  (+14 -2, 3 hunks)

Adds new fields to the Bisect translation struct and English strings used by the new bisect UI messages. Other locales must be kept in sync with these keys.

- `pkg/i18n/english.go` (h91 h92 h93)

### 3. Compute bisect candidates with git rev-list  (+85 -0, 6 hunks)

Adds git-backed helpers that ask git for commits not yet ruled out by a bisect, and exposes the bisect info needed by the commit loader. The rev-list query runs during refreshes, so it may add cost on large repos.

- `pkg/commands/git_commands/bisect.go` (h46 h47 h48)
- `pkg/commands/git_commands/bisect_info.go` (h49 h50 h51)

### 4. Load commits and mark candidates during bisect  (+35 -44, 13 hunks)

Changes commit loading so the commits view can use the rev-list result to mark candidate commits with '?'. Existing commit-loading behaviour is touched in the loader and in the commits context limits.

- `pkg/commands/git_commands/commit_loader.go` (h52 h53)
- `pkg/gui/context/local_commits_context.go` (h54 h55)
- `pkg/gui/context/sub_commits_context.go` (h56 h57)
- `pkg/gui/presentation/commits.go` (h84 h85 h86 h87 h88 h89 h90)

### 5. Refresh commits and select the first bad commit  (+130 -31, 16 hunks)

Updates the refresh helper to load commits down to the selected commit and to select the first bad commit once a bisect is finished, and adds a conflicted-commit finder for background refresh. Refresh ordering changes are the main risk here.

- `pkg/gui/controllers/helpers/refresh_helper.go` (h65 h66 h67 h68 h69 h70 h71 h72 h73 h74 h75)
- `pkg/gui/controllers/helpers/sub_commits_helper.go` (h76 h77)
- `pkg/gui/gui.go` (h81 h82 h83)

### 6. Show bisect result instead of the bisect menu  (+196 -150, 5 hunks)

Reworks the bisect controller so a finished bisect shows the 'Bisect complete' dialog instead of mark/skip options, and resets only when the merge base is bad. Removes the old afterMark selection helper, so check that callers are updated.

- `pkg/gui/controllers/bisect_controller.go` (h60 h61 h62 h63 h64)

### 7. Wire controllers and commit handlers to the new flow  (+33 -15, 5 hunks)

Updates controller wiring and the local and sub-commits controllers to match the refresh and bisect changes, and removes the old log-menu handling path.

- `pkg/gui/controllers.go` (h58 h59)
- `pkg/gui/controllers/local_commits_controller.go` (h78 h79)
- `pkg/gui/controllers/sub_commits_controller.go` (h80)

### 8. Add integration tests for bisect candidates and results  (+676 -6, 20 hunks)

Adds integration tests covering candidate marks, the first-bad-commit selection, the result dialog, and starting bisect from various refs and the commits panel. Also adds test helpers and registers the new tests.

- `pkg/integration/components/shell.go` (h94)
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
- `pkg/integration/tests/ui/background_refresh_scrolls_to_conflicted_commit.go` (h113)

