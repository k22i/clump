# Files without text hunks get one pseudo-hunk

A binary file, a pure rename or copy, a mode-only change, or an empty new/deleted file has no `@@`
hunk. Instead of representing it as a file with zero hunks, the parser gives it one pseudo-hunk
whose header describes it (`(binary new file)`, `(rename)`, `(mode change 100644 -> 100755)`).

Reason: the grouping, edit and history models all key on hunk IDs, and `Check` requires every hunk
of a diff to be in exactly one group. A file with no hunks could not be grouped, moved, reviewed or
commented on, and a rename would vanish from the review. The pseudo-hunk's `ReviewKey` hashes its
header, so its reviewed mark still resets when the change changes.
