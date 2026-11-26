package handoff

import (
	"strings"
	"testing"
	"time"

	"github.com/k22i/clump/internal/comment"
)

func placedLine(path string, line int, side, text string) comment.Placed {
	return comment.Placed{Kind: comment.KindLine, Path: path, Side: side, Line: line, Text: text, ID: "id1",
		At: time.Unix(1700000000, 0), Anchor: "aaaa"}
}

func placedFile(path, text string) comment.Placed {
	return comment.Placed{Kind: comment.KindFile, Path: path, Text: text, ID: "id2", At: time.Unix(1700000000, 0), Anchor: "bbbb"}
}

func placedGroup(id, text string) comment.Placed {
	return comment.Placed{Kind: comment.KindGroup, GroupID: id, Text: text, ID: "id3", At: time.Unix(1700000000, 0), Anchor: "cccc"}
}

func TestPromptCarriesEveryKindWithRefAndTitle(t *testing.T) {
	p := Prompt(Review{
		Title: "Add the handoff command", Ref: "abc123",
		Comments: []comment.Placed{
			placedLine("cmd/clump/handoff.go", 42, comment.SideNew, "name the flag after the config key"),
			placedFile("README.md", "document the key too"),
			placedGroup("g2", "rewrite this group"),
		},
		GroupTitle: func(id string) string {
			if id == "g2" {
				return "CLI surface"
			}
			return ""
		},
	})
	for _, want := range []string{
		"Address these review comments on abc123 (Add the handoff command):",
		"1. cmd/clump/handoff.go:42 (new side): name the flag after the config key",
		"2. README.md (file): document the key too",
		"3. group \"CLI surface\" (clump group g2): rewrite this group",
		"Line comments name a side (old or new) and that side's line number",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt is missing %q:\n%s", want, p)
		}
	}
	if !strings.HasSuffix(p, "its title.") {
		t.Errorf("prompt ends with %q", p[len(p)-40:])
	}
}

func TestPromptWithoutRefNamesTheRepository(t *testing.T) {
	p := Prompt(Review{Comments: []comment.Placed{placedFile("a.txt", "fix")}})
	if !strings.HasPrefix(p, "Address these review comments in this repository.\n\n") {
		t.Errorf("head = %q", strings.Split(p, "\n")[0])
	}
	if !strings.Contains(p, "1. a.txt (file): fix") {
		t.Errorf("prompt:\n%s", p)
	}
}

func TestPromptGroupWithoutTitleFallsBackToID(t *testing.T) {
	p := Prompt(Review{Comments: []comment.Placed{placedGroup("g7", "split it")}})
	if !strings.Contains(p, "1. clump group g7: split it") {
		t.Errorf("prompt:\n%s", p)
	}
}

func TestPromptWithoutCommentsSaysNone(t *testing.T) {
	p := Prompt(Review{Comments: nil})
	if !strings.Contains(p, "(none)") {
		t.Errorf("prompt:\n%s", p)
	}
}

func TestPromptOldSideIsStated(t *testing.T) {
	p := Prompt(Review{Comments: []comment.Placed{placedLine("a.go", 3, comment.SideOld, "gone")}})
	if !strings.Contains(p, "1. a.go:3 (old side): gone") {
		t.Errorf("prompt:\n%s", p)
	}
}

func TestDefaultAgentsAppendThePrompt(t *testing.T) {
	prompt := "the prompt"
	for _, a := range DefaultAgents() {
		if a.Name == "" || a.Where != WhereReplace {
			t.Fatalf("builtin %v not replace", a)
		}
	}
	cases := map[string][]string{
		"claude":   {"claude", prompt},
		"codex":    {"codex", prompt},
		"opencode": {"opencode", "--prompt", prompt},
	}
	for _, a := range DefaultAgents() {
		want := cases[a.Name]
		got := a.ArgvWith(prompt)
		if len(got) != len(want) {
			t.Fatalf("%s argv = %q, want %q", a.Name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s argv = %q, want %q", a.Name, got, want)
			}
		}
	}
}

func TestArgvWithDoesNotAliasTheConfig(t *testing.T) {
	a := Agent{Name: "x", Argv: []string{"x", "-n"}, Where: WherePrint}
	first := a.ArgvWith("p")
	first[0] = "changed"
	if a.Argv[0] != "x" {
		t.Fatalf("ArgvWith mutated the configured argv: %q", a.Argv)
	}
}

func TestPaneArgvRex(t *testing.T) {
	got, via, ok := PaneArgv(Environment{RexSession: "s1", RexBlock: "b7"}, "/repo", "handoff: codex", []string{"codex", "fix it"})
	if !ok || via != "rex" {
		t.Fatalf("ok=%v via=%q", ok, via)
	}
	want := []string{"rex", "split", "-b", "b7", "--split", "right", "--cwd", "/repo", "--label", "handoff: codex", "--", "codex", "fix it"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestPaneArgvTmuxQuotesTheCommand(t *testing.T) {
	got, via, ok := PaneArgv(Environment{Tmux: "/tmp/tmux-0/default"}, "/repo", "handoff: codex", []string{"codex", "fix it 'now'"})
	if !ok || via != "tmux" {
		t.Fatalf("ok=%v via=%q", ok, via)
	}
	want := []string{"tmux", "split-window", "-h", "-c", "/repo", "'codex' 'fix it '\\''now'\\'''"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestPaneArgvNeedsAMultiplexer(t *testing.T) {
	env := Environment{RexSession: "s1", RexBlock: "", Tmux: ""}
	if got, _, ok := PaneArgv(env, "/repo", "l", []string{"claude"}); ok {
		t.Fatalf("no multiplexer, got %q", got)
	}
	if _, _, ok := PaneArgv(Environment{RexBlock: "b"}, "/repo", "l", []string{"claude"}); ok {
		t.Fatal("REX_BLOCK alone must not be enough")
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":       "''",
		"plain":  "'plain'",
		"it's":   `'it'\''s'`,
		"a\nb$c": "'a\nb$c'",
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRequestCommandQuotesThePrompt(t *testing.T) {
	r := &Request{Path: "/usr/local/bin/claude", Argv: []string{"claude", "go\nthere"}}
	want := "/usr/local/bin/claude 'go\nthere'"
	if got := r.Command(); got != want {
		t.Errorf("Command() = %q, want %q", got, want)
	}
}

func TestAnchorKeysAreUniqueInOrder(t *testing.T) {
	ps := []comment.Placed{placedLine("a", 1, "new", "x"), placedFile("a", "y"), placedLine("a", 2, "new", "z")}
	ps[2].Anchor = ps[0].Anchor
	got := AnchorKeys(ps)
	if len(got) != 2 || got[0] != "aaaa" || got[1] != "bbbb" {
		t.Fatalf("AnchorKeys = %q", got)
	}
}
