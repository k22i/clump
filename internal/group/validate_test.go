package group

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/k22i/clump/internal/gitdiff"
)

func mkDiff(hunksPerFile ...int) *gitdiff.Diff {
	d := &gitdiff.Diff{}
	n := 0
	for fi, k := range hunksPerFile {
		f := &gitdiff.File{NewPath: fmt.Sprintf("dir%d/f%d.go", fi%2, fi), OldPath: fmt.Sprintf("dir%d/f%d.go", fi%2, fi), Status: gitdiff.Modified}
		d.Files = append(d.Files, f)
		for j := 0; j < k; j++ {
			n++
			h := &gitdiff.Hunk{ID: fmt.Sprintf("h%d", n), Index: n - 1, FileIdx: fi, File: f, Header: fmt.Sprintf("@@ -%d +%d @@", j*10+1, j*10+1), Add: 1, Del: 1, Lines: []string{"-x", "+y"}}
			f.Hunks = append(f.Hunks, h)
			d.Hunks = append(d.Hunks, h)
		}
	}
	return d
}

func mustCheck(t *testing.T, d *gitdiff.Diff, r Result) {
	t.Helper()
	if err := Check(d, r.Grouping); err != nil {
		t.Fatalf("Check: %v (groups %+v, notices %v)", err, r.Groups, r.Notices)
	}
}

type wantGroup struct {
	id    string
	title string
	desc  string
	hunks []string
}

func checkGroups(t *testing.T, got []Group, want []wantGroup) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("groups: got %d (%+v) want %d (%+v)", len(got), got, len(want), want)
	}
	for i, w := range want {
		g := got[i]
		if g.ID != w.id {
			t.Errorf("group %d ID: got %q want %q", i, g.ID, w.id)
		}
		if g.Title != w.title {
			t.Errorf("group %d title: got %q want %q", i, g.Title, w.title)
		}
		if w.desc != "" && g.Description != w.desc {
			t.Errorf("group %d description: got %q want %q", i, g.Description, w.desc)
		}
		if !slices.Equal(g.Hunks, w.hunks) {
			t.Errorf("group %d hunks: got %v want %v", i, g.Hunks, w.hunks)
		}
	}
}

