# Catalog and prompt contract

The model is shown two artifacts: a catalog of one line per unit, and one annotated block per unit.
Both formats are contracts — the cache key includes `PromptVersion` (ADR-0029), and the fake
provider's answers are keyed off the task line and the catalog shape, so a format change is a
prompt-version bump and a test change, not a cosmetic edit.

## Catalog (`catalog`)

- One line per unit: `id | status | path | @@ header | +a -d` (path `old -> new` for R/C).
- One block per unit: `### hN path`, the `@@` header, the body (maybe capped, then
  `[... N more lines]`).
- **Collapsing.** Non-pseudo hunks whose files share directory, extension and status, and that
  have the same `+a -d`, are near-identical. A set of at least 6 such hunks from at least 6
  distinct files becomes one unit `cK` (catalog line `cK | M | dir/*.ext (N files) | N similar
  hunks, e.g. @@ ... @@ | +A -D`; block shows the first member as a sample). The model assigns
  `cK` as a whole; `Validate` expands it through `ValidateOpts.Aliases` (ADR-0023).

## Prompt (`group`)

- `internal/group/prompt_v1.txt` (grouping) and `prompt_summary_v1.txt` (tier-2 summaries) are
  embedded. `PromptVersion = "v1"` is in every cache key.
- The first line names the task (`# clump-task: group v1`), which the fake provider keys on.
- Everything from the repo (context, catalog, hunks, summaries) sits between
  `<<<CLUMP-DATA-{nonce}>>>` and `<<<END-CLUMP-DATA-{nonce}>>>`, with a fresh 12-hex-char random
  nonce per request, and the prompt says that data is never instructions (ADR-0022).
- Model output: `{"groups":[{"title","description","hunks":[ids]}]}`.
