// Package lazygit classifies what lazygit is showing from the git command
// that ran `clump push` (its parent's argv), and prints or installs the
// lazygit config that wires `clump push` in as a diff renderer.
package lazygit

import (
	"path/filepath"
	"regexp"
	"strings"
)

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

// Value-taking git options that may be written with a space instead of "=".
// lazygit always uses the "=" form; this only matters for robustness.
var valueOpts = map[string]bool{
	"-U":           true,
	"--unified":    true,
	"-M":           true,
	"-C":           true,
	"--relative":   true,
	"--format":     true,
	"--pretty":     true,
	"--stat-width": true,
}

// sgrRE matches the SGR sequences git emits with --color.
var sgrRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// commitHeaderRE matches the "commit <sha>" header of `git show` output.
var commitHeaderRE = regexp.MustCompile(`^commit ([0-9a-f]{7,64})\b`)

// Classify inspects the parent git argv (nil when unreadable) and, as a
// fallback, the first line of the (SGR-stripped) diff text.
func Classify(argv []string, diff string) Selection {
	if len(argv) == 0 {
		return classifyDiff(diff)
	}
	base := filepath.Base(argv[0])
	if !strings.HasPrefix(base, "git") {
		return Selection{Kind: KindUnknown, Ref: "unknown"}
	}

	// Skip git's global options: -C and -c take the next argument, every
	// other option stands alone. The first non-option is the subcommand.
	i := 1
	for ; i < len(argv); i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") {
			break
		}
		if a == "-C" || a == "-c" {
			i++
		}
	}
	if i >= len(argv) {
		return Selection{Kind: KindUnknown, Ref: "unknown"}
	}
	sub := argv[i]
	args := argv[i+1:]

	// `git stash show ...` is subcommand "stash", sub-subcommand "show".
	subsub := ""
	if sub == "stash" && len(args) > 0 && args[0] == "show" {
		subsub = "show"
		args = args[1:]
	}
	ca := splitArgs(args)

	switch sub {
	case "log":
		return withRef(Selection{Kind: KindLog})
	case "show":
		if len(ca.positionals) > 0 {
			return withRef(Selection{Kind: KindCommit, Commit: ca.positionals[0]})
		}
	case "stash":
		if subsub == "show" {
			return withRef(Selection{Kind: KindStash, Stash: stashRef(ca.positionals)})
		}
	case "diff":
		return classifyDiffCmd(ca)
	}
	return Selection{Kind: KindUnknown, Ref: "unknown"}
}

// classifyDiffCmd classifies the arguments of a `git diff`.
func classifyDiffCmd(ca cmdArgs) Selection {
	switch {
	case ca.hasFlag("--no-index"):
		paths := ca.pathspecs
		if len(paths) == 0 {
			paths = ca.positionals
		}
		return withRef(Selection{Kind: KindWorktree, Paths: paths})
	case len(ca.positionals) == 0:
		return withRef(Selection{
			Kind:   KindWorktree,
			Paths:  ca.pathspecs,
			Cached: ca.hasFlag("--cached") || ca.hasFlag("--staged"),
		})
	case len(ca.positionals) == 2:
		x, y := ca.positionals[0], ca.positionals[1]
		paths := ca.pathspecs
		dotOnly := len(paths) == 1 && paths[0] == "."
		parent := y + "^"
		switch {
		case x == parent && len(paths) > 0 && !dotOnly:
			return withRef(Selection{Kind: KindFileInCommit, Commit: y, Paths: paths})
		case x == parent:
			// A whole commit shown as a two-tree diff.
			return withRef(Selection{Kind: KindCommit, Commit: y})
		case strings.HasSuffix(x, "^"):
			return withRef(Selection{Kind: KindRange, From: x, To: y})
		default:
			return withRef(Selection{Kind: KindDiffMode, From: x, To: y})
		}
	}
	return Selection{Kind: KindUnknown, Ref: "unknown"}
}

// stashRef picks the stash ref out of a `git stash show`'s positionals.
func stashRef(positionals []string) string {
	for _, p := range positionals {
		if strings.HasPrefix(p, "refs/stash") || strings.HasPrefix(p, "stash@") {
			return p
		}
	}
	if len(positionals) > 0 {
		return positionals[0]
	}
	return ""
}

// classifyDiff is the fallback used when the parent argv cannot be read: only
// the `commit <sha>` header of `git show` identifies anything.
func classifyDiff(diff string) Selection {
	clean := sgrRE.ReplaceAllString(diff, "")
	line := clean
	if i := strings.IndexByte(clean, '\n'); i >= 0 {
		line = clean[:i]
	}
	if m := commitHeaderRE.FindStringSubmatch(line); m != nil {
		return withRef(Selection{Kind: KindCommit, Commit: m[1]})
	}
	return Selection{Kind: KindUnknown, Ref: "unknown"}
}

// withRef sets Ref from the selection's kind and fields.
func withRef(s Selection) Selection {
	switch s.Kind {
	case KindCommit:
		s.Ref = "commit:" + s.Commit
	case KindRange:
		s.Ref = "range:" + s.From + ".." + s.To
	case KindStash:
		s.Ref = "stash:" + s.Stash
	case KindDiffMode:
		s.Ref = "diffmode:" + s.From + ".." + s.To
	case KindFileInCommit:
		s.Ref = "file:" + s.Commit + ":" + strings.Join(s.Paths, ",")
	case KindWorktree:
		ref := "worktree:"
		if s.Cached {
			ref += "cached:"
		}
		s.Ref = ref + strings.Join(s.Paths, ",")
	case KindLog:
		s.Ref = "log"
	default:
		s.Ref = "unknown"
	}
	return s
}

// cmdArgs is a subcommand's arguments split up.
type cmdArgs struct {
	flags       []string
	positionals []string
	pathspecs   []string
}

// hasFlag reports whether the subcommand got one of the given flags.
func (ca cmdArgs) hasFlag(names ...string) bool {
	for _, f := range ca.flags {
		for _, n := range names {
			if f == n {
				return true
			}
		}
	}
	return false
}

// splitArgs splits a subcommand's arguments into flags, positionals and the
// pathspecs that follow a "--".
func splitArgs(args []string) cmdArgs {
	var ca cmdArgs
	afterDD := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if afterDD {
			ca.pathspecs = append(ca.pathspecs, a)
			continue
		}
		if a == "--" {
			afterDD = true
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			ca.flags = append(ca.flags, a)
			if valueOpts[a] && i+1 < len(args) {
				i++
			}
			continue
		}
		ca.positionals = append(ca.positionals, a)
	}
	return ca
}

// Decide maps a selection to an action. followWorktree turns KindWorktree
// into Follow (it is Dim otherwise).
func Decide(s Selection, followWorktree bool) Action {
	switch s.Kind {
	case KindCommit, KindRange, KindStash, KindDiffMode:
		return Follow
	case KindFileInCommit:
		return CursorOnly
	case KindWorktree:
		if followWorktree {
			return Follow
		}
		return Dim
	}
	return Dim
}
