# Near-identical hunks collapse into one catalog unit

Non-pseudo hunks are near-identical when their files share directory, extension and status and they
have the same `+a -d`. A set of at least 6 such hunks from at least 6 distinct files becomes one
unit `cK`, and the model assigns `cK` as a whole (`Validate` expands it through
`ValidateOpts.Aliases`).

This targets the 45-translation-file branch from `spike/B-grouping.md`: without it the catalog
alone blew the budget and the model invented descriptions for files it had only seen headers of.
The thresholds (6/6) are what kept single large edits and small clusters readable one by one.
`CollapseMin < 0` disables collapsing, `0` means the default.
