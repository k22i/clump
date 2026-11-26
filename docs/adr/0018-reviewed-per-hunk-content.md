# Reviewed is per hunk content, toggled per file or group

A file or group is the unit the user checks off, as on GitHub, but marks are stored per hunk by
content (`Hunk.ReviewKey() = sha256(DisplayPath \0 body lines)`, no line numbers).

Content keying is what lets a mark survive regrouping, carry from a commit to the branch range that
contains it, and reset when the hunk's text changes — the same property review comments need
(ADR-0017). Storage is 16 shards under `reviewed/`, 180-day TTL, atomic writes.

`c` is the key because `v` (GitHub's "viewed") is visual mode here.
