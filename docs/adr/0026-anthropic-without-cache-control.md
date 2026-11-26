# The Anthropic provider sends no `cache_control`

`anthropicProvider` posts to `/v1/messages` with `anthropic-version: 2023-06-01`, no
`cache_control`, no sampling parameters and no `thinking` field.

Prompt caching was measured at about half the price of `claude -p`, which bills a 1-hour cache
write on every call (`spike/B-grouping.md`), and clump's prompts are one-shot: there is no second
request to read a cache back. That API version also rejects `temperature`/`top_p`/`top_k`, so
sending them would be a 400 — the request shape is pinned to what the pinned version accepts.
