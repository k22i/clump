# Testing strategy

The tests are the specification clump relies on instead of comments (ADR-0039): every parser rule,
edit operation, validator repair and render invariant is pinned by a test or a golden file, and no
test touches a real network or model.

- Table tests over every fixture in `internal/gitdiff/testdata` (lazygit captures from the
  spike, golden git fixtures from `scripts/mkfixture.sh`, edge cases); a test that every hunk's
  body matches its `@@` counts; `*.show.txt` and `*.lazygit.txt` of the same commit hash equal.
- Validator: repair table + a 2000-case property test. Edit ops: a ≥5000-step random sequence
  of ops, undos and redos with `Check` after every step and the seed logged.
- Providers against `httptest` fakes and `printf`/`cat` scripts; never a real network call or
  model in tests.
- TUI: pure render functions with golden files (colour off); scripted `Update` key sequences.
- IPC: a real listener in a temp dir. push: a benchmark through the real binary.
- E2E (Process step 4): `docs/e2e/README.md`.
- No comments: `go run ./scripts/lint-comments` fails on any comment in clump's packages, with a
  baseline that can only ratchet down (ADR-0039). Directives (`//go:build`, `//go:embed`,
  `//nolint`) and the vendored `internal/chromalexers` are exempt. CI runs it.
