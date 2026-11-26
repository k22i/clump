# Cache key and layout

```
results/<key[0:2]>/<key>.json        Entry
summaries/<key[0:2]>/<key>.json      Summary
overrides/<hash[0:2]>/<hash>.json    Override (ADR-0003)
calibration.json                     {"<provider>": bytes_per_token}
reviewed/<key[0]>.json               16 shards (ADR-0018)
comments/<key[0]>.json               16 shards (ADR-0017)
```

`Key = sha256(diffHash \0 provider \0 modelID \0 PromptVersion \0 max_groups)`, plus
`\0 skip=<generated.Signature>` when the diff has generated files (`KeyWith`; a diff without them
keeps the old key, so caches written before generated-file skipping stay valid). `modelID` is
`model`, else the argv, else `base_url`, so two command providers with the same model do not share
entries.

`PromptVersion` is bumped whenever `prompt_v1.txt`, `prompt_summary_v1.txt` or `Build`'s rendering
changes: a prompt change invalidates every cached answer without touching the diff hash.
