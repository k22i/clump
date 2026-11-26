package group

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/gitdiff"
)

func buildTestDiff(files, hunksPer, bodyLines int) *gitdiff.Diff {
	d := &gitdiff.Diff{}
	n := 0
	for fi := 0; fi < files; fi++ {
		p := fmt.Sprintf("dir%d/file%d.go", fi%3, fi)
		f := &gitdiff.File{OldPath: p, NewPath: p, Status: gitdiff.Modified}
		for j := 0; j < hunksPer; j++ {
			n++
			h := &gitdiff.Hunk{ID: fmt.Sprintf("h%d", n), Index: n - 1, FileIdx: fi, File: f,
				Header: fmt.Sprintf("@@ -%d,0 +%d,%d @@ func f%d()", j*100+1, j*100+1, bodyLines, n), Add: bodyLines}
			for k := 0; k < bodyLines; k++ {
				h.Lines = append(h.Lines, fmt.Sprintf("+\tx%d := compute(%d, %d) // padding padding", k, n, k))
			}
			f.Hunks = append(f.Hunks, h)
			d.Hunks = append(d.Hunks, h)
		}
		d.Files = append(d.Files, f)
	}
	return d
}

var nonceRe = regexp.MustCompile(`<<<CLUMP-DATA-([0-9a-f]{12})>>>`)

func assertCatalog(t *testing.T, d *gitdiff.Diff, prompt string) {
	t.Helper()
	for _, h := range d.Hunks {
		line := fmt.Sprintf("%s | M | %s | %s | +%d -%d\n", h.ID, h.File.Path(), h.Header, h.Add, h.Del)
		if !strings.Contains(prompt, line) {
			t.Fatalf("catalog line missing: %q", line)
		}
	}
}

func TestBuildSmallIsUntruncated(t *testing.T) {
	d := buildTestDiff(2, 2, 3)
	p := Build(d, BuildOpts{CollapseMin: -1, MaxGroups: 5, Budget: 60000, BytesPerToken: 2, Context: "COMMIT MESSAGE:\nfix things"})
	if p.Tier != 1 || p.BodyCap != -1 || p.Truncated != 0 || p.Calls() != 1 || len(p.Notices) != 0 {
		t.Fatalf("plan = tier %d cap %d trunc %d notices %v", p.Tier, p.BodyCap, p.Truncated, p.Notices)
	}
	if !strings.HasPrefix(p.Prompt, "# clump-task: group v1\n") {
		t.Fatalf("task line missing")
	}
	m := nonceRe.FindStringSubmatch(p.Prompt)
	if m == nil || !strings.Contains(p.Prompt, "<<<END-CLUMP-DATA-"+m[1]+">>>") {
		t.Fatalf("data markers missing or mismatched")
	}
	if strings.Contains(p.Prompt, "{{") {
		t.Fatalf("unfilled placeholder in prompt")
	}
	if !strings.Contains(p.Prompt, "at most 5 groups") || !strings.Contains(p.Prompt, "fix things\nCATALOG") {
		t.Fatalf("max groups or context missing")
	}
	assertCatalog(t, d, p.Prompt)
	for _, h := range d.Hunks {
		if !strings.Contains(p.Prompt, "### "+h.ID+" "+h.File.Path()+"\n"+h.Header+"\n"+strings.Join(h.Lines, "\n")+"\n") {
			t.Fatalf("block of %s missing", h.ID)
		}
	}
	if want := (len(p.Prompt) + 1) / 2; p.EstTokens != want {
		t.Fatalf("est = %d, want %d", p.EstTokens, want)
	}
}

func TestBuildNonceIsFreshPerRequest(t *testing.T) {
	d := buildTestDiff(1, 1, 1)
	a := nonceRe.FindStringSubmatch(Build(d, BuildOpts{CollapseMin: -1}).Prompt)[1]
	b := nonceRe.FindStringSubmatch(Build(d, BuildOpts{CollapseMin: -1}).Prompt)[1]
	if a == b {
		t.Fatalf("nonce repeated: %s", a)
	}
	if got := Build(d, BuildOpts{CollapseMin: -1, Nonce: "abcdefabcdef"}).Prompt; !strings.Contains(got, "<<<CLUMP-DATA-abcdefabcdef>>>") {
		t.Fatalf("fixed nonce not used")
	}
}

