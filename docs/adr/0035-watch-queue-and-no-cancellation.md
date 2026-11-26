# Watch queues one call and does not cancel in-flight calls

At most one provider call is in flight per watch pane. A new selection that needs a call sets
`queued` (latest wins); a selection that needs no call clears it. A call that is already running is
never cancelled on selection change: it finishes, writes the cache, and its result is displayed only
if its hash is still the shown one, else discarded as stale.

Reason: a cancelled call pays for the tokens and writes nothing, whereas a finished one fills the
cache that the next visit to that commit will hit. With Haiku at ~5 s the waste is bounded to one
call (`spike/README.md` decision 2).

Messages older than the newest `sent_ns` are dropped, because lazygit's filter processes can finish
out of order.
