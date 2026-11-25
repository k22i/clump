package diffview

import (
	"strings"
	"testing"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui/view"
	"github.com/k22i/clump/internal/tui/view/viewtest"
)

const numPatch = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -8,3 +8,3 @@ func f() {
 a
-b
+c
 d
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1 @@
-x
+y
`

func TestLineNumbers(t *testing.T) {
	d, err := gitdiff.Parse(numPatch)
	if err != nil {
		t.Fatal(err)
	}
	if w := NumberWidth(d); w != 2 {
		t.Fatalf("NumberWidth = %d", w)
	}
	lines := RenderHunk(d.Hunks[0], view.PlainTheme(), HunkOpts{Numbers: 2})
	got := viewtest.JoinPlain(lines)
	want := "" +
		"  @@ -8,3 +8,3 @@ func f() {\n" +
		"   8  8  a\n" +
		"   9    -b\n" +
		"      9 +c\n" +
		"  10 10  d\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if lines[1].Gutter != 8 {
		t.Fatalf("Gutter = %d", lines[1].Gutter)
	}
	big := &gitdiff.Diff{Hunks: []*gitdiff.Hunk{{NewStart: 9990, NewLines: 20}}}
	if w := NumberWidth(big); w != 5 {
		t.Fatalf("NumberWidth(10010) = %d", w)
	}
}

func TestRenderGroupLinesFolded(t *testing.T) {
	d, err := gitdiff.Parse(numPatch)
	if err != nil {
		t.Fatal(err)
	}
	d.Files[1].Generated = "pattern b.go"
	lines := RenderGroupLinesOpts(d, []string{"h1", "h2"}, view.PlainTheme(), 70, GroupOpts{
		File: func(fi int, ids []string) view.FileState {
			return view.FileState{Folded: fi == 1, Reviewed: fi == 1, Generated: d.Files[fi].Generated}
		},
	})
	got := viewtest.JoinPlain(lines)
	if !strings.Contains(got, "── ✓ b.go (M +1 −1) · 1 hunk folded · generated (pattern b.go) ─") {
		t.Fatalf("folded header missing:\n%s", got)
	}
	if strings.Contains(got, "-x") {
		t.Fatalf("folded hunk rendered:\n%s", got)
	}
	last := lines[len(lines)-1]
	if last.Kind != view.KindFile || !last.Folded || strings.Join(last.BlockHunks, ",") != "h2" {
		t.Fatalf("header line = %+v", last)
	}
}
