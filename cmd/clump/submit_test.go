package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func submitStubGh(t *testing.T) (dir, logPath, apiStdin string) {
	t.Helper()
	dir = t.TempDir()
	fx, err := filepath.Abs("../../internal/gitdiff/testdata")
	if err != nil {
		t.Fatal(err)
	}
	logPath = filepath.Join(dir, "gh.log")
	apiStdin = filepath.Join(dir, "api-stdin.json")
	out, err := filepath.Abs(filepath.Join(dir, "api-out.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte(`{"html_url":"https://github.com/k22i/clump/pull/42#pullrequestreview-1","id":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := `#!/bin/sh
if [ "$1" = api ]; then
  cat > "$CLUMP_GH_API_STDIN"
  printf '%s\n' "$*" >> "$CLUMP_GH_LOG"
  cat "$CLUMP_GH_API_OUT"
  exit 0
fi
printf '%s\n' "$*" >> "$CLUMP_GH_LOG"
case "$2" in
diff) cat "$CLUMP_GH_DIFF" ;;
view) cat "$CLUMP_GH_VIEW" ;;
*) echo "gh: unexpected subcommand $2" >&2; exit 9 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLUMP_GH_LOG", logPath)
	t.Setenv("CLUMP_GH_DIFF", filepath.Join(fx, "github/pr-42.patch"))
	t.Setenv("CLUMP_GH_VIEW", filepath.Join(fx, "github/pr-42.json"))
	t.Setenv("CLUMP_GH_API_STDIN", apiStdin)
	t.Setenv("CLUMP_GH_API_OUT", out)
	return dir, logPath, apiStdin
}

func submitSandbox(t *testing.T) {
	t.Helper()
	t.Setenv("CLUMP_CONFIG", filepath.Join(t.TempDir(), "missing-config.toml"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
}

func submitRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runSubmit(args, os.Stdin, &out, &out)
	return out.String(), err
}

func TestParseSubmit(t *testing.T) {
	o, err := parseSubmit([]string{"--pr", "k22i/clump#42", "--event", "request-changes", "--body", "please", "--yes"})
	if err != nil {
		t.Fatalf("parseSubmit: %v", err)
	}
	if o.pr != "k22i/clump#42" || o.event != "request-changes" || o.body != "please" || !o.yes || o.dryRun {
		t.Errorf("opts = %+v", o)
	}
}

func TestParseSubmitErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: nil, want: "submit needs --pr"},
		{args: []string{"--pr", "nope"}, want: "is not a pull request"},
		{args: []string{"--pr", "42"}, want: "submit needs --event"},
		{args: []string{"--pr", "42", "--event", "draft"}, want: "unknown review event"},
		{args: []string{"--pr", "42", "--event", "comment", "--body", "x", "--body-file", "f"}, want: "exclusive"},
		{args: []string{"--pr", "42", "--event", "comment", "extra"}, want: "flags only"},
	}
	for _, tt := range tests {
		o, err := parseSubmit(tt.args)
		if err == nil {
			t.Errorf("parseSubmit(%q) = %+v, want an error", tt.args, o)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("parseSubmit(%q) err = %q, want %q", tt.args, err, tt.want)
		}
	}
}

func TestSubmitDryRun(t *testing.T) {
	submitStubGh(t)
	submitSandbox(t)
	if _, err := commentRun(t, "add", "--pr", "k22i/clump#42", "--file", "src/retry_test.go", "--line", "6", "why is the test empty?"); err != nil {
		t.Fatalf("comments add: %v", err)
	}
	got, err := submitRun(t, "--pr", "k22i/clump#42", "--event", "request-changes", "--body", "one more thing", "--dry-run")
	if err != nil {
		t.Fatalf("submit --dry-run: %v\n%s", err, got)
	}
	if !strings.Contains(got, "1 line comment") {
		t.Errorf("summary = %q", got)
	}
	i := strings.Index(got, "{")
	if i < 0 {
		t.Fatalf("no JSON in %q", got)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(got[i:]), &payload); err != nil {
		t.Fatalf("payload: %v\n%s", err, got)
	}
	if payload["event"] != "REQUEST_CHANGES" || payload["body"] != "one more thing" {
		t.Errorf("payload = %v", payload)
	}
	cs, _ := payload["comments"].([]any)
	if len(cs) != 1 {
		t.Fatalf("comments = %v", payload)
	}
	c, _ := cs[0].(map[string]any)
	if c["path"] != "src/retry_test.go" || c["side"] != "RIGHT" || c["line"] != float64(6) || c["body"] != "why is the test empty?" {
		t.Errorf("comment = %v", c)
	}
}

func TestSubmitPostsReview(t *testing.T) {
	_, logPath, apiStdin := submitStubGh(t)
	submitSandbox(t)
	if _, err := commentRun(t, "add", "--pr", "k22i/clump#42", "--file", "src/retry.go", "whole-file note"); err != nil {
		t.Fatalf("comments add: %v", err)
	}
	got, err := submitRun(t, "--pr", "k22i/clump#42", "--event", "approve", "--yes")
	if err != nil {
		t.Fatalf("submit: %v\n%s", err, got)
	}
	if !strings.Contains(got, "approve review: https://github.com/k22i/clump/pull/42#pullrequestreview-1") {
		t.Errorf("out = %q", got)
	}
	b, err := os.ReadFile(apiStdin)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("api stdin: %s", b)
	}
	if payload["event"] != "APPROVE" {
		t.Errorf("payload = %v", payload)
	}
	cs, _ := payload["comments"].([]any)
	if len(cs) != 1 {
		t.Fatalf("comments = %v", payload)
	}
	c, _ := cs[0].(map[string]any)
	if c["path"] != "src/retry.go" || c["subject_type"] != "file" {
		t.Errorf("comment = %v", c)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "api --method POST --input - repos/k22i/clump/pulls/42/reviews") {
		t.Errorf("gh log = %q", log)
	}
}

func TestSubmitNothingToSubmit(t *testing.T) {
	submitStubGh(t)
	submitSandbox(t)
	_, err := submitRun(t, "--pr", "k22i/clump#42", "--event", "comment")
	if err == nil || !strings.Contains(err.Error(), "nothing to submit") {
		t.Fatalf("err = %v, want nothing to submit", err)
	}
}
