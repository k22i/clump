# Socket path length fallback

Unix socket paths are limited to 104 bytes on macOS (108 on Linux). `SocketPath` builds
`SocketDir()/<sha256(repo root)[:16]>.sock`; when that would exceed the limit it falls back to
`/tmp/clump-<uid>/`.

The fallback directory is shared between users, so `Listen` refuses a socket directory that is a
symlink or that another user owns, and a leftover socket file (a crashed watch) is replaced only
after dialling it fails. Chosen over a hashed shorter path in a per-user XDG directory because
`$XDG_RUNTIME_DIR` may itself be long, and over a fixed `/tmp/clump/<hash>` because that gives no
per-user isolation.
