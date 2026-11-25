package gitdiff

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePRSpec(t *testing.T) {
	tests := []struct {
		in   string
		want PRSpec
	}{
		{in: "42", want: PRSpec{Number: "42"}},
		{in: "#42", want: PRSpec{Number: "42"}},
		{in: "042", want: PRSpec{Number: "42"}},
		{in: " 42 ", want: PRSpec{Number: "42"}},
		{in: "k22i/clump#42", want: PRSpec{Owner: "k22i", Repo: "clump", Number: "42"}},
		{in: "k22i/clump#007", want: PRSpec{Owner: "k22i", Repo: "clump", Number: "7"}},
		{in: "https://github.com/k22i/clump/pull/42", want: PRSpec{Owner: "k22i", Repo: "clump", Number: "42"}},
		{in: "https://github.com/k22i/clump/pull/42/", want: PRSpec{Owner: "k22i", Repo: "clump", Number: "42"}},
		{in: "https://github.com/k22i/clump/pull/42.diff", want: PRSpec{Owner: "k22i", Repo: "clump", Number: "42"}},
		{in: "https://github.com/k22i/clump/pull/42.patch", want: PRSpec{Owner: "k22i", Repo: "clump", Number: "42"}},
		{in: "http://github.com/k22i/clump/pull/42", want: PRSpec{Owner: "k22i", Repo: "clump", Number: "42"}},
		{in: "github.com/k22i/clump/pull/42", want: PRSpec{Owner: "k22i", Repo: "clump", Number: "42"}},
		{in: "https://github.com/k22i/eco.zone-ledger/pull/3", want: PRSpec{Owner: "k22i", Repo: "eco.zone-ledger", Number: "3"}},
	}
	for _, tt := range tests {
		got, err := ParsePRSpec(tt.in)
		if err != nil {
			t.Errorf("ParsePRSpec(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParsePRSpec(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParsePRSpecErrors(t *testing.T) {
	for _, in := range []string{"", "  ", "0", "#0", "00", "abc", "42x", "#", "k22i",
		"k22i/clump", "k22i/clump#", "/clump#42", "k22i/#42", "https://gitlab.com/k22i/clump/-/merge_requests/42",
		"https://github.com/k22i/clump/pulls/42", "https://github.com/k22i/clump/pull/"} {
		if p, err := ParsePRSpec(in); err == nil {
			t.Errorf("ParsePRSpec(%q) = %+v, want an error", in, p)
		}
	}
}

// stubGh answers LoadPR's gh calls from fixture files: "pr diff" prints
// diffPath, "pr view" prints viewPath. Every invocation appends
// "<cwd> <args>" to $CLUMP_GH_LOG; $CLUMP_GH_FAIL makes it exit 1 with that
// text on stderr. dir is for PATH.
func stubGh(t *testing.T, diffPath, viewPath string) (dir, logPath string) {
	t.Helper()
	dir = t.TempDir()
	logPath = filepath.Join(dir, "gh.log")
	// gh runs in the directories LoadPR is given, so the fixtures must be
	// absolute paths.
	if p, err := filepath.Abs(diffPath); err == nil {
		diffPath = p
	}
	if p, err := filepath.Abs(viewPath); err == nil {
		viewPath = p
	}
	sh := `#!/bin/sh
printf '%s %s\n' "$(pwd)" "$*" >> "$CLUMP_GH_LOG"
[ -n "$CLUMP_GH_FAIL" ] && { echo "$CLUMP_GH_FAIL" >&2; exit 1; }
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
	t.Setenv("CLUMP_GH_DIFF", diffPath)
	t.Setenv("CLUMP_GH_VIEW", viewPath)
	return dir, logPath
}

// ghLog returns the recorded stub invocations; nil when gh never ran.
func ghLog(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestLoadPRNamedRepo(t *testing.T) {
	_, logPath := stubGh(t, "testdata/github/pr-42.patch", "testdata/github/pr-42.json")
	l, err := LoadPR(context.Background(), "", "https://github.com/k22i/clump/pull/42")
	if err != nil {
		t.Fatalf("LoadPR: %v", err)
	}
	if len(l.Diff.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(l.Diff.Files))
	}
	if h := l.Diff.Hunks; len(h) == 0 {
		t.Fatal("no hunks parsed")
	}
	if l.Title != "#42 Add the retry policy" {
		t.Errorf("Title = %q", l.Title)
	}
	if l.Ref != "k22i/clump#42" {
		t.Errorf("Ref = %q, want k22i/clump#42", l.Ref)
	}
	for _, want := range []string{
		"PULL REQUEST: Add the retry policy",
		"Fixes #7.",
		"COMMITS IN THIS PULL REQUEST (oldest first)",
		"- fail before retry (8b4e081e1c2a)",
		"- retry: attempt up to three times (d12c05fa9e61)",
	} {
		if !strings.Contains(l.Context, want) {
			t.Errorf("Context misses %q:\n%s", want, l.Context)
		}
	}
	want := []string{
		"pr diff 42 --color=never --repo k22i/clump",
		"pr view 42 --repo k22i/clump --json number,title,body,commits",
	}
	got := ghLog(t, logPath)
	if len(got) != len(want) {
		t.Fatalf("gh called %d times, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if fields := strings.Fields(got[i])[1:]; strings.Join(fields, " ") != want[i] {
			t.Errorf("gh call %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadPRCurrentRepo(t *testing.T) {
	_, logPath := stubGh(t, "testdata/github/pr-42.patch", "testdata/github/pr-42.json")
	repo := t.TempDir() // just the directory gh runs in; not a real clone
	l, err := LoadPR(context.Background(), repo, "#42")
	if err != nil {
		t.Fatalf("LoadPR: %v", err)
	}
	if l.Ref != "pr/42" {
		t.Errorf("Ref = %q, want pr/42", l.Ref)
	}
	got := ghLog(t, logPath)
	if len(got) == 0 {
		t.Fatal("gh was not called")
	}
	fields := strings.Fields(got[0])
	// sh's pwd is the physical path; t.TempDir may be behind a symlink.
	real, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cwd := fields[0]; cwd != real {
		t.Errorf("gh ran in %q, want %q", cwd, real)
	}
	if args := strings.Join(fields[1:], " "); args != "pr diff 42 --color=never" {
		t.Errorf("gh args = %q, want no --repo", args)
	}
}

func TestLoadPRNumberWithoutRepo(t *testing.T) {
	_, logPath := stubGh(t, "testdata/github/pr-42.patch", "testdata/github/pr-42.json")
	_, err := LoadPR(context.Background(), "", "42")
	if err == nil || !strings.Contains(err.Error(), "run inside a clone") {
		t.Errorf("LoadPR(\"\") err = %v, want \"run inside a clone\"", err)
	}
	if len(ghLog(t, logPath)) != 0 {
		t.Error("gh was called despite no repository to resolve against")
	}
}

func TestLoadPRBadSpec(t *testing.T) {
	if _, err := LoadPR(context.Background(), t.TempDir(), "https://gitlab.com/x/y/-/merge_requests/1"); err == nil {
		t.Error("LoadPR accepted a non-github URL")
	}
}

func TestLoadPRGhFailure(t *testing.T) {
	stubGh(t, "testdata/github/pr-42.patch", "testdata/github/pr-42.json")
	t.Setenv("CLUMP_GH_FAIL", "gh: no pull request found: 42")
	_, err := LoadPR(context.Background(), "", "k22i/clump#42")
	if err == nil || !strings.Contains(err.Error(), "pr diff 42 --color=never --repo k22i/clump:") {
		t.Errorf("err = %v, want the gh call and its stderr", err)
	}
	if err != nil && !strings.Contains(err.Error(), "no pull request found") {
		t.Errorf("err = %v, wants gh's stderr clip", err)
	}
}

func TestLoadPRGhMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := LoadPR(context.Background(), "", "k22i/clump#42")
	if err == nil || !strings.Contains(err.Error(), "gh is not on PATH") {
		t.Errorf("err = %v, want a missing-gh error", err)
	}
}

func TestLoadPRContextCap(t *testing.T) {
	// A pull request with a huge body: the context must be capped, like the
	// git loaders cap theirs.
	viewPath := filepath.Join(t.TempDir(), "view.json")
	body := strings.Repeat("x", MaxContextBytes)
	view := `{"number": 42, "title": "Big", "body": "` + body + `", "commits": []}`
	if err := os.WriteFile(viewPath, []byte(view), 0o644); err != nil {
		t.Fatal(err)
	}
	stubGh(t, "testdata/github/pr-42.patch", viewPath)
	l, err := LoadPR(context.Background(), "", "k22i/clump#42")
	if err != nil {
		t.Fatalf("LoadPR: %v", err)
	}
	if len(l.Context) > MaxContextBytes+len(truncMarker) {
		t.Errorf("Context is %d bytes, above the cap", len(l.Context))
	}
	if !strings.HasSuffix(l.Context, truncMarker) {
		t.Errorf("Context does not end with the truncation marker:\n%q", l.Context[len(l.Context)-40:])
	}
}
