# One grouping pipeline for every mode

One-shot mode, watch and the `--json`/`--plain`/`--dry-run` outputs all run the same pipeline in the
same order, so a grouping written by one is found by the others (the cache key is the diff hash,
ADR-0029), and `--dry-run` differs only in stopping before the calls.

One pipeline for every mode:

1. `diffHash = Diff.Hash()`; `key = cache.Key(diffHash, provider, modelID, maxGroups)`.
2. Override for `diffHash`? -> show it (`edited`), no call.
3. Cache hit (`key`) -> show it (`cached`), no call. A hit is one file read.
4. `group.Build` -> `Plan` and `Estimate{tier, calls, tokens, cost, over ceiling}`.
5. Over the ceiling -> one-shot asks (`--yes` skips), watch shows `press g`.
6. Run: tier 1 one call; tier 2 the uncached summary calls (sequentially, one in flight) then
   the grouping call. `Validate`, write the cache entry, calibrate bytes per token.

`--dry-run` prints exactly the prompt(s) that would be sent (tier 2: the summary prompts, then
the grouping prompt with `(summary pending)` placeholders) and the estimate, and sends nothing.