func checkNotices(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !slices.Equal(got, want) {
		t.Fatalf("notices: got %q want %q", got, want)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		opts    ValidateOpts
		want    []wantGroup
		notices []string
	}{
		{
			name: "clean",
			text: `{"groups":[{"title":"A","description":"d","hunks":["h1","h2"]},{"title":"B","hunks":["h3"]}]}`,
			opts: ValidateOpts{MaxGroups: 2},
			want: []wantGroup{
				{id: "g1", title: "A", desc: "d", hunks: []string{"h1", "h2"}},
				{id: "g2", title: "B", hunks: []string{"h3"}},
			},
		},
		{
			name: "fenced with prose",
			text: "Sure!\n```json\n{\"groups\":[{\"title\":\"A\",\"hunks\":[\"h1\",\"h2\",\"h3\"]}]}\n```",
			want: []wantGroup{{id: "g1", title: "A", hunks: []string{"h1", "h2", "h3"}}},
		},
		{
			name: "out of order hunks",
			text: `{"groups":[{"title":"A","hunks":["h3","h1","h2"]}]}`,
			want: []wantGroup{{id: "g1", title: "A", hunks: []string{"h1", "h2", "h3"}}},
		},
		{
			name: "missing hunk",
			text: `{"groups":[{"title":"A","hunks":["h1"]}]}`,
			opts: ValidateOpts{MaxGroups: 2},
			want: []wantGroup{
				{id: "g1", title: "A", hunks: []string{"h1"}},
				{id: UngroupedID, title: UngroupedTitle, desc: "Hunks the model did not assign.", hunks: []string{"h2", "h3"}},
			},
			notices: []string{"repaired: 2 hunks missing from the model output; put in Ungrouped"},
		},
		{
			name: "duplicate and unknown",
			text: `{"groups":[{"title":"A","hunks":["h1","h9"]},{"title":"B","hunks":["h1","h2","h3"]}]}`,
			opts: ValidateOpts{MaxGroups: 2},
			want: []wantGroup{
				{id: "g1", title: "A", hunks: []string{"h1"}},
				{id: "g2", title: "B", hunks: []string{"h2", "h3"}},
			},
			notices: []string{
				"repaired: dropped 1 unknown hunk IDs",
				"repaired: 1 hunks were in two groups; kept the first",
			},
		},
		{
			name: "duplicate within one group",
			text: `{"groups":[{"title":"A","hunks":["h1","h1","h2","h3"]}]}`,
			want: []wantGroup{{id: "g1", title: "A", hunks: []string{"h1", "h2", "h3"}}},
			notices: []string{
				"repaired: 1 hunks were in two groups; kept the first",
			},
		},
		{
			name: "over cap",
			text: `{"groups":[{"title":"A","hunks":["h1"]},{"title":"B","hunks":["h2"]},{"title":"C","hunks":["h3"]}]}`,
			opts: ValidateOpts{MaxGroups: 2},
			want: []wantGroup{
				{id: "g1", title: "A", hunks: []string{"h1"}},
				{id: "g2", title: "Other changes", desc: "Groups beyond the cap of 2, merged.", hunks: []string{"h2", "h3"}},
			},
			notices: []string{"repaired: 3 groups exceeded the cap of 2; merged the tail"},
		},
		{
			name: "cap 1",
			text: `{"groups":[{"title":"A","hunks":["h1"]},{"title":"B","hunks":["h2","h3"]}]}`,
			opts: ValidateOpts{MaxGroups: 1},
			want: []wantGroup{
				{id: "g1", title: "Other changes", desc: "Groups beyond the cap of 1, merged.", hunks: []string{"h1", "h2", "h3"}},
			},
			notices: []string{"repaired: 2 groups exceeded the cap of 1; merged the tail"},
		},
		{
			name: "alias",
			text: `{"groups":[{"title":"A","hunks":["c1"]},{"title":"B","hunks":["h3"]}]}`,
			opts: ValidateOpts{Aliases: map[string][]string{"c1": {"h1", "h2"}}},
			want: []wantGroup{
				{id: "g1", title: "A", hunks: []string{"h1", "h2"}},
				{id: "g2", title: "B", hunks: []string{"h3"}},
			},
		},
		{
			name: "alias overlaps",
			text: `{"groups":[{"title":"A","hunks":["h1"]},{"title":"B","hunks":["c1","h3"]}]}`,
			opts: ValidateOpts{Aliases: map[string][]string{"c1": {"h1", "h2"}}},
			want: []wantGroup{
				{id: "g1", title: "A", hunks: []string{"h1"}},
				{id: "g2", title: "B", hunks: []string{"h2", "h3"}},
			},
			notices: []string{"repaired: 1 hunks were in two groups; kept the first"},
		},
		{
			name:    "non string IDs",
			text:    `{"groups":[{"title":"A","hunks":[1,"h1",null,"h2","h3"]}]}`,
			want:    []wantGroup{{id: "g1", title: "A", hunks: []string{"h1", "h2", "h3"}}},
			notices: []string{"repaired: dropped 2 unknown hunk IDs"},
		},
		{
			name: "whitespace IDs",
			text: `{"groups":[{"title":"A","hunks":[" h1 ","h2\n","h3"]}]}`,
			want: []wantGroup{{id: "g1", title: "A", hunks: []string{"h1", "h2", "h3"}}},
		},
		{
			name: "empty group dropped",
			text: `{"groups":[{"title":"Empty","hunks":[]},{"title":"A","hunks":["h1","h2","h3"]}]}`,
			want: []wantGroup{{id: "g1", title: "A", hunks: []string{"h1", "h2", "h3"}}},
		},
		{
			name: "untitled",
			text: `{"groups":[{"title":"  ","hunks":["h1","h2","h3"]}]}`,
			want: []wantGroup{{id: "g1", title: "(untitled group)", hunks: []string{"h1", "h2", "h3"}}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := mkDiff(2, 1)
			r := Validate(d, c.text, c.opts)
			mustCheck(t, d, r)
			if r.Fallback {
				t.Fatalf("unexpected fallback: %v", r.Notices)
			}
			checkGroups(t, r.Groups, c.want)
			checkNotices(t, r.Notices, c.notices)
		})
	}
}

