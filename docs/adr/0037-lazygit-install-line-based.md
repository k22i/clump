# `clump lazygit install` edits config.yml line by line

`lazygit.Install` prints the change, and with `--write` applies it after a y/N and a backup
(`config.yml.clump-backup-<ts>`). The editing rules are line-based: find the `git:` block, then its
`diffRenderers:` child, and rewrite or insert one list item.

No YAML library was used deliberately. A round-trip through a parser would rewrite the user's whole
file — reordering keys, dropping comments, reflowing the parts clump does not understand — for a
five-line edit. The cost is that anything unusual (flow-style lists, anchors or aliases, the legacy
`git.paging.pager`) is refused with the snippet and instructions to paste, rather than edited.

If an entry already contains `clump push`, clump reports "already installed" and changes nothing,
so the command is idempotent.

## The editing rules

`clump lazygit install` prints the change; `--write` applies it after a y/N and a backup
(`config.yml.clump-backup-<ts>`). Rules (line-based; no YAML library — ADR-0037):

1. No config file -> the snippet is the whole file.
2. No top-level `git:` key -> append the snippet.
3. `git:` without `diffRenderers:` -> insert `diffRenderers:` with the clump entry as the first
   child of `git:` (using the block's indent).
4. `diffRenderers:` in block style: if an entry's command contains `clump push`, report
   "already installed" and change nothing. Else rewrite the **first** entry's `command:` to
   `clump push --then '<old command>'` when it is a stdinFilter (no `type:` or
   `type: stdinFilter`); otherwise insert the clump entry first.
5. Anything else (flow-style lists, anchors, the legacy `git.paging.pager`, a `colorArg: never`
   on the entry it would wrap) -> print the snippet and instructions, never write.

`colorArg` stays at its default (`always`) (ADR-0038).
