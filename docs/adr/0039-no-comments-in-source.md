# No comments in clump's Go source

Status: accepted, ratcheted (see `scripts/lint-comments`).

Comments are banned in clump's own Go packages. Knowledge lives in the places that can be checked
and kept true instead:

- **Decisions** ("why this way, what was rejected") — these ADRs.
- **Behaviour** — the tests that assert it, and the names of the functions that implement it.
- **Contracts** — the ADRs that state one outright: package graph (ADR-0045), build and update
  (ADR-0046), loaders (ADR-0047), parser rules (ADR-0048), catalog and prompt (ADR-0049),
  validation and invariants (ADR-0050), the pipeline (ADR-0051), provider defaults (ADR-0052),
  theme detection (ADR-0053), TUI layout and keys (ADR-0054), `push` (ADR-0055), watch selection and state (ADR-0056), testing
  (ADR-0057).

The reason is that a comment is the only part of a file no compiler, test or linter checks. In this
repository they had drifted into three failure modes that a deletion pass found across all 148
files: restating the line below it (`// addComment resolves the anchor and stores the comment`),
recording a hack instead of fixing it (`// best effort` over a swallowed error), and pointing at
code or design sections that no longer exist.

## Exemptions

- **Compiler and tool directives** — `//go:build`, `//go:embed`, `//go:generate`, `//line`,
  `//export`, and `//nolint`. These are code, not prose; deleting them changes behaviour.
- **Vendored third-party code** — `internal/chromalexers` (vendored chroma lexers, ADR-0011). Its
  comments are upstream's and are kept as the licence requires; re-syncing upstream would delete
  them anyway.
- **Generated files** — anything carrying a `Code generated … DO NOT EDIT.` marker.

## How the ban is enforced

`scripts/lint-comments` fails on any comment in clump's packages. Because the removal is not
finished, it runs with a **baseline** (`scripts/lint-comments/baseline.txt`) that grandfathers the
comments still present, per file:

- a file absent from the baseline may have no comments at all;
- a listed file may have at most its recorded count;
- `-update` may only lower a count, never raise one, and deletes a file's entry at zero.

So the number can only ratchet down: new comments fail immediately, and the debt shrinks as files
are cleaned. Deleting a comment without re-running `-update` is fine; adding one is not.
