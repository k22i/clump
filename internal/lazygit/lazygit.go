// Package lazygit classifies what lazygit is showing from the git command
// that ran `clump push` (its parent's argv), and prints or installs the
// lazygit config that wires `clump push` in as a diff renderer.
package lazygit

// Kind is what kind of lazygit selection produced a pushed diff.
type Kind string

// Selection kinds. See DESIGN.md "Watch: selection rules".
const (
	KindUnknown      Kind = "unknown"        // anything not below: dim
	KindCommit       Kind = "commit"         // git show <sha>
	KindRange        Kind = "range"          // git diff A^ B (range selection in the commits panel)
	KindStash        Kind = "stash"          // git stash show refs/stash@{N}
	KindDiffMode     Kind = "diffmode"       // git diff X Y (diffing mode, two-dot)
	KindFileInCommit Kind = "file-in-commit" // git diff A^ A -- <path>
	KindWorktree     Kind = "worktree"       // git diff [--cached] -- <path>, or --no-index
	KindLog          Kind = "log"            // git log --graph (branches panel)
)

// Selection is a classified push.
type Selection struct {
	Kind Kind `json:"kind"`
	// Commit is the sha for KindCommit and KindFileInCommit.
	Commit string `json:"commit,omitempty"`
	// From and To are the two revisions of KindRange (From ends in "^") and
	// KindDiffMode.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Stash is the ref of KindStash, e.g. "refs/stash@{0}".
	Stash string `json:"stash,omitempty"`
	// Paths are the pathspecs after "--" (KindFileInCommit, KindWorktree).
	Paths []string `json:"paths,omitempty"`
	// Cached is set for a staged KindWorktree diff.
	Cached bool `json:"cached,omitempty"`
	// Ref is a stable identity string for dedupe/logs, e.g. "commit:<sha>",
	// "range:<from>..<to>", "stash:refs/stash@{0}", "diffmode:<x>..<y>",
	// "file:<sha>:<path>", "worktree:<path>", "log", "unknown".
	Ref string `json:"ref"`
}

// Action is what watch does with a selection.
type Action string

// Actions.
const (
	Follow     Action = "follow"      // show (group) this diff
	CursorOnly Action = "cursor-only" // keep the current groups, move the cursor to Paths
	Dim        Action = "dim"         // keep the last result, dimmed
)

// Classify inspects the parent git argv (nil when unreadable) and, as a
// fallback, the first line of the (SGR-stripped) diff text.
func Classify(argv []string, diff string) Selection {
	panic("lazygit: Classify not implemented")
}

// Decide maps a selection to an action. followWorktree turns KindWorktree
// into Follow (it is Dim otherwise).
func Decide(s Selection, followWorktree bool) Action {
	panic("lazygit: Decide not implemented")
}
