# Caching, edits and review comments

## Caching and your edits

Results live under `$XDG_CACHE_HOME/clump` (else `~/.cache/clump`):

```
results/<key[0:2]>/<key>.json      one cached grouping
summaries/<key[0:2]>/<key>.json    one cached per-file summary
overrides/<hash[0:2]>/<hash>.json  your edits for one diff
reviewed/<k>.json                  hunks you marked reviewed, by content
comments/<k>.json                  your review comments, by their anchor
calibration.json                   bytes-per-token per provider
```

The key hashes the diff's canonical form, the provider, the model, the prompt version and
`max_groups`, so the same commit reached two ways — `clump <sha>` or lazygit's coloured `git show` —
hits the same entry. A hit is one file read and no provider call; in the e2e run a revisit showed
`cached` about 16 ms after the debounce. `--no-cache` and `r` make reads miss while writes still
happen, so a regroup refills the cache.

Your edits are saved as an override keyed by the diff hash, separate from model results: reopening
that diff shows them marked `edited` with no provider call, even without a cached result, and it
survives a restart (e2e: edit, close the pane, reopen, back to the commit, still `edited`). Undo is
per session; undoing back to the model's grouping deletes the override, so the next open shows the
cached result again.

Reviewed marks (`c`) are kept per hunk, keyed by the file path and the hunk's lines without line
numbers, not by diff. So a hunk you checked off while stepping through commits in watch mode is
already checked when you open the branch range (`clump --branch`). It stays checked after a
regroup, an edit, a provider switch or a rebase that only moves it, and is unchecked again as soon
as its text changes. Marks older than 180 days are dropped.

Review comments (`C`) are keyed by content the same way: a line comment by its hunk and line
(a side and a line number), a file comment by the file's path and the whole set of its hunks, a
group comment by the diff and the group's ID, so it stays while that grouping — a cached result or
your saved edits — is the one on screen, and a regroup that renumbers groups drops it from view
(the comment is kept in the store until it expires after 180 days). `D` deletes the comment under
the cursor. Comments show as `✎` rows under their line, file header or group block, are searchable
like the diff, and travel with the `y` summary, `--json` and `--plain` (each group's section lists
its line, file and group comments, numbered).

`r` with edits asks first: `d` discards them and regroups, `s` keeps them as a snapshot (`before
regroup <time>`) and regroups, `esc` cancels; after a regroup only the snapshots remain. `--json`
and `--plain` print the effective grouping, override included.

## `clump comments`: the store without the TUI

`clump comments` reads and writes them without the TUI, for scripts and coding agents:

```
clump comments [REV | A..B | A...B | --branch | --pr SPEC] [--json]
clump comments add --file PATH [--line N --side old|new] [--rev REV|--branch|--pr SPEC] TEXT
clump comments add --group ID [--rev REV|…] TEXT     # a saved grouping must exist
clump comments delete --id ID
```

The list numbers the comments one per line — `` `a.txt:2` (new side) — text `` — or prints them as
JSON with `--json`; `add` prints the added comment and its `id` (which `delete` takes); the list
also takes a patch on stdin, so `git show | clump comments` works. Group comments are listed via
the saved grouping (your edits, else the cached result); no `comments` command calls a provider.
So an agent review ends with `clump comments <ref> --json`, and the TUI, `y`, `--json` and the
CLI all read the same store.

## Posting a review

`clump submit` posts a pull request's stored comments back to GitHub as one review, through the
`gh` CLI (clump stores no tokens; the review is posted as whatever account `gh` is logged into):

```
clump submit --pr SPEC --event comment|approve|request-changes [--body TEXT] [--yes] [--dry-run]
```

Line comments land inline on their line, file comments on their file header, and group comments —
a clump concept GitHub has no place for — go into the review body (`--body`, if given, is its
summary). The whole review is one `gh api` request. Without `--yes` clump prints what it is about
to post and asks first; `--dry-run` prints the JSON payload and sends nothing.

In the TUI opened with `--pr`, `P` does the same: it shows what would be posted
("Post review of `<ref>`: 2 line comments, …") and the three events to pick from; `enter` posts,
`esc` cancels. A diff opened any other way answers `P` with a hint to use `--pr`.
