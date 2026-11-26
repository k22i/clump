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

Because `internal/chromalexers` is vendored third-party code, the comment ban (ADR-0039) does not
apply to it.
