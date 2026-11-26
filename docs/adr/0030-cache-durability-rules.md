# Which cached data is precious

Cache files are not all equally valuable, and the store treats them in three tiers:

| data | unreadable or other version |
|---|---|
| `results/`, `summaries/` | a miss (nil, false, nil) |
| `reviewed/`, `comments/` | an empty shard |
| `overrides/` | an **error**, so edits are never silently lost |

Only the user's own edits get the loud treatment: losing one to a bad parse would be work lost, while
losing a cached grouping or a reviewed mark costs one provider call or one click. Unexpected I/O
errors on results are still returned, so a broken disk is not reported as a cache miss.

`--no-cache` (`NoCache`) makes result and summary reads miss while writes still happen, and never
bypasses overrides or calibration.
