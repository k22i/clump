# Diff loaders and prompt context

Every input form (`clump <rev>`, `clump A...B`, `--branch`, `--pr`, stdin, and watch's pushed diff)
produces the same `gitdiff.Loaded`: the patch, a one-line title, a short ref, and the prompt
context. The git invocation is pinned for every loader (ADR-0020) and `--pr` is the one loader that
shells out to `gh` instead of git, so clump never handles a GitHub token itself.

The context formats below are a contract, not a suggestion: they are what the model sees, so they
are stable per input kind and capped so one huge commit message cannot crowd out the diff.

## Loaders (`gitdiff`)

Every git call is `git -C <repo> <GitArgs> <subcommand> <DiffFlags> ...` where
`GitArgs = -c core.quotepath=off -c diff.noprefix=false -c diff.mnemonicPrefix=false
-c diff.srcPrefix=a/ -c diff.dstPrefix=b/ -c color.ui=never` pins the output against user
config and `DiffFlags = --no-color --no-ext-diff --unified=3 --find-renames --submodule`
matches what lazygit asks for. Both exist so a user's git config cannot change the diff hash
(ADR-0020).

| input | git | context for the prompt |
|---|---|---|
| `clump <rev>` | `show --format= <DiffFlags> --diff-merges=first-parent <rev>` | `COMMIT MESSAGE:` + `git log -1` message |
| `clump A...B` | `diff <DiffFlags> A...B` (merge base, like a PR) | `COMMITS IN THIS RANGE (newest first):` + up to 60 messages of `git log --no-merges A..B` |
| `clump --branch` | `A...B` with A = `origin/HEAD`'s target, else `main`, else `master`; B = `HEAD` | as above |
| `clump --pr 123` | `gh pr diff 123 --color=never` in the repo root | `PULL REQUEST: <title>` + body + `COMMITS IN THIS PULL REQUEST (oldest first):` + the `messageHeadline`s of `gh pr view --json number,title,body,commits` |
| `clump --pr OWNER/REPO#123` / URL | the same calls with `--repo OWNER/REPO`, no clone needed | as above |
| stdin | the patch as given | its `commit` header, if any |
| watch range / diffing mode | `diff <DiffFlags> X Y` (two-dot, what lazygit shows) | messages of `X..Y` |

Context is capped at 6000 bytes (`[... truncated]`). **Merge commits** are always diffed
against the first parent. The parser never accepts `diff --cc`: it returns an error wrapping
`gitdiff.ErrCombined`, and watch re-loads such a commit with `LoadRev`. A clean merge's plain
`git show` has a `--stat` block but no patch at all; watch treats a `git show` of a commit with a
`Merge:` header and no files the same way (ADR-0019).

`--pr` is the one loader that does not run git: `runGh` (github.go) shells out to the `gh` CLI
for both the patch and the metadata, so clump never handles a GitHub token itself — gh's login
is the credential, `doctor` reports whether the binary is on PATH, and a missing gh is an error
naming the install URL. The spec grammar accepts `123`, `#123`, `OWNER/REPO#123` and pull request
URLs (optionally with `.diff`/`.patch` suffix); a number-only spec resolves against the
repository gh is run in, so it needs a clone — the `--repo`-named forms do not. gh lists a pull
request's commits oldest first, so the context keeps that order (a review reads a PR top to
bottom; the git range loaders stay newest first).
