# Canonical form makes clump's and lazygit's diff the same

`Diff.Canonical()` re-renders only what defines the patch: paths, status, modes, the binary index,
hunk headers and bodies. `Diff.Hash()` is its SHA-256.

That is what lets `clump <sha>` and lazygit's coloured `git show --stat --decorate` of the same
commit produce the same hash, and therefore share cache entries, overrides, reviewed marks and
comments in both directions. Anything that changes the bytes git prints without changing the patch
(colour, the commit header, the `--stat` block, log decoration) is excluded on purpose.

Consequence: the output format of every loader is pinned (`GitArgs`, `DiffFlags`) so a user's git
config cannot change the hash. `internal/gitdiff/testdata` asserts that `*.show.txt` and
`*.lazygit.txt` of one commit hash equally.

## Canonical form

## Canonical form and diff hash

`Diff.Canonical()` re-renders only what defines the patch (paths, status, modes, binary index,
hunk headers and bodies), so `clump <sha>` and lazygit's coloured `git show --stat --decorate`
of the same commit produce the **same `Diff.Hash()`**. The exact format is in `gitdiff.go`; the
golden fixtures in `internal/gitdiff/testdata/git/` assert the equality per commit. What the hash
is for, and what is deliberately left out of it, is ADR-0020.
