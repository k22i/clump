# IPC: one JSON line per connection, and hard limits

`push` and `watch` speak one JSON line per connection over a per-repo Unix socket
(`{v, sent_ns, repo_root, git_argv, rex_block, diff}`).

- Socket directory 0700, socket 0600. The umask is set around the bind so the socket is never
  briefly world-writable, and `Close` only unlinks the file while it is still the one this listener
  created (`os.SameFile` compares device and inode) — a newer watch may already own the path.
- A message is capped at 64 MiB, and the reader asks for cap+1 bytes so an over-limit message is
  detectable rather than silently truncated.
- `push` treats the connection as fire-and-forget: 50 ms to dial, 100 ms to write, then exit 0
  silently. A missing, stale or busy socket is not an error for a diff renderer.
- `watch` removes a stale socket (dial fails) before listening, and refuses to start (`ErrInUse`)
  when a live watch answers.
- Connections are served one at a time in accept order, which keeps the state machine single-threaded.
