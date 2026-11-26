# Watch uses the diff lazygit pushed

`clump watch` groups the patch that `clump push` received on stdin instead of re-running `git`.

lazygit has already run exactly the git command it wants rendered, with its own flags and its own
selection semantics; re-deriving it would race the panel and could disagree with what the user is
looking at. Consequences:

- The diff arrives mid-flight, so anything the TUI needs beyond the patch (title, prompt context,
  a stash's sha) is resolved from git afterwards — see ADR-0013.
- A combined merge diff (`diff --cc`) cannot be parsed by `gitdiff` (ADR-0019), so watch reloads
  that commit with `LoadRev`.
- Dedupe is by diff hash, because lazygit re-sends the same patch every ~10 s.
