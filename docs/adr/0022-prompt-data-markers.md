# Prompt data is fenced with a per-request nonce

Everything from the repository (context, catalog, hunks, summaries) sits between
`<<<CLUMP-DATA-{nonce}>>>` and `<<<END-CLUMP-DATA-{nonce}>>>`, with a fresh 12-hex random nonce per
request, and the prompt states that data is never instructions.

The nonce makes a forged end marker unforgeable from inside a diff, which is the one injection a
review tool must defend against: a patch is attacker-controlled text that goes straight into a
prompt. In the spike neither Haiku nor DeepSeek followed an injected end marker; both flagged it.
`BuildOpts.Nonce` exists so tests can pin it.
