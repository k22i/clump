# Generated files are skipped, not hidden

Files that `generated.Detector` marks keep their hunks in the grouping, in a folded `generated`
group, and are never sent to the model.

Skipping rather than hiding means the `Check` invariant (every hunk in exactly one group) still
holds, the user can unfold, read and move those hunks, and a group comment can still be written on
them. Only the model never sees their bodies.

A `generated/` directory pattern was rejected: it caught clump's own `internal/generated` package.
The built-in list deliberately leaves out files a tool writes but people still read for intent
(Django migrations, Jest snapshots, `go.mod`).
