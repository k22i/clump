package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/cache"
	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/provider"
)

func testDiff(t *testing.T, files, hunks, lines int) *gitdiff.Diff {
	t.Helper()
	var b strings.Builder
	for f := 0; f < files; f++ {
		fmt.Fprintf(&b, "diff --git a/d%d/f%d.go b/d%d/f%d.go\n--- a/d%d/f%d.go\n+++ b/d%d/f%d.go\n", f%3, f, f%3, f, f%3, f, f%3, f)
		for h := 0; h < hunks; h++ {
			fmt.Fprintf(&b, "@@ -%d,0 +%d,%d @@ func f%d()\n", h*100+1, h*100+1, lines+f, h)
			for l := 0; l < lines+f; l++ {
				fmt.Fprintf(&b, "+\tvalue%d := compute(%d, %d, %d)\n", l, f, h, l)
			}
		}
	}
	d, err := gitdiff.Parse(b.String())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

type fixture struct {
	e    *Engine
	fake *provider.Fake
}

func newFixture(t *testing.T, mut func(*config.Config)) fixture {
	t.Helper()
	cfg := config.Default()
	cfg.Providers = append(cfg.Providers, provider.Config{Name: "fake", Type: provider.TypeFake, InputPrice: 1, OutputPrice: 2, BytesPerToken: 2})
	cfg.Cheap = "fake"
	if mut != nil {
		mut(cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	reg, err := cfg.Registry()
	if err != nil {
		t.Fatal(err)
	}
	fake := &provider.Fake{ProviderName: "fake"}
	if err := reg.Set("fake", fake); err != nil {
		t.Fatal(err)
	}
	store, err := cache.Open(cache.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{New(cfg, reg, store), fake}
}

func TestRunCachesAndLookupHits(t *testing.T) {
	fx := newFixture(t, nil)
	d := testDiff(t, 4, 2, 3)
	if _, ok, err := fx.e.Lookup(d, "cheap"); err != nil || ok {
		t.Fatalf("lookup before run: ok=%v err=%v", ok, err)
	}
	j, err := fx.e.Prepare(Input{Diff: d, Context: "COMMIT MESSAGE:\nx"}, "cheap")
	if err != nil {
		t.Fatal(err)
	}
	if j.Provider != "fake" || j.Est.Calls != 1 || j.Est.Tier != 1 || j.Est.Tokens <= 0 || j.Est.CostUSD <= 0 {
		t.Fatalf("job = %+v", j.Est)
	}
	out, err := fx.e.Run(context.Background(), j, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Cached || out.Calls != 1 || fx.fake.Calls() != 1 {
		t.Fatalf("outcome %+v calls %d", out, fx.fake.Calls())
	}
	if err := group.Check(d, out.Result.Grouping); err != nil {
		t.Fatal(err)
	}
	if len(out.Result.Groups) != 3 { // AutoAnswer: one group per top-level dir d0, d1, d2
		t.Fatalf("groups = %d", len(out.Result.Groups))
	}
	hit, ok, err := fx.e.Lookup(d, "cheap")
	if err != nil || !ok || !hit.Cached || hit.Key != j.Key || !group.Equal(hit.Result.Grouping, out.Result.Grouping) {
		t.Fatalf("lookup after run: ok=%v err=%v", ok, err)
	}
	if fx.fake.Calls() != 1 {
		t.Fatalf("lookup made a call")
	}
	// A different provider or max_groups misses.
	if _, ok, _ := fx.e.Lookup(d, "strong"); ok {
		t.Fatal("other provider hit")
	}
}

func TestRunProviderErrorCachesNothing(t *testing.T) {
	fx := newFixture(t, nil)
	fx.fake.Err = errors.New("boom")
	d := testDiff(t, 1, 1, 1)
	j, _ := fx.e.Prepare(Input{Diff: d}, "cheap")
	if _, err := fx.e.Run(context.Background(), j, nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
	if _, ok, _ := fx.e.Lookup(d, "cheap"); ok {
		t.Fatal("failed run was cached")
	}
}

func TestRunGarbageFallsBackAndIsValid(t *testing.T) {
	fx := newFixture(t, nil)
	fx.fake.Responses = []string{"sorry"}
	d := testDiff(t, 3, 1, 1)
	j, _ := fx.e.Prepare(Input{Diff: d}, "cheap")
	out, err := fx.e.Run(context.Background(), j, nil)
	if err != nil || !out.Result.Fallback || len(out.Result.Groups) != 3 || group.Check(d, out.Result.Grouping) != nil {
		t.Fatalf("out = %+v err %v", out, err)
	}
}

func TestOverCeiling(t *testing.T) {
	fx := newFixture(t, func(c *config.Config) { c.Ceiling = 100 })
	j, _ := fx.e.Prepare(Input{Diff: testDiff(t, 3, 2, 5)}, "cheap")
	if !j.Est.OverCeiling {
		t.Fatalf("est %+v", j.Est)
	}
	if !strings.HasPrefix(j.Est.String(), "~") || !strings.Contains(j.Est.String(), "tokens, ~$") {
		t.Fatalf("string = %q", j.Est.String())
	}
}

func TestTier2SummariesCachedPerFile(t *testing.T) {
	fx := newFixture(t, func(c *config.Config) { c.Budget = 1000 })
	d := testDiff(t, 12, 2, 40)
	j, err := fx.e.Prepare(Input{Diff: d}, "cheap")
	if err != nil {
		t.Fatal(err)
	}
	if j.Plan.Tier != 2 || j.Est.Calls != len(j.Plan.Summaries)+1 {
		t.Fatalf("tier %d calls %d summaries %d", j.Plan.Tier, j.Est.Calls, len(j.Plan.Summaries))
	}
	var steps []Progress
	out, err := fx.e.Run(context.Background(), j, func(p Progress) { steps = append(steps, p) })
	if err != nil {
		t.Fatal(err)
	}
	if out.Calls != j.Est.Calls || fx.fake.Calls() != j.Est.Calls || len(steps) != out.Calls || steps[len(steps)-1].What != "grouping" {
		t.Fatalf("calls %d fake %d steps %v", out.Calls, fx.fake.Calls(), steps)
	}
	last := fx.fake.Prompts()[fx.fake.Calls()-1]
	if !strings.Contains(last, "- d0/f0.go: Fake summary of d0/f0.go.") {
		t.Fatalf("final prompt lacks summaries")
	}
	if err := group.Check(d, out.Result.Grouping); err != nil {
		t.Fatal(err)
	}
	// Regrouping bypassing the result cache reuses every summary: one call.
	j2, _ := fx.e.Prepare(Input{Diff: d}, "cheap")
	if j2.Est.Calls != 1 {
		t.Fatalf("estimate after summaries cached: %d calls", j2.Est.Calls)
	}
	before := fx.fake.Calls()
	if _, err := fx.e.Run(context.Background(), j2, nil); err != nil {
		t.Fatal(err)
	}
	if fx.fake.Calls()-before != 1 {
		t.Fatalf("second run made %d calls", fx.fake.Calls()-before)
	}
}

func TestCalibrationSkipsFakeButUsesReportedCounts(t *testing.T) {
	fx := newFixture(t, nil)
	fx.e.calibrate("fake", provider.Config{Type: provider.TypeFake}, 10000, provider.Usage{InputTokens: 4000})
	if _, ok := fx.e.Store.Calibration("fake"); ok {
		t.Fatal("fake provider was calibrated")
	}
	pc := provider.Config{Type: provider.TypeCommand, Format: provider.FormatClaudeJSON, OverheadTokens: 650}
	fx.e.calibrate("claude-haiku", pc, 2100, provider.Usage{InputTokens: 1650})
	if v, ok := fx.e.Store.Calibration("claude-haiku"); !ok || v < 2.09 || v > 2.11 {
		t.Fatalf("calibration = %v %v", v, ok)
	}
	if got := fx.e.BytesPerToken("claude-haiku", pc); got < 2.09 || got > 2.11 {
		t.Fatalf("BytesPerToken = %v", got)
	}
}

func TestDryRun(t *testing.T) {
	fx := newFixture(t, nil)
	d := testDiff(t, 2, 1, 2)
	j, _ := fx.e.Prepare(Input{Diff: d}, "cheap")
	s := fx.e.DryRun(j)
	if !strings.Contains(s, "# ---- request 1: grouping ----\n# clump-task: group v1\n") || !strings.Contains(s, "provider fake, tier 1") {
		t.Fatalf("dry run:\n%s", s)
	}
	if fx.fake.Calls() != 0 {
		t.Fatal("dry run called the provider")
	}
	fx2 := newFixture(t, func(c *config.Config) { c.Budget = 1000 })
	j2, _ := fx2.e.Prepare(Input{Diff: testDiff(t, 12, 2, 40)}, "cheap")
	s2 := fx2.e.DryRun(j2)
	if !strings.Contains(s2, "summary chunk 1/") || !strings.Contains(s2, "(summary pending)") || fx2.fake.Calls() != 0 {
		t.Fatalf("tier 2 dry run wrong")
	}
}

func TestUnknownProvider(t *testing.T) {
	fx := newFixture(t, nil)
	if _, err := fx.e.Prepare(Input{Diff: testDiff(t, 1, 1, 1)}, "nope"); err == nil {
		t.Fatal("want error")
	}
}