func TestBuildTier1Truncates(t *testing.T) {
	d := buildTestDiff(4, 2, 200)
	full := Build(d, BuildOpts{CollapseMin: -1, BytesPerToken: 2})
	budget := full.EstTokens / 3
	p := Build(d, BuildOpts{CollapseMin: -1, Budget: budget, BytesPerToken: 2})
	if p.Tier != 1 || p.BodyCap < 4 || p.BodyCap >= 200 {
		t.Fatalf("tier %d cap %d", p.Tier, p.BodyCap)
	}
	if p.EstTokens > budget {
		t.Fatalf("est %d over budget %d", p.EstTokens, budget)
	}
	if p.Truncated != len(d.Hunks) || p.DroppedLines != len(d.Hunks)*(200-p.BodyCap) {
		t.Fatalf("truncated %d dropped %d", p.Truncated, p.DroppedLines)
	}
	if !strings.Contains(p.Prompt, fmt.Sprintf("[... %d more lines]", 200-p.BodyCap)) {
		t.Fatalf("cut marker missing")
	}
	assertCatalog(t, d, p.Prompt)
	for _, h := range d.Hunks {
		if !strings.Contains(p.Prompt, h.Header+"\n") {
			t.Fatalf("header of %s missing", h.ID)
		}
	}
	if len(p.Notices) != 1 || !strings.Contains(p.Notices[0], fmt.Sprintf("cut to %d lines", p.BodyCap)) {
		t.Fatalf("notices = %v", p.Notices)
	}
}

func TestBuildTier1CapIsMaximal(t *testing.T) {
	d := buildTestDiff(3, 1, 100)
	full := Build(d, BuildOpts{CollapseMin: -1, BytesPerToken: 2})
	p := Build(d, BuildOpts{CollapseMin: -1, Budget: full.EstTokens / 2, BytesPerToken: 2})
	if p.Tier != 1 {
		t.Fatalf("tier %d", p.Tier)
	}
	o := BuildOpts{CollapseMin: -1, Budget: full.EstTokens / 2, BytesPerToken: 2, Nonce: "000000000000"}.buildNorm()
	units := p.Units
	h, _, _ := buildHunks(units, p.BodyCap+1)
	if o.fits(o.buildRenderGroup("000000000000", buildCatalog(units), h)) {
		t.Fatalf("cap %d is not maximal", p.BodyCap)
	}
}

func TestBuildTier2WhenCapTooSmall(t *testing.T) {
	d := buildTestDiff(30, 3, 60)
	zero := BuildOpts{CollapseMin: -1, BytesPerToken: 2}.buildNorm()
	h0, _, _ := buildHunks(Build(d, BuildOpts{CollapseMin: -1}).Units, 0)
	catTokens := zero.buildEst(zero.buildRenderGroup("000000000000", buildCatalog(Build(d, BuildOpts{CollapseMin: -1}).Units), h0))
	budget := catTokens + 300
	p := Build(d, BuildOpts{CollapseMin: -1, Budget: budget, BytesPerToken: 2})
	if p.Tier != 2 || p.Prompt != "" || len(p.Summaries) < 2 || p.Calls() != len(p.Summaries)+1 {
		t.Fatalf("tier %d summaries %d", p.Tier, len(p.Summaries))
	}
	var files []int
	for _, c := range p.Summaries {
		files = append(files, c.Files...)
		if c.EstTokens > budget {
			t.Fatalf("chunk est %d over budget %d", c.EstTokens, budget)
		}
		if !strings.HasPrefix(c.Prompt, "# clump-task: summarize v1\n") || strings.Contains(c.Prompt, "{{") {
			t.Fatalf("bad summary prompt")
		}
		for _, fi := range c.Files {
			if !strings.Contains(c.Prompt, "=== file "+d.Files[fi].Path()+" (") {
				t.Fatalf("file %d missing from its chunk", fi)
			}
		}
	}
	for i, fi := range files {
		if fi != i {
			t.Fatalf("files not covered once in order: %v", files)
		}
	}
	if len(files) != len(d.Files) {
		t.Fatalf("covered %d of %d files", len(files), len(d.Files))
	}
	sum := p.FinalEstTokens
	for _, c := range p.Summaries {
		sum += c.EstTokens
	}
	if p.EstTokens != sum || p.FinalEstTokens <= 0 {
		t.Fatalf("est %d, want %d", p.EstTokens, sum)
	}
	if !strings.Contains(strings.Join(p.Notices, "\n"), "summarising 30 files") {
		t.Fatalf("notices = %v", p.Notices)
	}
}

