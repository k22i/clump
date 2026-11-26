package handoff

import (
	"fmt"
	"strings"

	"github.com/k22i/clump/internal/comment"
)

const (
	WhereReplace = "replace"
	WherePane    = "pane"
	WherePrint   = "print"
)

func ValidWhere(w string) bool {
	return w == WhereReplace || w == WherePane || w == WherePrint
}

type Agent struct {
	Name  string   `toml:"-"`
	Argv  []string `toml:"argv"`
	Where string   `toml:"where"`
}

func DefaultAgents() []Agent {
	return []Agent{
		{Name: "claude", Argv: []string{"claude"}, Where: WhereReplace},
		{Name: "codex", Argv: []string{"codex"}, Where: WhereReplace},
		{Name: "opencode", Argv: []string{"opencode", "--prompt"}, Where: WhereReplace},
	}
}

func (a Agent) ArgvWith(prompt string) []string {
	out := make([]string, 0, len(a.Argv)+1)
	out = append(out, a.Argv...)
	return append(out, prompt)
}

type Review struct {
	Title      string
	Ref        string
	Comments   []comment.Placed
	GroupTitle func(id string) string
}

func TargetLabel(p comment.Placed, groupTitle func(id string) string) string {
	switch p.Kind {
	case comment.KindLine:
		return fmt.Sprintf("%s:%d (%s side)", p.Path, p.Line, p.Side)
	case comment.KindFile:
		return p.Path + " (file)"
	case comment.KindGroup:
		ids := "clump group " + p.GroupID
		if t := groupTitle(p.GroupID); t != "" {
			return "group \"" + t + "\" (" + ids + ")"
		}
		return ids
	default:
		return "the review"
	}
}

const promptLegend = "\nLine comments name a side (old or new) and that side's line number; file comments name a" +
	" file; group comments carry clump's own grouping and its title."

func Prompt(r Review) string {
	head := "Address these review comments in this repository."
	switch {
	case r.Ref != "" && r.Ref != "stdin" && r.Title != "":
		head = fmt.Sprintf("Address these review comments on %s (%s):", r.Ref, r.Title)
	case r.Ref != "" && r.Ref != "stdin":
		head = "Address these review comments on " + r.Ref + ":"
	}
	var b strings.Builder
	b.WriteString(head)
	b.WriteString("\n\n")
	groupTitle := func(id string) string {
		if r.GroupTitle == nil {
			return ""
		}
		return r.GroupTitle(id)
	}
	for i, p := range r.Comments {
		fmt.Fprintf(&b, "%d. %s: %s\n", i+1, TargetLabel(p, groupTitle), p.Text)
	}
	if len(r.Comments) == 0 {
		b.WriteString("(none)\n")
	}
	b.WriteString(promptLegend)
	return b.String()
}

func AnchorKeys(placed []comment.Placed) []string {
	seen := map[string]bool{}
	var keys []string
	for _, p := range placed {
		if p.Anchor != "" && !seen[p.Anchor] {
			seen[p.Anchor] = true
			keys = append(keys, p.Anchor)
		}
	}
	return keys
}

type Request struct {
	Agent   string
	Where   string
	Argv    []string
	Path    string
	Prompt  string
	Dir     string
	Anchors []string
}

func (r *Request) Command() string {
	if r == nil || len(r.Argv) == 0 {
		return ""
	}
	if r.Path != "" {
		return r.Path + " " + shellJoin(r.Argv[1:])
	}
	return shellJoin(r.Argv)
}

type Environment struct {
	RexSession string
	RexBlock   string
	Tmux       string
}

func PaneArgv(e Environment, repo, label string, agentArgv []string) ([]string, string, bool) {
	if e.RexSession != "" && e.RexBlock != "" {
		out := []string{"rex", "split", "-b", e.RexBlock, "--split", "right", "--cwd", repo, "--label", label}
		return append(out, agentArgv...), "rex", true
	}
	if e.Tmux != "" {
		return []string{"tmux", "split-window", "-h", "-c", repo, shellJoin(agentArgv)}, "tmux", true
	}
	return nil, "", false
}

func shellJoin(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		parts = append(parts, ShellQuote(a))
	}
	return strings.Join(parts, " ")
}

func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b.WriteString("'\\''")
		} else {
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func LabelFor(agent string) string {
	return "handoff: " + agent
}
