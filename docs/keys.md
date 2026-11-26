# Keys

Press `?` for the same list inside the TUI.

**Navigate**

| key | action |
|---|---|
| `j/k ↓/↑` | next/previous group (lines in the diff pane) |
| `enter l →` | focus the diff pane |
| `esc h ←` | back to the group list |
| `]/[` | next/previous group from anywhere |
| `}/{` | next/previous hunk (diff pane) |
| `ctrl+d/ctrl+u` | half page down/up |
| `a` | toggle grouped / all hunks in file order |
| `wheel` | scroll the window over the diff pane (or the group list); the highlight stays on the edge the window scrolls past |
| `click` | put the cursor on the line (diff pane) or group (list) under the pointer |

**Review**

| key | action |
|---|---|
| `c` | mark the file under the cursor (diff pane) or the whole group (list) reviewed; when the group is done, move on to the next unreviewed one. `c` again unmarks |
| `C` | comment on the line, file header or group under the cursor (saved as you type enter; `esc` cancels) |
| `D` | delete the comment row under the cursor |
| `z` | fold/unfold the file under the cursor (diff pane) or every file of the group (list) |
| `Z` | fold/unfold every file in the pane |
| `#` | line numbers on/off |
| `s` | side-by-side diff on/off (old left, new right) |

Reviewed files fold, their header gets a `✓`, and the list shows `2/5h` for a group you have partly
reviewed and `✓` for one you have finished. The header shows `3/7 reviewed`, which does not count
the Generated files group. Generated files start folded. `}`/`{` stop at folded files, and `x` or `e`
on a folded header act on its hunks.

**Search & tools**

| key | action |
|---|---|
| `/` | search the diff pane |
| `n/N` | next/previous match (while searching) |
| `e` | open `$EDITOR` at the hunk |
| `y` | copy a Markdown summary |
| `H` | hand the review's comments to a coding agent (`claude`, `codex`, `opencode` or an `[agents.<name>]` entry) as its initial prompt, then get out of the way — a picker lists each configured agent and its launch shape; `enter` picks, `esc` cancels ([Posting a review](reviews.md#handing-off-to-a-coding-agent)) |
| `P` | post the comments as a review of the pull request (a diff opened with `--pr`; see [Posting a review](reviews.md#posting-a-review)) |
| `r` | regroup (bypass the cache) |
| `m` | switch provider |
| `?` | this help |
| `q ctrl+c` | quit |

**Edit groups**

| key | action |
|---|---|
| `x` | mark the hunk (the whole group in the list) |
| `v` | visual mode: mark a range of hunks |
| `M` | move marked hunks to a group or a new one |
| `n` | new empty group |
| `R` | rename group and edit its explanation |
| `d` | delete group (hunks go to Ungrouped) |
| `J/K` | move group down/up |
| `g` | merge this group into another |
| `u/ctrl+r` | undo/redo |

**Watch**

| key | action |
|---|---|
| `A` | toggle auto-group |
| `g` | group now (when not grouped yet) |

`n`/`N` are next/previous match only while a search query is active (`esc` clears it); otherwise `n`
is "new empty group". `g` is "group now" in watch mode when nothing is grouped yet, "merge"
otherwise. `pgup`/`pgdown` (also `space`, `ctrl+f`, `ctrl+b`), `home` and `end` move in the diff pane;
`1`–`9` pick a group by number in the target menus.