func TestBuildTier2WhenCatalogExceedsBudget(t *testing.T) {
	d := buildTestDiff(40, 2, 5)
	p := Build(d, BuildOpts{CollapseMin: -1, Budget: 500, BytesPerToken: 2})
	if p.Tier != 2 {
		t.Fatalf("tier %d", p.Tier)
	}
	if !strings.Contains(p.Notices[0], "the catalog alone") {
		t.Fatalf("notices = %v", p.Notices)
	}
	final, est := p.BuildFromSummaries(d, map[int]string{0: "Adds compute calls.", 3: ""})
	assertCatalog(t, d, final)
	if !strings.Contains(final, "- dir0/file0.go: Adds compute calls.\n") || !strings.Contains(final, "- dir0/file3.go: (no summary)\n") {
		t.Fatalf("summaries missing from final prompt")
	}
	if strings.Contains(final, "HUNKS:") || strings.Contains(final, "{{") {
		t.Fatalf("final prompt has hunk bodies or placeholders")
	}
	if est != (len(final)+1)/2 {
		t.Fatalf("est = %d", est)
	}
}

func TestBuildCollapsedUnitsAndAliases(t *testing.T) {
	d := &gitdiff.Diff{}
	for i := 0; i < 8; i++ {
		p := fmt.Sprintf("i18n/lang_%d.json", i)
		f := &gitdiff.File{OldPath: p, NewPath: p, Status: gitdiff.Modified}
		h := &gitdiff.Hunk{ID: fmt.Sprintf("h%d", i+1), Index: i, FileIdx: i, File: f, Header: "@@ -3 +3 @@", Add: 1, Del: 1,
			Lines: []string{`-  "hi": "old"`, `+  "hi": "new"`}}
		f.Hunks = []*gitdiff.Hunk{h}
		d.Files, d.Hunks = append(d.Files, f), append(d.Hunks, h)
	}
	p := Build(d, BuildOpts{})
	if len(p.Units) != 1 || p.Units[0].ID != "c1" || len(p.Aliases["c1"]) != 8 {
		t.Fatalf("units %d aliases %v", len(p.Units), p.Aliases)
	}
	if !strings.Contains(p.Prompt, "c1 | M | i18n/*.json (8 files) | 8 similar hunks") {
		t.Fatalf("collapsed catalog line missing")
	}
}

func TestParseSummaries(t *testing.T) {
	got, err := ParseSummaries("Here:\n" + `{"files":[{"path":"a.go","summary":"Adds\nretry\u001b[31m."},{"path":" ","summary":"x"},{"path":"b.go","summary":""}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["a.go"] != "Adds retry [31m." {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("é", 500)
	got, _ = ParseSummaries(`{"files":[{"path":"a","summary":"` + long + `"}]}`)
	if r := []rune(got["a"]); len(r) != 400 || r[399] != '…' {
		t.Fatalf("not capped: %d runes", len(r))
	}
	if _, err := ParseSummaries("nope"); err == nil {
		t.Fatal("want error for no JSON")
	}
	if _, err := ParseSummaries(`{"files":"x"}`); err == nil {
		t.Fatal("want error for bad JSON")
	}
}
