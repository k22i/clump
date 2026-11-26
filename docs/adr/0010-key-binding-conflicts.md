# Key bindings: how `n`/`N` and `g` are shared

Two keys carry two meanings, resolved by state:

- `n`/`N` are next/previous search match **only while a search query is active**; with no query,
  `n` is "new group" (and `N` unused as a group action).
- `g` is "group now" in watch when nothing is grouped yet; otherwise it is "merge groups".

Both are collision-driven: `n` is the natural "new" and also vim's "next", and watch needs a
send-now key without stealing `g`'s merge meaning from one-shot mode. Rejected: a modal keymap,
which would have made the footer's key hints state-dependent in a second way.
