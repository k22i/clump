# `push` start-up budget: palettes in code, lexers vendored and lazy

Status: accepted. Supersedes the plain use of `chroma/v2/styles` and `chroma/v2/lexers`.

Every package `init` runs before `main`, and the binary links the TUI. `chroma/v2/styles`
(3.6 ms) and `chroma/v2/lexers` (3.5 ms, plus three analyser `init`s that force the whole
registry) made `clump push` 11.5 ms slower than `cat`, against a budget of 5 ms p50 (ADR-0005).

Fixes, all in place:

- The two syntax palettes are built in code (`diffview.palette`, GitHub light and dark colours).
- `chroma/v2/lexers` is vendored as `internal/chromalexers` (MIT, LICENSE kept) with the registry
  built on first use, the analysers run at that point, and its Rule/Token literals keyed for
  `go vet`.
- No new module: `github.com/dlclark/regexp2/v2`, already chroma's dependency, became a direct
  requirement because the vendored code imports it.

Result: `push` adds 4.7 ms p50 over `cat`, measured by `TestPushLatency` (asserted with
`CLUMP_PUSH_LATENCY=1`, since `go test ./...` runs packages in parallel and the timing is noisy
there).

## How CI enforces the budget

Every package `init` runs before `main`, so the budget is spent before `push` is even reached, and a
wall-clock comparison against `cat` is too noisy to gate a shared CI runner with. CI gates the cause
instead: `TestPushInitBudget` runs the real binary with `GODEBUG=inittrace=1` and sums what every
linked package does before `main`. The budget is 12 ms of init clock, 5 ms in any one package and
2 MiB of init allocations (measured: ~1-3 ms and ~660 KiB across 110 packages, the largest single
`init` being `internal/chromalexers` at 0.3 ms).

The allocation bound is the one that matters, because it is deterministic: `chroma/v2/styles` alone
allocates 1.9 MiB at `init` (3.4 ms) and `chroma/v2/lexers` 2.7 MiB (16 ms), so re-adding either
fails the test on any machine. Anything that needs a registry at start-up has to go behind a lazy
`sync.Once`, as `internal/chromalexers` already does. The wall-clock p50 comparison stays a local
measurement: it says how much of the budget the current machine really sees, and it is the number
this ADR quotes.


Because `internal/chromalexers` is vendored third-party code, the comment ban (ADR-0039) does not
apply to it.
