package provider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFakeResponsesInOrder(t *testing.T) {
	f := &Fake{ProviderName: "scripted", Responses: []string{"a", "b"}}
	ctx := context.Background()

	for i, want := range []string{"a", "b", "b"} {
		text, _, err := f.Complete(ctx, "prompt-"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if text != want {
			t.Fatalf("call %d = %q, want %q", i, text, want)
		}
	}
	if got := f.Calls(); got != 3 {
		t.Fatalf("Calls() = %d, want 3", got)
	}
	want := []string{"prompt-0", "prompt-1", "prompt-2"}
	got := f.Prompts()
	if len(got) != len(want) {
		t.Fatalf("Prompts() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Prompts() = %q, want %q", got, want)
		}
	}
	if f.Name() != "scripted" {
		t.Fatalf("Name() = %q, want \"scripted\"", f.Name())
	}
}

func TestFakeErrAndDelay(t *testing.T) {
	t.Run("err", func(t *testing.T) {
		boom := errors.New("boom")
		f := &Fake{Err: boom}
		text, u, err := f.Complete(context.Background(), "x")
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want boom", err)
		}
		if text != "" || u != (Usage{}) {
			t.Fatalf("text = %q u = %+v, want zero values", text, u)
		}
		if f.Calls() != 1 {
			t.Fatalf("Calls() = %d, want the failing call recorded", f.Calls())
		}
	})

	t.Run("delay cancelled", func(t *testing.T) {
		f := &Fake{Delay: 2 * time.Second, Responses: []string{"late"}}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, _, err := f.Complete(ctx, "x")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want context.DeadlineExceeded", err)
		}
		if waited := time.Since(start); waited > time.Second {
			t.Fatalf("wait took %v, want it to end with the context", waited)
		}
	})

	t.Run("delay then answer", func(t *testing.T) {
		f := &Fake{Delay: 5 * time.Millisecond, Responses: []string{"late"}}
		text, _, err := f.Complete(context.Background(), "x")
		if err != nil || text != "late" {
			t.Fatalf("Complete = %q, %v", text, err)
		}
	})
}

