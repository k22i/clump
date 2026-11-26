# Estimates use calibrated bytes-per-token, and above the ceiling clump asks

Prompt size is estimated with the provider's calibrated `bytes_per_token` (ADR-0031), never
bytes/4: bytes/4 under-counts Claude by roughly 2× (`spike/B-grouping.md`), which would have sent
prompts over the ceiling silently.

The estimate (calls, tokens, cost) is shown before sending. Above `ceiling` (default 125000)
one-shot mode asks (`--yes` skips) and watch never auto-sends — it shows
`press g to group (~N tokens, ~$X)` instead. The ceiling sits a little above `budget` (120000) so a
prompt that `Build` fitted to the budget never trips the ask-first gate by a rounding error.
