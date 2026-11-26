package report

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
)

func doc(t *testing.T) Doc {
	t.Helper()
	b, err := os.ReadFile("../gitdiff/testdata/lazygit/range-selection.txt")
	if err != nil {
		t.Fatal(err)
	}
	d, err := gitdiff.Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return Doc{Title: "Range", Ref: "bdd913d801bb^..6bdf738591d3", Diff: d, Provider: "claude-haiku", DiffHash: d.Hash(), Edited: true,
		Grouping: group.Grouping{Groups: []group.Group{
			{ID: "g1", Title: "Rename docs", Description: "Moves old.txt.", Hunks: []string{"h1"}},
			{ID: "g2", Title: "Add assets", Hunks: []string{"h2", "h3", "h4"}},
			{ID: "g3", Title: "Empty", Hunks: nil},
		}},
		Notices: []string{"repaired: x"}}
}

func TestMarkdown(t *testing.T) {
	got := Markdown(doc(t))
	want := "# Range (bdd913d801bb^..6bdf738591d3)\n\n3 groups · 4 files · +2 −0 · grouped by claude-haiku · edited\n\n" +
		"## 1. Rename docs (+1 −0)\n\nMoves old.txt.\n\n- `docs/old.txt -> docs/new.txt` (+1 −0)\n" +
		"\n## 2. Add assets (+1 −0)\n\n- `logo.bin` (+0 −0)\n- `nonl.txt` (+1 −0)\n- `src/main.go` (+0 −0)\n" +
		"\n## 3. Empty (+0 −0)\n\n_(empty)_\n\n> repaired: x\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMarkdownPartialFile(t *testing.T) {
	d, _ := gitdiff.Parse("diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-x\n+y\n@@ -9 +9 @@\n-x\n+y\n")
	md := Markdown(Doc{Title: "T", Ref: "stdin", Diff: d, Grouping: group.Grouping{Groups: []group.Group{{ID: "g1", Title: "A", Hunks: []string{"h2"}}, {ID: "g2", Title: "B", Hunks: []string{"h1"}}}}})
	if !strings.HasPrefix(md, "# T\n\n2 groups · 1 file · +2 −2\n") || !strings.Contains(md, "- `a` (+1 −1, 1 of 2 hunks)") {
		t.Fatalf("md:\n%s", md)
	}
}

func TestMarkdownComments(t *testing.T) {
	d := doc(t)
	d.Comments = []comment.Placed{
		{Kind: comment.KindGroup, GroupID: "g1", Text: "split this out"},
		{Kind: comment.KindFile, GroupID: "g1", Path: "docs/old.txt", Text: "document the rename"},
		{Kind: comment.KindLine, GroupID: "g2", Path: "logo.bin", Side: comment.SideNew, Line: 3, Text: "suspicious rename"},
	}
	md := Markdown(d)
	if !strings.Contains(md, "3 review comments") {
		t.Fatalf("meta missing comments:\n%s", md)
	}
	if !strings.Contains(md, "1. this group — split this out\n2. `docs/old.txt` (file) — document the rename") {
		t.Fatalf("group comments:\n%s", md)
	}
	if !strings.Contains(md, "1. `logo.bin:3` (new side) — suspicious rename") {
		t.Fatalf("line comment:\n%s", md)
	}
	if strings.Contains(Markdown(doc(t)), "Review comments") {
		t.Fatal("empty comments should not render a section")
	}
}

func TestJSONComments(t *testing.T) {
	d := doc(t)
	d.Comments = []comment.Placed{
		{Kind: comment.KindLine, GroupID: "g1", Path: "docs/old.txt", Side: comment.SideOld, Line: 2,
			Text: "here", ID: "abc123", At: time.Unix(1000, 0).UTC(), Anchor: "k"},
	}
	b, err := JSON(d)
	if err != nil {
		t.Fatal(err)
	}
	var out JSONDoc
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Comments) != 1 {
		t.Fatalf("json comments: %+v", out.Comments)
	}
	c := out.Comments[0]
	if c.Kind != "line" || c.GroupID != "g1" || c.Path != "docs/old.txt" || c.Side != "old" ||
		c.Line != 2 || c.Text != "here" || c.ID != "abc123" {
		t.Fatalf("json comment: %+v", c)
	}
}

func TestJSON(t *testing.T) {
	b, err := JSON(doc(t))
	if err != nil {
		t.Fatal(err)
	}
	var out JSONDoc
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Version != 1 || !out.Edited || out.Files != 4 || out.HunkCount != 4 || len(out.Groups) != 3 || out.Groups[2].Hunks == nil {
		t.Fatalf("out = %+v", out)
	}
	h := out.Groups[0].Hunks[0]
	if h.ID != "h1" || h.File != "docs/new.txt" || h.OldFile != "docs/old.txt" || h.Status != "R" || h.Add != 1 {
		t.Fatalf("hunk = %+v", h)
	}
	if !out.Groups[1].Hunks[0].Pseudo || !strings.HasSuffix(string(b), "}\n") {
		t.Fatalf("pseudo flag or trailing newline missing")
	}
}
