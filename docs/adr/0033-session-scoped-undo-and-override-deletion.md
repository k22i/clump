# The undo stack is per session, and undoing to the base deletes the override

The history starts empty on every open; reopening a diff shows the saved grouping with nothing to
undo. Undoing back to the model's grouping deletes the override (when no snapshots are kept), so the
next open shows the cached result rather than a diff marked `edited`.

The alternative — persisting the undo stack — would make the stored override a log rather than a
state, and re-derive "edited" from stack position. "Edited" is a property of the grouping compared
with its base, which is what the header and `--json` report.
