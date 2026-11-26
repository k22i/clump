package comment

import (
	"testing"
	"time"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
)

func parse(t *testing.T, patch string) *gitdiff.Diff {
	t.Helper()
	d, err := gitdiff.Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

const smallPatch = "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,2 +1,2 @@\n ctx\n-old\n+new\n"

const delPatch = "diff --git a/gone b/gone\n--- a/gone\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-bye\n-now\n"

func TestLineAnchorSides(t *testing.T) {
	d := parse(t, smallPatch)
	as := Anchors(d, "hash", nil)
	var got []string
	for _, a := range as {
		if a.Kind == KindLine {
			got = append(got, a.Side+":"+itoa(a.Line))
		}
	}
	want := []string{"new:1", "old:2", "new:2"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("line anchors: got %v, want %v", got, want)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestDeletedFileAnchorsOnOldSide(t *testing.T) {
	d := parse(t, delPatch)
	as := Anchors(d, "hash", nil)
	var got []string
	for _, a := range as {
		if a.Kind == KindLine {
			got = append(got, a.Side+":"+itoa(a.Line))
		}
	}
	if len(got) != 2 || got[0] != "old:1" || got[1] != "old:2" {
		t.Fatalf("deleted file anchors: got %v", got)
	}
}

func TestKeysAreStableAndDistinct(t *testing.T) {
	d := parse(t, smallPatch)
	d2 := parse(t, smallPatch)
	a1 := Anchors(d, "diffhash", []string{"g1"})
	a2 := Anchors(d2, "diffhash", []string{"g1"})
	if len(a1) != len(a2) {
		t.Fatal("same diff should give the same anchor count")
	}
	seen := map[string]bool{}
	for i := range a1 {
		if a1[i].Key != a2[i].Key {
			t.Fatalf("anchor %d key drift: %s != %s", i, a1[i].Key, a2[i].Key)
		}
		if seen[a1[i].Key] {
			t.Fatalf("duplicate key %s", a1[i].Key)
		}
		seen[a1[i].Key] = true
	}
	if a1[len(a1)-1].Kind != KindGroup || a1[len(a1)-1].GroupID != "g1" {
		t.Fatal("group anchor missing")
	}
}

func TestFileKeyChangesWithContent(t *testing.T) {
	k1 := FileKey("a", []string{"h1"})
	k2 := FileKey("a", []string{"h1"})
	k3 := FileKey("a", []string{"h1", "h2"})
	if k1 != k2 {
		t.Fatal("same content must give same key")
	}
	if k1 == k3 {
		t.Fatal("different hunk sets must give different keys")
	}
	if FileKey("a", []string{"h1"}) == FileKey("b", []string{"h1"}) {
		t.Fatal("different paths must give different keys")
	}
}

func TestFindAnchor(t *testing.T) {
	d := parse(t, smallPatch)
	a, err := FindAnchor(d, "a", SideNew, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := LineKey(d.Hunks[0].ReviewKey(), SideNew, 2)
	if a.Key != want || a.Path != "a" || a.Side != SideNew || a.Line != 2 {
		t.Fatalf("line anchor: %+v", a)
	}
	a, err = FindAnchor(d, "a", SideOld, 2)
	if err != nil {
		t.Fatal(err)
	}
	if a.Side != SideOld {
		t.Fatalf("old-side anchor: %+v", a)
	}
	// path alone: file comment
	a, err = FindAnchor(d, "a", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != KindFile || a.Key != FileKey("a", []string{d.Hunks[0].ReviewKey()}) {
		t.Fatalf("file anchor: %+v", a)
	}
	// deleted file: old side only
	dd := parse(t, delPatch)
	a, err = FindAnchor(dd, "gone", SideNew, 1)
	if err == nil {
		t.Fatalf("new-side line should not exist in a deleted file: %+v", a)
	}
	a, err = FindAnchor(dd, "gone", SideOld, 2)
	if err != nil {
		t.Fatal(err)
	}
	if a.Side != SideOld || a.Line != 2 {
		t.Fatalf("deleted-file anchor: %+v", a)
	}
	if _, err = FindAnchor(d, "nope", SideNew, 1); err == nil {
		t.Fatal("unknown file should error")
	}
	if _, err = FindAnchor(d, "a", SideNew, 9); err == nil {
		t.Fatal("unknown line should error")
	}
}

func TestResolvePlace(t *testing.T) {
	d := parse(t, smallPatch+"diff --git a/b b/b\n--- a/b\n+++ b/b\n@@ -1 +1 @@\n-x\n+y\n")
	if len(d.Hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d", len(d.Hunks))
	}
	anchors := Anchors(d, "dh", []string{"g1", "g2"})
	byKind := map[Kind][]Anchor{}
	for _, a := range anchors {
		byKind[a.Kind] = append(byKind[a.Kind], a)
	}
	comments := map[string][]Comment{
		byKind[KindLine][0].Key:     {{ID: "c1", Text: "first", At: time.Unix(10, 0)}},
		byKind[KindLine][1].Key:     {{ID: "c2", Text: "second", At: time.Unix(11, 0)}},
		byKind[KindFile][0].Key:     {{ID: "c3", Text: "file a", At: time.Unix(12, 0)}},
		anchors[len(anchors)-1].Key: {{ID: "c4", Text: "group b", At: time.Unix(13, 0)}},
	}
	anchors[0].GroupID = "" // resolved from grouping below
	res := Resolve(anchors, comments)
	// first three comments are on a's hunks; the group comment is on g2
	g := group.Grouping{Groups: []group.Group{
		{ID: "g1", Title: "A", Hunks: []string{d.Hunks[0].ID}},
		{ID: "g2", Title: "B", Hunks: []string{d.Hunks[1].ID}},
	}}
	placed := Place(res, d, g)
	if len(placed) != 4 {
		t.Fatalf("want 4 placed comments, got %d", len(placed))
	}
	byID := map[string]Placed{}
	for _, p := range placed {
		byID[p.ID] = p
	}
	if byID["c1"].GroupID != "g1" || byID["c1"].Kind != KindLine {
		t.Fatalf("c1 should be a line comment in g1: %+v", byID["c1"])
	}
	if byID["c3"].GroupID != "g1" || byID["c3"].Kind != KindFile {
		t.Fatalf("file comment follows the file's first grouped hunk: %+v", byID["c3"])
	}
	if byID["c2"].GroupID != "g1" || byID["c2"].Side != SideOld {
		t.Fatalf("c2: %+v", byID["c2"])
	}
	if byID["c4"].GroupID != "g2" || byID["c4"].Kind != KindGroup {
		t.Fatalf("group comment keeps its group: %+v", byID["c4"])
	}
}

func TestKindText(t *testing.T) {
	for k, want := range map[Kind]string{KindLine: "line", KindFile: "file", KindGroup: "group"} {
		var got Kind
		if err := got.UnmarshalText([]byte(want)); err != nil || got != k {
			t.Fatalf("unmarshal %s: %v %v", want, got, err)
		}
	}
	var k Kind
	if err := k.UnmarshalText([]byte("nope")); err == nil {
		t.Fatal("unknown kind should error")
	}
}

func TestLineTarget(t *testing.T) {
	if s, n := LineTarget(false, 1, 5); s != SideNew || n != 5 {
		t.Fatalf("context prefers new: %s %d", s, n)
	}
	if s, n := LineTarget(true, 3, 5); s != SideOld || n != 3 {
		t.Fatalf("del prefers old: %s %d", s, n)
	}
	if s, n := LineTarget(false, 7, 0); s != SideOld || n != 7 {
		t.Fatalf("no new side falls back to old: %s %d", s, n)
	}
}
