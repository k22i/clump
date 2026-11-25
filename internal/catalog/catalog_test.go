package catalog

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/gitdiff"
)

type spec struct {
	path     string
	status   gitdiff.Status
	add, del int
	lines    int
}

func diffOf(specs ...spec) *gitdiff.Diff {
	d := &gitdiff.Diff{}
	for i, s := range specs {
		f := &gitdiff.File{OldPath: s.path, NewPath: s.path, Status: s.status}
		h := &gitdiff.Hunk{ID: fmt.Sprintf("h%d", i+1), Index: i, FileIdx: i, File: f,
			Header: fmt.Sprintf("@@ -%d,3 +%d,4 @@", i+1, i+1), Add: s.add, Del: s.del}
		for j := 0; j < s.lines; j++ {
			h.Lines = append(h.Lines, fmt.Sprintf("+line %d", j))
		}
		f.Hunks = []*gitdiff.Hunk{h}
		d.Files = append(d.Files, f)
		d.Hunks = append(d.Hunks, h)
	}
	return d
}

func translations(n int) []spec {
	var s []spec
	for i := 0; i < n; i++ {
		s = append(s, spec{fmt.Sprintf("docs/keys/Keys_%02d.md", i), gitdiff.Modified, 1, 1, 2})
	}
	return s
}

func TestUnitsCollapse(t *testing.T) {
	specs := append([]spec{{"src/a.go", gitdiff.Modified, 3, 1, 4}}, translations(7)...)
	specs = append(specs, spec{"README.md", gitdiff.Modified, 1, 1, 2})
	d := diffOf(specs...)
	u := Units(d, Options{})
	if len(u) != 3 {
		t.Fatalf("units = %d, want 3", len(u))
	}
	if u[0].ID != "h1" || u[1].ID != "c1" || u[2].ID != "h9" {
		t.Fatalf("ids = %s %s %s", u[0].ID, u[1].ID, u[2].ID)
	}
	if u[1].Pattern != "docs/keys/*.md" || u[1].Files != 7 || len(u[1].Hunks) != 7 {
		t.Fatalf("collapsed = %+v", u[1])
	}
	want := map[string][]string{"c1": {"h2", "h3", "h4", "h5", "h6", "h7", "h8"}}
	if got := Aliases(u); !reflect.DeepEqual(got, want) {
		t.Fatalf("aliases = %v", got)
	}
	if got := Line(u[1]); got != "c1 | M | docs/keys/*.md (7 files) | 7 similar hunks, e.g. @@ -2,3 +2,4 @@ | +7 -7" {
		t.Fatalf("line = %q", got)
	}
}

func TestUnitsNoCollapse(t *testing.T) {
	cases := map[string][]spec{
		"too few":          translations(5),
		"different counts": append(translations(5), spec{"docs/keys/Keys_99.md", gitdiff.Modified, 2, 1, 3}),
		"different ext":    append(translations(5), spec{"docs/keys/Keys_99.txt", gitdiff.Modified, 1, 1, 2}),
		"different status": append(translations(5), spec{"docs/keys/Keys_99.md", gitdiff.Added, 1, 1, 2}),
	}
	for name, specs := range cases {
		t.Run(name, func(t *testing.T) {
			u := Units(diffOf(specs...), Options{})
			if len(u) != len(specs) {
				t.Fatalf("units = %d, want %d", len(u), len(specs))
			}
			for i, x := range u {
				if x.Collapsed() || x.ID != fmt.Sprintf("h%d", i+1) {
					t.Fatalf("unit %d = %+v", i, x)
				}
			}
		})
	}
	if u := Units(diffOf(translations(9)...), Options{CollapseMin: -1}); len(u) != 9 {
		t.Fatalf("disabled collapse gave %d units", len(u))
	}
	if u := Units(diffOf(translations(3)...), Options{CollapseMin: 3}); len(u) != 1 {
		t.Fatalf("CollapseMin 3 gave %d units", len(u))
	}
}

func TestPseudoHunksNeverCollapse(t *testing.T) {
	d := diffOf(translations(7)...)
	for _, h := range d.Hunks {
		h.Pseudo = true
	}
	if u := Units(d, Options{}); len(u) != 7 {
		t.Fatalf("units = %d", len(u))
	}
}

func TestLineAndBlock(t *testing.T) {
	d := diffOf(spec{"src/a.go", gitdiff.Modified, 2, 1, 5})
	d.Files[0].OldPath = "src/old.go"
	d.Files[0].Status = gitdiff.Renamed
	u := Units(d, Options{})[0]
	if got := Line(u); got != "h1 | R | src/old.go -> src/a.go | @@ -1,3 +1,4 @@ | +2 -1" {
		t.Fatalf("line = %q", got)
	}
	got, dropped := Block(u, 2)
	want := "### h1 src/old.go -> src/a.go\n@@ -1,3 +1,4 @@\n+line 0\n+line 1\n[... 3 more lines]\n"
	if got != want || dropped != 3 {
		t.Fatalf("block = %q dropped %d", got, dropped)
	}
	got, dropped = Block(u, -1)
	if dropped != 0 || strings.Count(got, "\n") != 7 || strings.Contains(got, "more lines") {
		t.Fatalf("uncapped block = %q", got)
	}
	if got, _ := Block(u, 0); got != "### h1 src/old.go -> src/a.go\n@@ -1,3 +1,4 @@\n[... 5 more lines]\n" {
		t.Fatalf("cap 0 block = %q", got)
	}
}

func TestCollapsedBlock(t *testing.T) {
	u := Units(diffOf(translations(6)...), Options{})[0]
	got, _ := Block(u, -1)
	if !strings.HasPrefix(got, "### c1 docs/keys/*.md (6 similar hunks; sample: docs/keys/Keys_00.md)\n@@ -1,3 +1,4 @@\n+line 0\n") {
		t.Fatalf("block = %q", got)
	}
}

func TestFileBlock(t *testing.T) {
	d := diffOf(spec{"a.go", gitdiff.Modified, 2, 1, 3})
	got, dropped := FileBlock(d, 0, 1)
	want := "=== file a.go (M, +2 -1, 1 hunks)\n### h1 a.go\n@@ -1,3 +1,4 @@\n+line 0\n[... 2 more lines]\n"
	if got != want || dropped != 2 {
		t.Fatalf("file block = %q, %d", got, dropped)
	}
}
