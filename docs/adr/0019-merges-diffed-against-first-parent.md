# Merge commits are diffed against their first parent

`LoadRev` runs `git show --format= --diff-merges=first-parent <rev>`, and `Parse` rejects a
combined merge diff (`diff --cc` / `diff --combined`) with an error wrapping
`gitdiff.ErrCombined`.

A combined diff is not a patch — it has several parents' worth of prefixes per line and would need
a different line model, and lazygit never shows one for review. Callers that hit `ErrCombined`
reload the commit with `LoadRev`, which reduces it to the first-parent patch. A clean merge's plain
`git show` has a `--stat` block and no patch at all; watch treats a `Merge:` header with no files
the same way.
