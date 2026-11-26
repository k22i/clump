package forge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/comment"
	"github.com/k22i/clump/internal/gitdiff"
)

// forgePatch has one modified file, one rename and one deleted file, to map
// comments from every kind of file onto the PR's paths.
const forgePatch = `diff --git a/src/app.go b/src/app.go
--- a/src/app.go
+++ b/src/app.go
@@ -1,4 +1,4 @@
 package main
 
-foo
 func New() {}
+func Added() {}
diff --git a/old_name.txt b/new_name.txt
similarity index 95%
rename from old_name.txt
rename to new_name.txt
--- a/old_name.txt
+++ b/new_name.txt
@@ -1,3 +1,3 @@
 context one
-removed
+renamed
 context two
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-line one
-line two
`

func parsePatch(t *testing.T) *gitdiff.Diff {
	t.Helper()
	d, err := gitdiff.Parse(forgePatch)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func TestParseEvent(t *testing.T) {
	for in, want := range map[string]Event{
		"comment":         EventComment,
		"COMMENT":         EventComment,
		" approve ":       EventApprove,
		"request-changes": EventRequestChange,
		"request_changes": EventRequestChange,
		"request changes": EventRequestChange,
	} {
		got, err := ParseEvent(in)
		if err != nil || got != want {
			t.Errorf("ParseEvent(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "draft", "pending", "leave", "open"} {
		if _, err := ParseEvent(in); err == nil {
			t.Errorf("ParseEvent(%q) accepted", in)
		}
	}
}

func TestEventString(t *testing.T) {
	if EventComment.String() != "comment" || EventApprove.String() != "approve" ||
		EventRequestChange.String() != "request changes" {
		t.Fatal("unexpected Event.String")
	}
}

func TestBuild(t *testing.T) {
	d := parsePatch(t)
	placed := []comment.Placed{
		// An added line: RIGHT, the file's current path.
		{Kind: comment.KindLine, Path: "src/app.go", Side: comment.SideNew, Line: 4, Text: "Wrap it."},
		// A removed line: LEFT, the file's old path.
		{Kind: comment.KindLine, Path: "src/app.go", Side: comment.SideOld, Line: 3, Text: "Why is it gone?"},
		// Removed in a renamed file: LEFT is the old name.
		{Kind: comment.KindLine, Path: "old_name.txt -> new_name.txt", Side: comment.SideOld, Line: 2, Text: "Check the rename."},
		// Added in a renamed file: RIGHT is the new name.
		{Kind: comment.KindLine, Path: "old_name.txt -> new_name.txt", Side: comment.SideNew, Line: 2, Text: "Renamed how?"},
		// A line of a deleted file: LEFT, and the path stays the old one.
		{Kind: comment.KindLine, Path: "gone.txt", Side: comment.SideOld, Line: 1, Text: "Was it used?"},
		// A file comment: subject_type file.
		{Kind: comment.KindFile, Path: "src/app.go", Text: "This file needs tests."},
		// A group comment: into the body.
		{Kind: comment.KindGroup, GroupID: "1", Text: "Ship it."},
	}
	r := Build(EventRequestChange, "Please have a look.", placed, d, func(id string) string {
		return "the fix"
	})
	if r.Event != EventRequestChange {
		t.Errorf("Event = %s", r.Event)
	}
	want := []Inline{
		{Path: "src/app.go", Side: "RIGHT", Line: 4, Body: "Wrap it."},
		{Path: "src/app.go", Side: "LEFT", Line: 3, Body: "Why is it gone?"},
		{Path: "old_name.txt", Side: "LEFT", Line: 2, Body: "Check the rename."},
		{Path: "new_name.txt", Side: "RIGHT", Line: 2, Body: "Renamed how?"},
		{Path: "gone.txt", Side: "LEFT", Line: 1, Body: "Was it used?"},
		{Path: "src/app.go", SubjectType: "file", Body: "This file needs tests."},
	}
	if len(r.Comments) != len(want) {
		t.Fatalf("comments = %d, want %d: %+v", len(r.Comments), len(want), r.Comments)
	}
	for i, w := range want {
		if r.Comments[i] != w {
			t.Errorf("comment %d = %+v, want %+v", i, r.Comments[i], w)
		}
	}
	wantBody := "Please have a look.\n\n## Group comments\n1. the fix: Ship it."
	if r.Body != wantBody {
		t.Errorf("Body = %q, want %q", r.Body, wantBody)
	}
	if r.Counts != (Counts{Line: 5, File: 1, Group: 1}) {
		t.Errorf("Counts = %+v", r.Counts)
	}
}

func TestBuildEmpty(t *testing.T) {
	r := Build(EventApprove, "", nil, parsePatch(t), nil)
	if len(r.Comments) != 0 || r.Body != "" || r.Counts.Total() != 0 {
		t.Fatalf("empty build = %+v", r)
	}
	if r.Summary() != "no comments" {
		t.Errorf("Summary = %q", r.Summary())
	}
}

func TestBuildWithoutGroupTitle(t *testing.T) {
	placed := []comment.Placed{{Kind: comment.KindGroup, GroupID: "2", Text: "Nice."}}
	r := Build(EventComment, "", placed, parsePatch(t), nil)
	if r.Counts.Group != 1 || !strings.Contains(r.Body, "## Group comments\n1. Nice.") {
		t.Fatalf("Body = %q", r.Body)
	}
}

func TestPayload(t *testing.T) {
	r := Review{Event: EventApprove, Body: "lgtm",
		Comments: []Inline{
			{Path: "a.go", Side: "RIGHT", Line: 4, Body: "b1"},
			{Path: "b.go", SubjectType: "file", Body: "b2"},
		}}
	b, err := r.Payload()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["event"] != "APPROVE" || got["body"] != "lgtm" {
		t.Fatalf("payload = %s", b)
	}
	cs, _ := got["comments"].([]any)
	if len(cs) != 2 {
		t.Fatalf("payload comments: %s", b)
	}
	line, _ := cs[0].(map[string]any)
	if line["path"] != "a.go" || line["side"] != "RIGHT" || line["line"] != float64(4) {
		t.Errorf("line comment = %v", line)
	}
	if _, ok := line["subject_type"]; ok {
		t.Errorf("line comment has subject_type: %v", line)
	}
	file, _ := cs[1].(map[string]any)
	if file["subject_type"] != "file" || file["path"] != "b.go" {
		t.Errorf("file comment = %v", file)
	}
	if _, ok := file["line"]; ok {
		t.Errorf("file comment has line: %v", file)
	}
	if _, ok := file["side"]; ok {
		t.Errorf("file comment has side: %v", file)
	}
}

func TestSummary(t *testing.T) {
	tests := []struct {
		c    Counts
		body string
		want string
	}{
		{Counts{Line: 1}, "", "1 line comment"},
		{Counts{Line: 2}, "", "2 line comments"},
		{Counts{Line: 1, File: 2}, "", "1 line comment, 2 file comments"},
		{Counts{Line: 1, File: 1, Group: 3}, "", "1 line comment, 1 file comment, and 3 group comment(s) in the body"},
		{Counts{}, "hi", "a review body, no comments"},
		{Counts{}, "", "no comments"},
	}
	for _, tt := range tests {
		r := Review{Counts: tt.c, Body: tt.body}
		if got := r.Summary(); got != tt.want {
			t.Errorf("Summary(%+v, %q) = %q, want %q", tt.c, tt.body, got, tt.want)
		}
	}
}

// stubGh puts a gh on PATH that records its arguments and stdin to
// $CLUMP_GH_LOG for "api" calls, and prints $CLUMP_GH_JSON.
func stubGh(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "gh.log")
	outPath := filepath.Join(dir, "api.json")
	if err := os.WriteFile(outPath, []byte(`{"html_url":"https://github.com/k22i/clump/pull/42#pullrequestreview-99","id":99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := `#!/bin/sh
[ -n "$CLUMP_GH_FAIL" ] && { echo "$CLUMP_GH_FAIL" >&2; exit 1; }
if [ "$1" = api ]; then
  cat > "$CLUMP_GH_STDIN"
  printf '%s\n' "$*" >> "$CLUMP_GH_LOG"
  cat "$CLUMP_GH_OUT"
fi
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLUMP_GH_LOG", logPath)
	t.Setenv("CLUMP_GH_OUT", outPath)
	t.Setenv("CLUMP_GH_STDIN", filepath.Join(dir, "stdin.json"))
	return logPath
}

func readLog(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestSubmitCurrentRepo(t *testing.T) {
	logPath := stubGh(t)
	r := Review{Event: EventComment, Body: "hm", Comments: []Inline{{Path: "a.go", Side: "RIGHT", Line: 2, Body: "b"}}}
	res, err := Submit(context.Background(), "", "42", r)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.URL != "https://github.com/k22i/clump/pull/42#pullrequestreview-99" || res.Number != 99 {
		t.Errorf("Result = %+v", res)
	}
	got := readLog(t, logPath)
	if len(got) != 1 || got[0] != "api --method POST --input - repos/{owner}/{repo}/pulls/42/reviews" {
		t.Fatalf("gh called with %q", got)
	}
	stdin, err := os.ReadFile(os.Getenv("CLUMP_GH_STDIN"))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdin, &payload); err != nil {
		t.Fatalf("stdin: %s", stdin)
	}
	if payload["event"] != "COMMENT" || payload["body"] != "hm" {
		t.Errorf("stdin = %s", stdin)
	}
}

func TestSubmitNamedRepo(t *testing.T) {
	logPath := stubGh(t)
	if _, err := Submit(context.Background(), "", "https://github.com/k22i/clump/pull/7", Review{Event: EventApprove}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	got := readLog(t, logPath)
	if len(got) != 1 || got[0] != "api --method POST --input - repos/k22i/clump/pulls/7/reviews" {
		t.Fatalf("gh called with %q", got)
	}
}

func TestSubmitFailure(t *testing.T) {
	stubGh(t)
	t.Setenv("CLUMP_GH_FAIL", "no such pull request")
	_, err := Submit(context.Background(), "", "42", Review{Event: EventComment})
	if err == nil || !strings.Contains(err.Error(), "submit review: api") || !strings.Contains(err.Error(), "no such pull request") {
		t.Fatalf("Submit error = %v", err)
	}
}

func TestSubmitBadSpec(t *testing.T) {
	if _, err := Submit(context.Background(), "", "abc", Review{Event: EventComment}); err == nil {
		t.Fatal("Submit accepted a bad spec")
	}
}