func TestValidateDefaultCap(t *testing.T) {
	per := make([]int, 13)
	for i := range per {
		per[i] = 1
	}
	d := mkDiff(per...)

	var b strings.Builder
	b.WriteString(`{"groups":[`)
	for i := 1; i <= 13; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"title":"T%d","hunks":["h%d"]}`, i, i)
	}
	b.WriteString(`]}`)

	r := Validate(d, b.String(), ValidateOpts{})
	mustCheck(t, d, r)
	if r.Fallback {
		t.Fatalf("unexpected fallback: %v", r.Notices)
	}
	if len(r.Groups) != DefaultMaxGroups {
		t.Fatalf("groups: got %d want %d", len(r.Groups), DefaultMaxGroups)
	}
	for i := 0; i < DefaultMaxGroups-1; i++ {
		if want := fmt.Sprintf("T%d", i+1); r.Groups[i].Title != want {
			t.Errorf("group %d title: got %q want %q", i, r.Groups[i].Title, want)
		}
	}
	last := r.Groups[DefaultMaxGroups-1]
	if last.Title != "Other changes" {
		t.Errorf("last title: got %q want %q", last.Title, "Other changes")
	}
	if !slices.Equal(last.Hunks, []string{"h12", "h13"}) {
		t.Errorf("last hunks: got %v want [h12 h13]", last.Hunks)
	}
	checkNotices(t, r.Notices, []string{fmt.Sprintf("repaired: 13 groups exceeded the cap of %d; merged the tail", DefaultMaxGroups)})
}

func TestValidateFallback(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		opts       ValidateOpts
		diff       *gitdiff.Diff
		wantPrefix string
		wantGroups []wantGroup
	}{
		{
			name:       "garbage",
			text:       "I cannot help with that",
			opts:       ValidateOpts{MaxGroups: 2},
			wantPrefix: "fallback: one group per file (no JSON object in model output)",
			wantGroups: []wantGroup{
				{id: "g1", title: "dir0/f0.go", desc: "All changes to dir0/f0.go.", hunks: []string{"h1", "h2"}},
				{id: "g2", title: "dir1/f1.go", desc: "All changes to dir1/f1.go.", hunks: []string{"h3"}},
			},
		},
		{
			name:       "invalid JSON",
			text:       `{"groups":[{"title":}]}`,
			opts:       ValidateOpts{MaxGroups: 2},
			wantPrefix: "fallback: one group per file (invalid JSON: ",
		},
		{
			name:       "empty groups",
			text:       `{"groups":[]}`,
			wantPrefix: "fallback: one group per file (model returned no usable groups)",
		},
		{
			name:       "no groups key",
			text:       `{"result":"x"}`,
			wantPrefix: "fallback: one group per file (model returned no usable groups)",
		},
		{
			name:       "all unknown",
			text:       `{"groups":[{"title":"A","hunks":["h7"]}]}`,
			wantPrefix: "fallback: one group per file (model returned no usable groups)",
		},
		{
			name:       "unbalanced",
			text:       `{"groups":[`,
			wantPrefix: "fallback: one group per file (no JSON object in model output)",
		},
		{
			name:       "empty diff",
			text:       `{"groups":[]}`,
			diff:       &gitdiff.Diff{},
			wantPrefix: "fallback: one group per file (model returned no usable groups)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := c.diff
			if d == nil {
				d = mkDiff(2, 1)
			}
			r := Validate(d, c.text, c.opts)
			mustCheck(t, d, r)
			if !r.Fallback {
				t.Fatalf("want fallback, got %+v", r.Groups)
			}
			if len(r.Notices) != 1 {
				t.Fatalf("notices: got %q want exactly one fallback notice", r.Notices)
			}
			if !strings.HasPrefix(r.Notices[0], c.wantPrefix) {
				t.Fatalf("notice: got %q want prefix %q", r.Notices[0], c.wantPrefix)
			}
			if c.wantGroups != nil {
				checkGroups(t, r.Groups, c.wantGroups)
			}
		})
	}
}

func TestValidateSanitises(t *testing.T) {
	d := mkDiff(2, 1)

	t.Run("title control runes", func(t *testing.T) {
		text := `{"groups":[{"title":"Fix\u001b[31m red\nthing\t now","hunks":["h1","h2","h3"]}]}`
		r := Validate(d, text, ValidateOpts{})
		mustCheck(t, d, r)
		if got, want := r.Groups[0].Title, "Fix [31m red thing now"; got != want {
			t.Fatalf("title: got %q want %q", got, want)
		}
		assertNoControl(t, r)
	})

	t.Run("long title", func(t *testing.T) {
		long := strings.Repeat("a", 200)
		text := fmt.Sprintf(`{"groups":[{"title":"%s","hunks":["h1","h2","h3"]}]}`, long)
		r := Validate(d, text, ValidateOpts{})
		mustCheck(t, d, r)
		got := r.Groups[0].Title
		if n := len([]rune(got)); n != 80 {
			t.Fatalf("title runes: got %d want 80", n)
		}
		if !strings.HasSuffix(got, "\u2026") {
			t.Fatalf("title: got %q want ellipsis suffix", got)
		}
		if want := strings.Repeat("a", 79) + "\u2026"; got != want {
			t.Fatalf("title: got %q want %q", got, want)
		}
	})

	t.Run("exactly 80 rune title is kept", func(t *testing.T) {
		ok := strings.Repeat("b", 80)
		text := fmt.Sprintf(`{"groups":[{"title":"%s","hunks":["h1","h2","h3"]}]}`, ok)
		r := Validate(d, text, ValidateOpts{})
		mustCheck(t, d, r)
		if got := r.Groups[0].Title; got != ok {
			t.Fatalf("title: got %q want it unchanged", got)
		}
	})

	t.Run("description lines", func(t *testing.T) {
		text := `{"groups":[{"title":"A","description":"line one\n\n  line\ttwo  \n","hunks":["h1","h2","h3"]}]}`
		r := Validate(d, text, ValidateOpts{})
		mustCheck(t, d, r)
		if got, want := r.Groups[0].Description, "line one\n\nline two"; got != want {
			t.Fatalf("description: got %q want %q", got, want)
		}
		assertNoControl(t, r)
	})

	t.Run("long description", func(t *testing.T) {
		long := strings.Repeat("c", 3000)
		text := fmt.Sprintf(`{"groups":[{"title":"A","description":"%s","hunks":["h1","h2","h3"]}]}`, long)
		r := Validate(d, text, ValidateOpts{})
		mustCheck(t, d, r)
		got := r.Groups[0].Description
		if n := len([]rune(got)); n != 1000 {
			t.Fatalf("description runes: got %d want 1000", n)
		}
		if want := strings.Repeat("c", 999) + "\u2026"; got != want {
			t.Fatalf("description: got %q want %q", got, want)
		}
	})

	t.Run("description control runes", func(t *testing.T) {
		text := `{"groups":[{"title":"A","description":"a\u0000b\r\u001b[1mc","hunks":["h1","h2","h3"]}]}`
		r := Validate(d, text, ValidateOpts{})
		mustCheck(t, d, r)
		assertNoControl(t, r)
	})
}

func assertNoControl(t *testing.T, r Result) {
	t.Helper()
	for _, g := range r.Groups {
		for _, ru := range g.Title {
			if unicode.IsControl(ru) {
				t.Errorf("group %q title has control rune %U: %q", g.ID, ru, g.Title)
			}
		}
		for _, ru := range g.Description {
			if ru == '\n' {
				continue
			}
			if unicode.IsControl(ru) {
				t.Errorf("group %q description has control rune %U: %q", g.ID, ru, g.Description)
			}
		}
	}
}

func TestFallbackPerFile(t *testing.T) {
	d := mkDiff(2, 1, 3)
	r := FallbackPerFile(d, "x")
	mustCheck(t, d, r)
	if !r.Fallback {
		t.Fatal("Fallback: got false want true")
	}
	checkGroups(t, r.Groups, []wantGroup{
		{id: "g1", title: "dir0/f0.go", desc: "All changes to dir0/f0.go.", hunks: []string{"h1", "h2"}},
		{id: "g2", title: "dir1/f1.go", desc: "All changes to dir1/f1.go.", hunks: []string{"h3"}},
		{id: "g3", title: "dir0/f2.go", desc: "All changes to dir0/f2.go.", hunks: []string{"h4", "h5", "h6"}},
	})
	checkNotices(t, r.Notices, []string{"fallback: one group per file (x)"})

	t.Run("renamed file", func(t *testing.T) {
		rd := mkDiff(1)
		rd.Files[0].OldPath = "old.txt"
		rd.Files[0].NewPath = "new.txt"
		rd.Files[0].Status = gitdiff.Renamed
		rr := FallbackPerFile(rd, "why")
		mustCheck(t, rd, rr)
		checkGroups(t, rr.Groups, []wantGroup{
			{id: "g1", title: "old.txt -> new.txt", desc: "All changes to old.txt -> new.txt.", hunks: []string{"h1"}},
		})
	})

	t.Run("file without hunks is skipped", func(t *testing.T) {
		sd := mkDiff(1)
		sd.Files = append(sd.Files, &gitdiff.File{NewPath: "dir0/empty.go", OldPath: "dir0/empty.go", Status: gitdiff.Modified})
		rr := FallbackPerFile(sd, "why")
		mustCheck(t, sd, rr)
		if len(rr.Groups) != 1 || rr.Groups[0].Title != "dir0/f0.go" {
			t.Fatalf("groups: got %+v want one group for dir0/f0.go", rr.Groups)
		}
	})
}

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"plain object", `{"a":1}`, `{"a":1}`, true},
		{"prose before and after", "Here you go:\n" + `{"a":1}` + "\nthanks", `{"a":1}`, true},
		{"nested braces", `{"a":{"b":{}},"c":1}`, `{"a":{"b":{}},"c":1}`, true},
		{"braces inside strings", `{"a":"}{"}`, `{"a":"}{"}`, true},
		{"escaped quote inside string", `{"a":"\"}"}`, `{"a":"\"}"}`, true},
		{"two objects", `{"a":1}{"b":2}`, `{"a":1}`, true},
		{"none", `no json here`, "", false},
		{"empty", "", "", false},
		{"unbalanced", `{"a":1`, "", false},
		{"brace before first object", `} then {"a":1}`, `{"a":1}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ExtractJSON(c.in)
			if ok != c.ok {
				t.Fatalf("ok: got %v want %v (got %q)", ok, c.ok, got)
			}
			if got != c.want {
				t.Fatalf("object: got %q want %q", got, c.want)
			}
		})
	}
}