func TestFakeConcurrent(t *testing.T) {
	f := &Fake{Responses: []string{"r"}}
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := f.Complete(ctx, "concurrent"); err != nil {
				t.Errorf("Complete: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := f.Calls(); got != 50 {
		t.Fatalf("Calls() = %d, want 50", got)
	}
	if got := len(f.Prompts()); got != 50 {
		t.Fatalf("len(Prompts()) = %d, want 50", got)
	}
}

func TestFakeDefaultUsage(t *testing.T) {
	f := &Fake{}
	prompt := strings.Repeat("x", 100)

	text, u, err := f.Complete(context.Background(), prompt)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if u.InputTokens != 50 {
		t.Fatalf("InputTokens = %d, want 50", u.InputTokens)
	}
	if want := len(text) / 2; u.OutputTokens != want {
		t.Fatalf("OutputTokens = %d, want %d", u.OutputTokens, want)
	}
	if u.CostUSD != 0 || u.CostSource != CostEstimated {
		t.Fatalf("usage = %+v, want a free estimate", u)
	}
}

func promptTemplate(t *testing.T, name string, fills map[string]string) string {
	t.Helper()
	path := filepath.Join("..", "group", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := string(raw)
	for placeholder, value := range fills {
		out = strings.ReplaceAll(out, placeholder, value)
	}
	return out
}

func TestAutoAnswerGroup(t *testing.T) {
	catalog := strings.Join([]string{
		"h1 | M | src/a.go | @@ -1,3 +1,4 @@ | +2 -1",
		"h2 | A | src/b.go | @@ -0,0 +1,9 @@ | +9 -0",
		"h3 | M | README.md | @@ -2,2 +2,2 @@ | +1 -1",
		"h4 | R | old.txt -> docs/new.txt | @@ -1,1 +1,1 @@ | +1 -1",
		"c1 | M | docs/*.md (6 files) | 6 similar hunks, e.g. @@ -3,3 +3,3 @@ | +6 -6",
	}, "\n")
	prompt := promptTemplate(t, "prompt_v1.txt", map[string]string{
		"{{NONCE}}":      "0123456789ab",
		"{{MAX_GROUPS}}": "12",
		"{{CONTEXT}}":    "branch: main",
		"{{CATALOG}}":    catalog,
		"{{HUNKS}}":      "### h1 src/a.go\n@@ -1,3 +1,4 @@\n-a\n+b",
	})

	answer := AutoAnswer(prompt)
	if strings.HasPrefix(taskLine(prompt), groupTaskPrefix) == false {
		t.Fatalf("test prompt is not a grouping prompt: %q", taskLine(prompt))
	}

	var got fakeGroups
	if err := json.Unmarshal([]byte(answer), &got); err != nil {
		t.Fatalf("AutoAnswer is not valid JSON (%v): %s", err, answer)
	}
	want := fakeGroups{Groups: []fakeGroup{
		{Title: "Changes in src", Description: "Fake grouping of 2 hunks in src.", Hunks: []string{"h1", "h2"}},
		{Title: "Changes in (root)", Description: "Fake grouping of 1 hunks in (root).", Hunks: []string{"h3"}},
		{Title: "Changes in docs", Description: "Fake grouping of 2 hunks in docs.", Hunks: []string{"h4", "c1"}},
	}}
	if len(got.Groups) != len(want.Groups) {
		t.Fatalf("groups = %+v, want %+v", got.Groups, want.Groups)
	}
	for i := range want.Groups {
		g, w := got.Groups[i], want.Groups[i]
		if g.Title != w.Title || g.Description != w.Description {
			t.Fatalf("group %d = %+v, want %+v", i, g, w)
		}
		if strings.Join(g.Hunks, ",") != strings.Join(w.Hunks, ",") {
			t.Fatalf("group %d hunks = %v, want %v", i, g.Hunks, w.Hunks)
		}
	}
}

func TestAutoAnswerSummarize(t *testing.T) {
	files := strings.Join([]string{
		"=== file a.go (M, +1 -0, 1 hunks)",
		"@@ -1,0 +1,1 @@",
		"+package a",
		"",
		"=== file docs/guide.md (M, +2 -2, 2 hunks)",
		"@@ -1,2 +1,2 @@",
		"-old",
		"+new",
	}, "\n")
	prompt := promptTemplate(t, "prompt_summary_v1.txt", map[string]string{
		"{{NONCE}}":   "0123456789ab",
		"{{CONTEXT}}": "branch: main",
		"{{FILES}}":   files,
	})

	answer := AutoAnswer(prompt)
	var got fakeSummaries
	if err := json.Unmarshal([]byte(answer), &got); err != nil {
		t.Fatalf("AutoAnswer is not valid JSON (%v): %s", err, answer)
	}
	want := []fakeFileSummary{
		{Path: "a.go", Summary: "Fake summary of a.go."},
		{Path: "docs/guide.md", Summary: "Fake summary of docs/guide.md."},
	}
	if len(got.Files) != len(want) {
		t.Fatalf("files = %+v, want %+v", got.Files, want)
	}
	for i := range want {
		if got.Files[i] != want[i] {
			t.Fatalf("file %d = %+v, want %+v", i, got.Files[i], want[i])
		}
	}
}

func TestAutoAnswerOther(t *testing.T) {
	for _, prompt := range []string{"hello", "", "# clump-task: something else", "no data markers at all"} {
		if got := AutoAnswer(prompt); got != `{"groups":[]}` {
			t.Fatalf("AutoAnswer(%q) = %q, want {\"groups\":[]}", prompt, got)
		}
	}
}

func TestAutoAnswerGroupWithoutData(t *testing.T) {
	prompt := "# clump-task: group v1\nnothing between the markers\n"
	if got := AutoAnswer(prompt); got != `{"groups":[]}` {
		t.Fatalf("AutoAnswer = %q, want an empty group list", got)
	}
}

func TestTaskLineAndPromptData(t *testing.T) {
	prompt := "  # clump-task: group v1  \nintro\n<<<CLUMP-DATA-abc>>>\nline one\nline two\n<<<END-CLUMP-DATA-abc>>>\ntrailing\n"
	if got := taskLine(prompt); got != groupTaskPrefix+" v1" {
		t.Fatalf("taskLine = %q", got)
	}
	if got := promptData(prompt); got != "line one\nline two" {
		t.Fatalf("promptData = %q", got)
	}
	open := "# clump-task: group v1\n<<<CLUMP-DATA-abc>>>\nonly line\n"
	if got := promptData(open); got != "only line\n" {
		t.Fatalf("promptData without a closing marker = %q", got)
	}
	if got := promptData("no markers"); got != "" {
		t.Fatalf("promptData without markers = %q, want empty", got)
	}
}
