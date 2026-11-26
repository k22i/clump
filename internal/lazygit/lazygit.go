package lazygit

import (
	"path/filepath"
	"regexp"
	"strings"
)

type Kind string

const (
	KindUnknown      Kind = "unknown"
	KindCommit       Kind = "commit"
	KindRange        Kind = "range"
	KindStash        Kind = "stash"
	KindDiffMode     Kind = "diffmode"
	KindFileInCommit Kind = "file-in-commit"
	KindWorktree     Kind = "worktree"
	KindLog          Kind = "log"
)

type Selection struct {
	Kind   Kind     `json:"kind"`
	Commit string   `json:"commit,omitempty"`
	From   string   `json:"from,omitempty"`
	To     string   `json:"to,omitempty"`
	Stash  string   `json:"stash,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	Cached bool     `json:"cached,omitempty"`
	Ref    string   `json:"ref"`
}

type Action string

const (
	Follow     Action = "follow"
	CursorOnly Action = "cursor-only"
	Dim        Action = "dim"
)

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

var sgrRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

var commitHeaderRE = regexp.MustCompile(`^commit ([0-9a-f]{7,64})\b`)

func Classify(argv []string, diff string) Selection {
	if len(argv) == 0 {
		return classifyDiff(diff)
	}
	base := filepath.Base(argv[0])
	if !strings.HasPrefix(base, "git") {
		return Selection{Kind: KindUnknown, Ref: "unknown"}
	}

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
			return withRef(Selection{Kind: KindCommit, Commit: y})
		case strings.HasSuffix(x, "^"):
			return withRef(Selection{Kind: KindRange, From: x, To: y})
		default:
			return withRef(Selection{Kind: KindDiffMode, From: x, To: y})
		}
	}
	return Selection{Kind: KindUnknown, Ref: "unknown"}
}

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

type cmdArgs struct {
	flags       []string
	positionals []string
	pathspecs   []string
}

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
