# Watch resolves titles and context itself

`gitdiff` gained `CommitMessage`, `RangeContext`, `CommitContext` and `ShortRev` so watch can
title and contextualise a pushed diff without re-running the diff that produced it.

The diff must come from lazygit (ADR-0004), but the header still needs a subject line and the
prompt still needs commit messages. These four calls fetch only that metadata — a title is not a
reason to re-diff, and re-diffing would risk a hash mismatch that hides the cache entry.
