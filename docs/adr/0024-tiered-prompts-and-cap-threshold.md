# Tiered prompts, and the 4-line cap threshold for tier 2

`group.Build` picks the cheapest strategy that fits the budget:

- **Tier 0**: the whole prompt fits — send it untruncated.
- **Tier 1**: binary-search the largest per-hunk body cap that fits. The catalog and every `@@`
  header are never cut; cut bodies end with `[... N more lines]`.
- **Tier 2** when the best cap would be below 4 lines, or the catalog alone exceeds the budget:
  per-file summary calls (packed into chunks that fit), then one grouping call over the full
  catalog plus one summary per file.

The cap-4 threshold comes from the spike: header-only grouping stays structurally valid but
descriptions start to be invented, so truncation is abandoned before it reaches that point and
summaries are paid for instead. Summaries are cached per file (`SummaryKey`), so regrouping a
branch after one more commit only re-summarises the files that changed.
