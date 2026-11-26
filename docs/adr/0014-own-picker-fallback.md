# `clump --pick` shows clump's own picker

`rex.ui.pick` returned `unsupported` with the current Rex app build, so `clump_pick` opens
clump's own picker (this branch vs default, 50 recent commits, type to filter) as the fallback,
in a floating layer.

The fallback is not a stub: it is the same picker `clump --pick` shows outside Rex, so the Rex path
is a fast path rather than the only path. It triggers on `unsupported`, `no_client`, `busy`,
`timeout` and `failed`.