func TestValidateProperty(t *testing.T) {
	seed := uint64(time.Now().UnixNano())
	if s := os.Getenv("CLUMP_SEED"); s != "" {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			t.Fatalf("CLUMP_SEED=%q: %v", s, err)
		}
		seed = v
	}
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewPCG(seed, seed))

	const iters = 2000
	fallbacks, nonFallbacks, merges, missings := 0, 0, 0, 0
	for i := 0; i < iters; i++ {
		nf := 1 + rng.IntN(8)
		per := make([]int, nf)
		for j := range per {
			per[j] = 1 + rng.IntN(4)
		}
		d := mkDiff(per...)

		realIDs := make([]string, len(d.Hunks))
		for j, h := range d.Hunks {
			realIDs[j] = h.ID
		}

		aliases := map[string][]string{}
		for a := 0; a < rng.IntN(3); a++ {
			n := 2 + rng.IntN(3)
			members := make([]string, n)
			for j := range members {
				members[j] = realIDs[rng.IntN(len(realIDs))]
			}
			aliases[fmt.Sprintf("c%d", a+1)] = members
		}

		text := randomModelText(rng, realIDs, aliases)
		cap := 1 + rng.IntN(6)

		r := Validate(d, text, ValidateOpts{MaxGroups: cap, Aliases: aliases})
		fail := func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			t.Fatalf("seed %d iteration %d cap %d aliases %v: %s\ntext: %q\ngroups: %+v\nnotices: %v\nfallback: %v",
				seed, i, cap, aliases, msg, text, r.Groups, r.Notices, r.Fallback)
		}

		if err := Check(d, r.Grouping); err != nil {
			fail("Check: %v", err)
		}
		if !r.Fallback {
			n := 0
			for _, g := range r.Groups {
				if g.ID != UngroupedID {
					n++
				}
			}
			if n > cap {
				fail("group count %d exceeds cap %d", n, cap)
			}
		}
		assertGroupShape(t, r, func(format string, args ...any) { fail(format, args...) })

		if r.Fallback {
			fallbacks++
		} else {
			nonFallbacks++
		}
		for _, n := range r.Notices {
			if strings.Contains(n, "exceeded the cap") {
				merges++
			}
			if strings.Contains(n, "missing from the model output") {
				missings++
			}
		}
	}

	t.Logf("seed %d: %d fallbacks, %d repaired, %d cap merges, %d missing-hunk repairs", seed, fallbacks, nonFallbacks, merges, missings)
	if fallbacks == 0 || nonFallbacks == 0 || merges == 0 || missings == 0 {
		t.Fatalf("seed %d: iterations did not cover every path (fallbacks %d, repaired %d, merges %d, missing %d)",
			seed, fallbacks, nonFallbacks, merges, missings)
	}
}

