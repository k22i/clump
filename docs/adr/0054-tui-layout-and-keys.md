# TUI layout and key map

The layout exists to keep chrome and content apart: rules above and below the body, a list sized to
its content, prose indented off the title column so an explanation does not read as more diff, and a
header that is dim except the subject. The footer drops hints whole rather than cutting them, so the
narrowest terminal still shows `? keys`.

The key map below is the contract shared by `HelpSections`, the README table and the help overlay;
adding a key means adding it in all three (ADR-0010 covers the two collisions).

Layout: the body sits between two dim rules, one under the header and one above the key hints,
so chrome and content never run together. Group list left (title, `+a −d`, hunk count, markers),
right pane = selected group's title, its counts, its explanation (indented two cells under a blank
line, so prose reads as commentary and not as more diff), then its hunks grouped by file. Side by
side when the width is at least 100 columns, stacked (inline) below. Each pane keeps a cell of
padding off the separator and the screen edge. The list is sized to its longest title and counts
(capped at 48 cells and 45% of the width), so titles are not cut for no reason; `titleGap` cells
always stay free before the counts, and marks are left out of the measurement so marking a hunk
does not resize the list under the diff. Header: `clump`, the subject in bold, short ref, provider,
`cached`/`edited`, `N/M reviewed` and any state — all dim except the subject, so the eye lands on
what is being reviewed; watch adds `auto` and `dimmed`. The footer is the notices bar, or the key
hints as `key description` pairs (keys in `th.Key`, descriptions dim): a hint that does not fit is
dropped whole rather than cut mid-word, so the narrowest terminal still shows `? keys`.

Async flow: the UI opens at once with a spinner; the raw file list (the `a` view) is browsable
while the groups load; `groupsReadyMsg` fills them in. Provider calls run in `tea.Cmd`s with a
context the model can cancel.

Keys: `j/k` groups, `⏎` focus diff pane (`esc` back), `]`/`[` next/prev group anywhere, `a`
grouped/all, `/` search, `n`/`N` next/prev match, `e` open `$EDITOR` at `+line file`, `y` copy
Markdown, `r` regroup, `m` provider, `?` help, `q` quit, `s` side-by-side/unified diff. Editing:
`x` mark hunk, `v` visual range,
`M` move marks, `n` new group (while a search query is active, `n`/`N` are next/previous match
instead; `esc` clears the query), `R` rename/explain, `d` delete, `J`/`K` reorder, `g` merge (one-shot) / group now
(watch, when nothing is grouped yet), `u` undo, `ctrl+r` redo. Watch: `A` auto-group toggle.

Mouse (`MouseModeCellMotion`, so clicks and the wheel arrive while the terminal keeps its own
text selection on shift): the wheel scrolls the window (`m.scroll`, `wheelStep` lines a notch)
instead of walking the cursor down the pane — the pane under the pointer, or the group list one
group per notch. The cursor never scrolls out of sight: a window scrolled past it pins it to the
nearest edge (`scrollDiff`/`pinDcur`), so the arrow keys carry on from there inside the window.
A left click puts the cursor on what is drawn there: the line at `m.scroll + row - diffRect.y`
in the diff pane, or the group at that row of the list's window (`listview.Window`). `diffRect`
and `listRect` mirror `renderBody`'s layout (ADR-0041), so the mapping holds in the wide and
stacked layouts both. Overlays swallow mouse events.

Review: `c` marks the file block under the diff cursor (or the selected group, from the list)
reviewed. A finished group moves the cursor to the next group that is not reviewed, wrapping
around. The Generated files group and empty groups are not counted in the header's `N/M reviewed`.
A file block is folded by default when it is generated or fully reviewed. `z` (block) and `Z` (pane)
flip that, remembered per `(group ID, file)` (`""` in the all view) until the block's reviewed
state changes. `C` comments the target under the cursor — a diff line (its side is the old
number for `-` lines, the new number otherwise, the old one when there is none), a file header,
or a group (the list, the group block) — through the one-line input overlay; `D` deletes the
comment row under the cursor. Comment rows are woven into the pane lines (a new `KindComment`),
marked `✎`, wrapped at the pane width, placed after their line, header or group block, and
searchable through `Gutter`. A group's comments carry the grouping in play: they are re-read
with it (open, override, regroup), and a group created after that fetches its key on first use., and reset on a new grouping. A folded header carries `BlockHunks`, so `x`, `e`,
`}`/`{` act on it. `#` toggles the line-number gutter (`line_numbers`, default on). `Line.Gutter`
excludes the gutter from search. `s` toggles the side-by-side layout (`side_by_side`, default off):
the pane's width splits into an old half (with the mark gutter and old numbers) and a new half
around one separator column; a context line fills both halves, the i-th `-` line of a run faces the
i-th `+` line, the excess of one side stands against blanks, and "\ No newline" markers render on
the side of the line they follow. A split row is one `Line` whose `OldLine`/`NewLine` name the pair
(`KindContext` unless one side is empty, then that side's kind), so cursor moves, marks, search and
review work on it unchanged; a hunk given less than 3 columns falls back to the unified layout.

Theme: `CLUMP_THEME` wins, else the terminal is queried before Bubble Tea starts; dark is the
fallback (ADR-0053).

Rex program status (only when `REX_SESSION` and `REX_BLOCK` are set), always `app=clump`:
`working` while grouping, `blocked kind=question` over the ceiling, `done` with the group count,
`error` with the redacted message. Emitted with `tea.Raw`.
