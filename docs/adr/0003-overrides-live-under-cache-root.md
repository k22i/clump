# Overrides live under the cache root

The user's edited groupings (`overrides/<hash[:2]>/<hash>.json`) sit under the cache root
(`$XDG_CACHE_HOME/clump`), in their own directory, separate from `results/` and `summaries/`.

They are user work, not a cache: nothing evicts them and a corrupt one is an error rather than a
miss (see ADR-0030). Keeping them under the cache root is what the brief specified; putting them
in `$XDG_CONFIG_HOME` was rejected as a second lookup path for no gain.

Consequence: `clump --no-cache` must never bypass them — a user's edits are not a cache.

## Shape on disk

## Overrides

Saved after every edit, keyed by diff hash, separate from model results:

```json
{"version":1,"diff_hash":"…","base_cache_key":"…","edited_at":"…",
 "grouping":{"groups":[…]},"snapshots":[{"name":"…","at":"…","grouping":{…}}]}
```

- Opening a diff with an override shows the override, marked `edited`, without a model call
  (even when there is no cached result). An override that fails `Check` is ignored with a
  notice (it cannot happen for a matching hash, but the file is user-editable).
- The undo stack is per session. Undoing back to the model's grouping deletes the override
  (when no snapshots are kept), so the next open shows the cached result, not "edited" (ADR-0033).
- `r` regroup with edits asks: `d` discard edits, `s` keep them as a snapshot
  (`before regroup <time>`) and regroup, `esc` cancel. After a regroup the override's
  `grouping` is empty (no active edits) and only its `snapshots` remain (ADR-0034).
- `--json` and `y`/`--plain` print the effective grouping (override if any).