func assertGroupShape(t *testing.T, r Result, fail func(string, ...any)) {
	t.Helper()
	k := 0
	for gi, g := range r.Groups {
		if g.ID == UngroupedID {
			if gi != len(r.Groups)-1 {
				fail("ungrouped group at index %d of %d", gi, len(r.Groups))
			}
			continue
		}
		k++
		if want := fmt.Sprintf("g%d", k); g.ID != want {
			fail("group %d ID: got %q want %q", gi, g.ID, want)
		}
		for _, ru := range g.Title {
			if unicode.IsControl(ru) {
				fail("group %q title has control rune %U", g.ID, ru)
			}
		}
		if n := len([]rune(g.Title)); n > 80 {
			fail("group %q title is %d runes", g.ID, n)
		}
		if strings.TrimSpace(g.Title) == "" {
			fail("group %q has an empty title", g.ID)
		}
		for _, ru := range g.Description {
			if ru != '\n' && unicode.IsControl(ru) {
				fail("group %q description has control rune %U", g.ID, ru)
			}
		}
		if n := len([]rune(g.Description)); n > 1000 {
			fail("group %q description is %d runes", g.ID, n)
		}
	}
}

func randomModelText(rng *rand.Rand, realIDs []string, aliases map[string][]string) string {
	if rng.IntN(10) == 0 {
		return randomGarbage(rng)
	}

	pool := append([]string(nil), realIDs...)
	aliasIDs := make([]string, 0, len(aliases))
	for id := range aliases {
		aliasIDs = append(aliasIDs, id)
	}
	slices.Sort(aliasIDs)
	pool = append(pool, aliasIDs...)
	pool = append(pool, "h0", "h99", "zz")

	groups := make([]map[string]any, 0, rng.IntN(9))
	for i := 0; i < rng.IntN(9); i++ {
		g := map[string]any{}
		if rng.IntN(6) != 0 {
			g["title"] = randomText(rng)
		}
		if rng.IntN(2) == 0 {
			g["description"] = randomText(rng)
		}
		hunks := make([]any, 0, 6)
		for j := 0; j < rng.IntN(7); j++ {
			switch rng.IntN(10) {
			case 0:
				hunks = append(hunks, nil)
			case 1:
				hunks = append(hunks, rng.IntN(50))
			default:
				hunks = append(hunks, pool[rng.IntN(len(pool))])
			}
		}
		g["hunks"] = hunks
		groups = append(groups, g)
	}

	b, err := json.Marshal(map[string]any{"groups": groups})
	if err != nil {
		return "{}"
	}
	s := string(b)
	switch rng.IntN(6) {
	case 0:
		return "Sure! Here is the grouping:\n```json\n" + s + "\n```\n"
	case 1:
		if len(s) > 1 {
			s = s[:rng.IntN(len(s))]
		}
	case 2:
		return "```\n" + s + "\n```"
	case 3:
		return "Result: " + s + " (end)"
	}
	return s
}

