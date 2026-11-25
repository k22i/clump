package group

import (
	"strings"
	"testing"
)

func TestValidateGenerated(t *testing.T) {
	d := mkDiff(2, 1, 2) // h1 h2 | h3 | h4 h5
	d.Files[1].Generated = "pattern go.sum"
	// The model groups what it saw and names a generated hunk anyway.
	r := Validate(d, `{"groups":[{"title":"A","hunks":["h1","h3"]},{"title":"B","hunks":["h4"]}]}`, ValidateOpts{})
	mustCheck(t, d, r)
	ids := func(i int) string { return strings.Join(r.Groups[i].Hunks, ",") }
	if len(r.Groups) != 4 || r.Groups[2].ID != UngroupedID || r.Groups[3].ID != GeneratedID {
		t.Fatalf("groups = %+v", r.Groups)
	}
	if ids(0) != "h1" || ids(1) != "h4" || ids(2) != "h2,h5" || ids(3) != "h3" {
		t.Fatalf("hunks = %s | %s | %s | %s", ids(0), ids(1), ids(2), ids(3))
	}
	if r.Groups[3].Title != GeneratedTitle {
		t.Fatalf("title = %q", r.Groups[3].Title)
	}
	want := []string{"repaired: dropped 1 unknown hunk IDs", "repaired: 2 hunks missing from the model output; put in Ungrouped"}
	if strings.Join(r.Notices, "|") != strings.Join(want, "|") {
		t.Fatalf("notices = %q", r.Notices)
	}

	// The fallback keeps generated files out of the per-file groups.
	fb := Validate(d, "not json", ValidateOpts{})
	mustCheck(t, d, fb)
	if !fb.Fallback || len(fb.Groups) != 3 || fb.Groups[2].ID != GeneratedID || ids(3) != "h3" {
		t.Fatalf("fallback = %+v", fb.Groups)
	}
	if _, ok := AllGenerated(d); ok {
		t.Fatal("AllGenerated with reviewable hunks")
	}
}

func TestAllGenerated(t *testing.T) {
	d := mkDiff(1, 2)
	d.Files[0].Generated, d.Files[1].Generated = "x", "y"
	r, ok := AllGenerated(d)
	if !ok || len(r.Groups) != 1 || r.Groups[0].ID != GeneratedID || len(r.Groups[0].Hunks) != 3 {
		t.Fatalf("AllGenerated = %+v %v", r, ok)
	}
	mustCheck(t, d, r)
	if _, ok := AllGenerated(mkDiff()); ok {
		t.Fatal("AllGenerated on an empty diff")
	}
}

func TestBuildLeavesOutGenerated(t *testing.T) {
	d := mkDiff(1, 1, 1)
	d.Files[1].Generated = "pattern go.sum"
	p := Build(d, BuildOpts{Nonce: "n", Context: "COMMIT MESSAGE:\nx"})
	if len(p.Units) != 2 || p.Units[0].ID != "h1" || p.Units[1].ID != "h3" {
		t.Fatalf("units = %+v", p.Units)
	}
	if strings.Contains(p.Prompt, "### h2") || strings.Contains(p.Prompt, "h2 |") {
		t.Fatal("generated hunk in the prompt")
	}
	if !strings.Contains(p.Prompt, "Generated files also changed (left out of this request; do not group them): dir1/f1.go") {
		t.Fatalf("no generated note in:\n%s", p.Prompt)
	}

	// Tier 2 summarises only the reviewable files.
	big := mkDiff(1, 1, 1)
	big.Files[0].Generated = "x"
	for _, h := range big.Hunks {
		h.Lines = make([]string, 400)
		for i := range h.Lines {
			h.Lines[i] = "+" + strings.Repeat("y", 60)
		}
	}
	p2 := Build(big, BuildOpts{Nonce: "n", Budget: 3000, BytesPerToken: 3, MinBodyCap: 1000})
	if p2.Tier != 2 {
		t.Fatalf("tier = %d", p2.Tier)
	}
	for _, c := range p2.Summaries {
		for _, fi := range c.Files {
			if fi == 0 {
				t.Fatal("generated file summarised")
			}
		}
	}
	final, _ := p2.BuildFromSummaries(big, map[int]string{1: "s1", 2: "s2"})
	if strings.Contains(final, "- dir0/f0.go:") {
		t.Fatal("generated file in the summaries section")
	}
}
