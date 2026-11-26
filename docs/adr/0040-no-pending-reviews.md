# `clump submit` posts finished reviews, not pending ones

`clump submit` posts one `POST /repos/{owner}/{repo}/pulls/{n}/reviews` per review, with event
`COMMENT`, `APPROVE` or `REQUEST_CHANGES`.

A *pending* review — drafts the user keeps editing in the GitHub UI — needs GraphQL (`addPullRequest
ReviewThread`) and is not built. The REST call alone cannot create one, so `Draft` and `Leave` are
deliberately absent from `forge.Event` rather than half-implemented.

Consequences: the review is built and posted in one request (`gh api --input -`), so a failure is
all-or-nothing; the CLI confirms before posting (`--yes` skips, `--dry-run` prints the payload); the
TUI posts on `enter` from the event picker, only when the diff came from `--pr`.

Line comments carry `side: LEFT` for removed lines, using the old path of a rename — the path the PR
base shows. Group comments are a clump-only concept with no GitHub target, so they go in the review
body.

## Building the review

Posting a review (`internal/forge`, `clump submit`, `P` in the TUI) turns those anchors into
  one GitHub pull request review through the gh CLI: line comments inline (`side` LEFT is the
  old path of a rename, as the base shows it), file comments as `subject_type: file`, group
  comments — a clump-only concept — into the body. One `gh api --input -` request with event
  COMMENT, APPROVE or REQUEST_CHANGES (a pending review needs GraphQL and is not built); the CLI
  confirms before posting (`--yes` skips, `--dry-run` prints the payload), and the TUI posts on
  `enter` from the event picker only when the diff came from `--pr`.
