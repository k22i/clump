# AGENTS.md

clump is a Go 1.27 TUI CLI: one module, one static binary (`cmd/clump`), no cgo, no runtime
dependencies of its own. All packages live under `internal/` (plus `rex/` for the Rex Lua module).
Comments in this README-style file are fine; comments in Go source are not (below).

## Commands

```
go build ./...                              # must pass
go vet ./...                               # must pass
go test -race ./...                        # the full suite; no build tags needed
go run ./scripts/lint-comments             # comment ban (see Hard rules); CI runs this
```

`scripts/mkfixture.sh DIR [N]` builds a scratch repo full of parser edge cases; run
`scripts/mkgolden.sh` on that repo to regenerate `internal/gitdiff/testdata/git`. Prefer updating
a golden only after understanding which rule the mismatch comes from
(`docs/adr/0048-parser-rules.md`).

## Hard rules

- **Write no prose comments in Go source.** New comments in clump's own packages fail CI
  (ADR-0039). Directives (`//go:build`, `//go:embed`, `//nolint`), generated files, and the vendored
  `internal/chromalexers` are exempt. Put the "why" in a test name, a commit message, or an ADR —
  never in a comment. Never re-run `lint-comments -update` in a way that raises a baseline count.
- **Import directions are load-bearing.** `gitdiff`, `provider`, `ipc`, `lazygit` and `rex` import
  the standard library only (plus `golang.org/x/sys` for `ipc`/`rex`) — this is what keeps
  `clump push` free of config, provider and TUI setup. Render packages under `internal/tui/*`
  stay pure so they golden-test without a terminal. Before adding a cross-package import, read
  ADR-0045 (`docs/adr/0045-package-graph.md`); if the edge is not in its graph, stop and check.
- **No test makes a real network call or needs a model.** Providers test against `httptest`
  fakes and `printf`/`cat` scripts (ADR-0057). Never add a test that depends on credentials or
  network state.
- **Start-up latency of `clump push` is a budget** (ADR-0011): no work in `init()`, registries are
  built lazily (`internal/chromalexers` exists for this reason), palettes live in code.
- **Stray files:** `spike/` holds measurements this build rests on, and `docs/e2e/` holds TUI
  captures — neither is application source; don't "clean them up".

## Where knowledge lives

There is no DESIGN.md. `docs/adr/` — one decision per file, indexed in
`docs/adr/README.md` — is the repo's only design documentation, and ADRs may carry the contract
itself, not just the reasoning. Reach the index when you:

- touch a format or contract (diff parser, catalog and prompt, cache layout and key, IPC messages,
  key map, terminal themes) — find the owning ADR and change both together;
- need the reason behind a constraint you just hit — it is almost certainly decided already;
- make a hard-to-reverse, trade-off decision of your own — record it as a new numbered ADR and add
  it to `docs/adr/README.md`'s tables.

Behaviour belongs in tests, not prose: parser rules, edit ops, validator repair and render
invariants are each pinned by table, property or golden tests. If you must choose between a
comment and a test that asserts the fact, write the test.

## Testing notes

- Table tests cover every fixture in `internal/gitdiff/testdata`; renderers golden-test with
  colour off (ADR-0043: rendering must not change the visible text).
- The validator has a property test; edit ops run a seeded random sequence with `Check` after every
  step — reproducible from the logged seed.
- `CLUMP_PUSH_LATENCY=1` asserts the `push` latency budget through the real binary; off by
  default.
- TUI keyboard changes must clear ADR-0010's conflict rules (`n`/`N` and `g` are shared).

## Git

Conventional commits, scoped (`feat(tui): …`, `fix(config): …`); no attribution or co-authoring
trailers. CI (`.github/workflows/ci.yml`) is the gate for main: build, vet, lint-comments, test.
