# Edit operations are pure, and group IDs never change meaning

Every edit op (`Move`, `MoveToNew`, `NewGroup`, `Rename`, `Delete`, `Reorder`, `Merge`) is pure: it
returns a new `Grouping` or its input unchanged with an error, and never mutates its argument,
including the `Hunks` slices it shares. A result is `Check`-clean whenever the input was.

Two invariants follow and are relied on elsewhere:

- Group IDs are stable for the life of a grouping. Edits never renumber existing groups, and
  `NextGroupID` never reuses a number. Review comments and folds are keyed by ID (ADR-0017), so a
  renumber would move a user's comments to another group.
- Empty groups are legal (a fresh `n` group), except `ungrouped`, which is removed when empty and
  created at the end when needed.

The undo/redo stack is snapshot-based (`History`) rather than op-based for the same reason
purity is required: a snapshot is only safe to restore if no two states share a backing array.

## The operations

```go
type Group    struct { ID, Title, Description string; Hunks []string }   // ID "g1".. or "ungrouped"
type Grouping struct { Groups []Group }
```

Pure operations; each returns a new `Grouping` or `(input, error)`, and never mutates its input
:

| op | rule |
|---|---|
| `Move(g, hunks, to)` | `hunks` non-empty (`ErrNoHunks`), each in some group (`ErrNoHunk`), duplicates ignored; `to` exists (`ErrNoGroup`) or is `ungrouped` (created at the end if absent). Hunks leave their groups and join `to`, which is re-sorted. Source groups may become empty and stay, except `ungrouped`, which is removed when empty. Moving hunks already in `to` is a no-op. |
| `MoveToNew(g, hunks, title)` | like Move into a new group `NextGroupID(g)` appended at the end; trimmed title must be non-empty (`ErrEmptyTitle`). |
| `NewGroup(g, title)` | appends an empty group `NextGroupID(g)`; `ErrEmptyTitle`. |
| `Rename(g, id, title, desc)` | `ErrNoGroup`; `ErrEmptyTitle`; `ErrUngroupedOp` for `ungrouped`. Title/desc trimmed. |
| `Delete(g, id)` | `ErrNoGroup`; `ErrUngroupedOp` for `ungrouped`. Its hunks go to `ungrouped` (created at the end if absent; nothing is created for an empty group). |
| `Reorder(g, id, delta)` | `ErrNoGroup`; new index `clamp(i+delta, 0, len-1)`; others keep their order. |
| `Merge(g, src, dst)` | `ErrNoGroup` for either; `ErrSameGroup`; src's hunks join dst (sorted), src removed; dst keeps its title. Merging `ungrouped` away is allowed, and so is merging into it. |
| `NextGroupID(g)` | `g<max N + 1>` over IDs `g<digits>`. IDs are never reused while a group holds them and never renumbered. |

Empty groups are allowed (a fresh `n` group), except `ungrouped`.

`History{past, cur, future}` is snapshot-based and unlimited: `Do(next)` pushes `cur` (no-op
if `Equal`), clears `future`; `Undo`/`Redo` move one step; `Reset` (after a regroup) clears
both stacks. `Apply(op)` runs an op and `Do`es its result.