func randomText(rng *rand.Rand) string {
	alphabet := []rune("abcXYZ \t\n\x1b\x00\x7fé中…_-[]{}@#/\"\\")
	n := rng.IntN(100)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(alphabet[rng.IntN(len(alphabet))])
	}
	return b.String()
}

func randomGarbage(rng *rand.Rand) string {
	alphabet := []rune("abc XYZ,.;:!?\n\t")
	n := rng.IntN(60)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(alphabet[rng.IntN(len(alphabet))])
	}
	if rng.IntN(2) == 0 {
		b.WriteString("{\"groups\":[")
	}
	return b.String()
}

func TestValidateNeverPanics(t *testing.T) {
	seed := uint64(time.Now().UnixNano())
	if s := os.Getenv("CLUMP_SEED"); s != "" {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			t.Fatalf("CLUMP_SEED=%q: %v", s, err)
		}
		seed = v
	}
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b9))

	for i := 0; i < 1000; i++ {
		var b strings.Builder
		if rng.IntN(2) == 0 {
			b.WriteString(`{"groups":[{"title":"A","hunks":["h1"`)
		}
		for j := 0; j < rng.IntN(200); j++ {
			switch rng.IntN(4) {
			case 0:
				b.WriteByte(byte(rng.IntN(256)))
			default:
				b.WriteRune(rune(1 + rng.IntN(0x2FF)))
			}
		}
		nf := 1 + rng.IntN(3)
		per := make([]int, nf)
		for j := range per {
			per[j] = 1 + rng.IntN(3)
		}
		d := mkDiff(per...)
		text := b.String()
		r := Validate(d, text, ValidateOpts{MaxGroups: rng.IntN(8)})
		if err := Check(d, r.Grouping); err != nil {
			t.Fatalf("seed %d iteration %d: Check: %v\ntext: %q\ngroups: %+v", seed, i, err, text, r.Groups)
		}
	}
}
