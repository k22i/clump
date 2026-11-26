# Review comments are content-anchored, like reviewed marks

A comment attaches to a line, a file or a group, each keyed by content:

```
line   sha256("line"  \0 ReviewKey \0 side \0 lineNo)
file   sha256("file"  \0 DisplayPath \0 ReviewKeys in file order)
group  sha256("group" \0 diffHash \0 groupID)
```

tuicr-style comments were requested; the question they raise is what they attach to, and clump's
existing persistence model already answered it for reviewed marks (ADR-0018). Consequences:

- Line and file comments follow a change across diffs, regroupings, provider switches and the
  commit-versus-range view, and disappear when the text changes.
- Group comments ride the grouping in play (cached or override), so a regroup that renumbers groups
  hides them — a group comment is about a grouping, not about a diff.
- Storage mirrors `reviewed/`: 16 shards, 180-day TTL, atomic writes, several comments per anchor
  with 12-hex ids (`sha256(key \0 at \0 text)`).
- The TUI weaves `✎` rows into the pane after their target as a new `KindComment`, with the marker
  as `Gutter` so search still sees the text.
