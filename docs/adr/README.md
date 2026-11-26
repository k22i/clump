# Decision records

One decision per file, numbered in the order they were recorded. This directory is clump's only
design documentation: `DESIGN.md` was retired into it (see the note at the end), so an ADR here may
carry the contract it decided as well as the reasoning behind it.

A decision belongs here when it is hard to reverse, surprising without context, and the result of a
real trade-off. Behaviour that is fully expressed by a test stays in the test.

## Foundations

| ADR | Decision |
|---|---|
| [0001](0001-go-version-1-27.md) | Go 1.27 in `go.mod` |
| [0045](0045-package-graph.md) | Package layout and the import graph |
| [0046](0046-build-release-and-update.md) | Distribution: one static binary, prebuilt releases, self-update |
| [0039](0039-no-comments-in-source.md) | No comments in clump's Go source |

## Diff input

| ADR | Decision |
|---|---|
| [0047](0047-diff-loaders.md) | Diff loaders and prompt context |
| [0048](0048-parser-rules.md) | Unified-diff parser rules |
| [0019](0019-merges-diffed-against-first-parent.md) | Merge commits are diffed against their first parent |
| [0021](0021-pseudo-hunks.md) | Files without text hunks get one pseudo-hunk |
| [0020](0020-canonical-form-and-diff-hash.md) | Canonical form makes clump's and lazygit's diff the same |
| [0006](0006-copies-only-when-git-reports-them.md) | Copies are only detected when git reports them |
| [0044](0044-git-and-gh-environment.md) | git and gh run pinned and never wait on a terminal |

## Grouping

| ADR | Decision |
|---|---|
| [0049](0049-catalog-and-prompt-contract.md) | Catalog and prompt contract |
| [0023](0023-near-identical-hunk-collapsing.md) | Near-identical hunks collapse into one catalog unit |
| [0022](0022-prompt-data-markers.md) | Prompt data is fenced with a per-request nonce |
| [0050](0050-validation-and-invariants.md) | Validation, repair and the grouping invariants |
| [0024](0024-tiered-prompts-and-cap-threshold.md) | Tiered prompts, and the 4-line cap threshold for tier 2 |
| [0025](0025-token-estimates-and-ceiling.md) | Estimates use calibrated bytes-per-token; above the ceiling clump asks |
| [0016](0016-generated-files-skipped-not-hidden.md) | Generated files are skipped, not hidden |
| [0032](0032-edit-ops-pure-and-ids-stable.md) | Edit operations are pure, and group IDs never change meaning |
| [0033](0033-session-scoped-undo-and-override-deletion.md) | The undo stack is per session; undoing to the base deletes the override |
| [0034](0034-regroup-asks-discard-or-snapshot.md) | Regrouping with edits asks what to do with them |

## Providers and engine

| ADR | Decision |
|---|---|
| [0051](0051-grouping-pipeline.md) | One grouping pipeline for every mode |
| [0052](0052-provider-configuration.md) | Provider configuration, defaults and prices |
| [0027](0027-agent-cli-isolation.md) | Agent CLIs run isolated from the repo |
| [0026](0026-anthropic-without-cache-control.md) | The Anthropic provider sends no `cache_control` |
| [0042](0042-openai-response-format-retry.md) | One retry when a server rejects `response_format` |
| [0028](0028-secrets-never-in-config.md) | Secrets never live in config, and errors are redacted |
| [0029](0029-cache-key-and-layout.md) | Cache key and layout |
| [0030](0030-cache-durability-rules.md) | Which cached data is precious |
| [0031](0031-calibration-of-bytes-per-token.md) | Calibrating bytes per token |
| [0003](0003-overrides-live-under-cache-root.md) | Overrides live under the cache root |

## Reviewing

| ADR | Decision |
|---|---|
| [0054](0054-tui-layout-and-keys.md) | TUI layout and key map |
| [0053](0053-terminal-theme-detection.md) | The terminal's colour scheme is asked for, once, before the TUI starts |
| [0043](0043-render-invariants.md) | Rendering never changes the visible text |
| [0041](0041-mouse-geometry-mirrors-the-layout.md) | Mouse geometry mirrors the render layout |
| [0007](0007-tui-split-into-sub-packages.md) | TUI split into render sub-packages |
| [0008](0008-no-bubbles-textinput.md) | No `bubbles/textinput` |
| [0010](0010-key-binding-conflicts.md) | Key bindings: how `n`/`N` and `g` are shared |
| [0009](0009-report-package-owns-outputs.md) | `internal/report` owns the Markdown and JSON outputs |
| [0018](0018-reviewed-per-hunk-content.md) | Reviewed is per hunk content, toggled per file or group |
| [0017](0017-review-comments-content-anchored.md) | Review comments are content-anchored, like reviewed marks |
| [0040](0040-no-pending-reviews.md) | `clump submit` posts finished reviews, not pending ones |

## lazygit, watch and Rex

| ADR | Decision |
|---|---|
| [0004](0004-watch-uses-pushed-diff.md) | Watch uses the diff lazygit pushed |
| [0005](0005-push-reads-no-config.md) | `clump push` reads no config |
| [0055](0055-push-contract.md) | `clump push`: the diff renderer contract |
| [0011](0011-push-startup-budget.md) | `push` start-up budget: palettes in code, lexers vendored and lazy |
| [0036](0036-ipc-socket-and-message-limits.md) | IPC: one JSON line per connection, and hard limits |
| [0012](0012-socket-path-length-fallback.md) | Socket path length fallback |
| [0056](0056-watch-selection-and-state.md) | Watch: selection rules and the state machine |
| [0035](0035-watch-queue-and-no-cancellation.md) | Watch queues one call and does not cancel in-flight calls |
| [0013](0013-watch-resolves-titles-itself.md) | Watch resolves titles and context itself |
| [0037](0037-lazygit-install-line-based.md) | `clump lazygit install` edits config.yml line by line |
| [0038](0038-colorarg-stays-always.md) | `colorArg` stays at `always` |
| [0015](0015-rex-module-configuration.md) | Rex module configuration |
| [0002](0002-own-osc-7501-encoder.md) | Own OSC 7501 encoder, not Bubble Tea's `View.ProgramStatus` |
| [0014](0014-own-picker-fallback.md) | `clump --pick` shows clump's own picker |

## Testing

| ADR | Decision |
|---|---|
| [0057](0057-testing-strategy.md) | Testing strategy |

## Why there is no DESIGN.md

`DESIGN.md` was a single 625-line document that mixed three things: decisions (with their
reasoning), contracts (formats, tables, key maps) and status notes from the build. It could not be
kept honest — a comment-free codebase (ADR-0039) moved everything a future reader needs into one
place, and one place that everything depends on is exactly what ADRs are for, one file at a time.

It was retired into the files above: every section has a home, every cross-reference points at an
ADR number rather than a section, and nothing is left behind. The old file is in git history
(`git log --diff-filter=D -- DESIGN.md`).
