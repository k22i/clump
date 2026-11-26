# Unified-diff parser rules

A patch is hostile input: it carries colour, a `git show` preamble, `--stat` blocks, binary
payloads and C-quoted paths, and any of those can appear again inside a hunk body. The parser is
therefore ordered and total — each rule below exists because a naive version mis-parsed a real
lazygit capture — and it never guesses: an input it cannot read is an error, not a partial diff.

The rules were ported from `spike/groupcli/parse.go`, which parsed the 200 latest lazygit commits in
both plain and coloured form.

## Parser rules

Ported from `spike/groupcli/parse.go` (which parsed the 200 latest lazygit commits in both
plain and coloured form):

- Strip SGR (`ESC\[[0-9;]*m`) first. With lazygit's default `colorArg: always` that is the only
  escape sequence in the input (A-lazygit.md), and the stripped text is byte-identical to
  `colorArg: never`.
- A preamble before the first `diff --git` (commit header, `Merge:`, Author/Date, indented
  message, `---`, the `--stat` block) is read for `commit <sha>` and the message only.
- Inside a hunk the `@@` counts decide what a line is, so a body line `--- old rule` is never a
  file header. `\ No newline at end of file` belongs to the hunk.
- Paths: `diff --git` line split (quoted, equal halves for spaces, else ` b/`), overridden by
  `rename from/to`, `copy from/to`, `---`/`+++` (trailing TAB trimmed; git adds one after paths
  with spaces). Quoted paths are C-unquoted (`"caf\303\251.txt"` -> `café.txt`).
- Status: `A` new file, `D` deleted, `R` rename, `C` copy, `T` mode only, else `M`.
- Binary: `Binary files ... differ` or `GIT binary patch` (payload skipped).
- Files with no text hunk (binary, pure rename/copy, mode-only, empty new/deleted file) get one
  **pseudo-hunk** whose header describes it, e.g. `(binary new file)`, `(rename)`,
  `(mode change) (mode 100644 -> 100755)` (ADR-0021).
- Hunk IDs `h1..hN` in file order across the diff.
