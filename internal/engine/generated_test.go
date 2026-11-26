package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/config"
	"github.com/k22i/clump/internal/group"
)

func TestGeneratedFilesAreNotSent(t *testing.T) {
	fx := newFixture(t, func(c *config.Config) { c.Generated.Patterns = []string{"d1/"} })
	d := testDiff(t, 3, 1, 2)
	plainKey, _, _, err := fx.e.ResultKey(d, "cheap")
	if err != nil {
		t.Fatal(err)
	}
	fx.e.MarkGenerated(context.Background(), d, "")
	if d.Files[1].Generated != "pattern d1/" || d.Files[0].Generated != "" {
		t.Fatalf("marks = %q %q", d.Files[0].Generated, d.Files[1].Generated)
	}
	key, _, _, _ := fx.e.ResultKey(d, "cheap")
	if key == plainKey {
		t.Fatal("the result key ignores the generated files")
	}
	j, err := fx.e.Prepare(Input{Diff: d}, "cheap")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(j.Plan.Prompt, "### h2 ") {
		t.Fatal("generated hunk sent")
	}
	if dr := fx.e.DryRun(j); !strings.Contains(dr, "# not sent (generated, pattern d1/): d1/f1.go") {
		t.Fatalf("dry run:\n%s", dr)
	}
	out, err := fx.e.Run(context.Background(), j, nil)
	if err != nil {
		t.Fatal(err)
	}
	gs := out.Result.Groups
	if last := gs[len(gs)-1]; last.ID != group.GeneratedID || strings.Join(last.Hunks, ",") != "h2" {
		t.Fatalf("groups = %+v", gs)
	}
	if err := group.Check(d, out.Result.Grouping); err != nil {
		t.Fatal(err)
	}
}

func TestAllGeneratedMakesNoCall(t *testing.T) {
	fx := newFixture(t, func(c *config.Config) { c.Generated.Patterns = []string{"*.go"} })
	d := testDiff(t, 2, 1, 2)
	fx.e.MarkGenerated(context.Background(), d, "")
	j, err := fx.e.Prepare(Input{Diff: d}, "cheap")
	if err != nil {
		t.Fatal(err)
	}
	if j.Est.Calls != 0 || j.Est.Tokens != 0 || j.Est.OverCeiling {
		t.Fatalf("estimate = %+v", j.Est)
	}
	out, err := fx.e.Run(context.Background(), j, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Calls != 0 || len(fx.fake.Prompts()) != 0 {
		t.Fatalf("calls = %d", out.Calls)
	}
	if len(out.Result.Groups) != 1 || out.Result.Groups[0].ID != group.GeneratedID {
		t.Fatalf("groups = %+v", out.Result.Groups)
	}
}
