package lazygit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Shas of the fixture commits the captured lazygit argv lines use.
const (
	sha6bdf738 = "6bdf738591d3bd3c8e57a24b7d885f492633294b"
	shaBdd913d = "bdd913d801bb5c57d40f5e14ea07d1d184b46eef"
	sha67a7d22 = "67a7d22b7de1f3d77a15f8eb743d7e2199ce055b"
)

// indexArgv reads the argv lines captured from lazygit 0.66 and keyed by the
// sample they produced.
func indexArgv(t *testing.T) []struct {
	name string
	argv []string
} {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "gitdiff", "testdata", "lazygit", "INDEX.tsv"))
	if err != nil {
		t.Fatalf("read INDEX.tsv: %v", err)
	}
	var rows []struct {
		name string
		argv []string
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if line == "" {
			continue
		}
		cols := strings.SplitN(line, "\t", 2)
		if len(cols) != 2 {
			t.Fatalf("INDEX.tsv row has no argv column: %q", line)
		}
		rows = append(rows, struct {
			name string
			argv []string
		}{cols[0], strings.Split(cols[1], " ")})
	}
	return rows
}

// TestClassifyIndex classifies every argv line captured from lazygit.
func TestClassifyIndex(t *testing.T) {
	want := map[string]Selection{
		"worktree-unstaged-dir.txt":     {Kind: KindWorktree, Paths: []string{"src"}},
		"commit-6bdf738.txt":            {Kind: KindCommit, Commit: sha6bdf738},
		"commit-bdd913d.txt":            {Kind: KindCommit, Commit: shaBdd913d},
		"range-selection.txt":           {Kind: KindRange, From: shaBdd913d + "^", To: sha6bdf738},
		"commit-file-in-commit.txt":     {Kind: KindCommit, Commit: sha6bdf738},
		"stash-entry.txt":               {Kind: KindStash, Stash: "refs/stash@{0}"},
		"branch-log-main.txt":           {Kind: KindLog},
		"diffmode-main-vs-feature.txt":  {Kind: KindDiffMode, From: "main", To: "feature"},
		"diffmode-main-vs-bdd913d8.txt": {Kind: KindDiffMode, From: "main", To: shaBdd913d},
		"commit-67a7d22.txt":            {Kind: KindCommit, Commit: sha67a7d22},
	}
	rows := indexArgv(t)
	if len(rows) < 2 {
		t.Fatalf("INDEX.tsv has %d rows; want the captured rows plus the trailer", len(rows))
	}
	// The last row is a note in the argv column, not an argv line.
	rows = rows[:len(rows)-1]
	if len(rows) != len(want) {
		t.Fatalf("INDEX.tsv has %d argv rows; want %d", len(rows), len(want))
	}
	for _, row := range rows {
		w, ok := want[row.name]
		if !ok {
			t.Fatalf("no expectation for INDEX.tsv row %q", row.name)
		}
		t.Run(row.name, func(t *testing.T) {
			got := normalize(Classify(row.argv, ""))
			if !selEqual(got, normalize(w)) {
				t.Fatalf("Classify(%q) = %+v; want %+v", row.argv, got, w)
			}
		})
	}
}

// TestClassifySynthetic covers argv shapes lazygit has not produced yet.
func TestClassifySynthetic(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want Selection
	}{
		{
			name: "cached single file",
			argv: []string{"git", "-C", "r", "diff", "--cached", "--", "a.go"},
			want: Selection{Kind: KindWorktree, Paths: []string{"a.go"}, Cached: true},
		},
		{
			name: "no-index",
			argv: []string{"git", "diff", "--no-index", "--", "/dev/null", "new.txt"},
			want: Selection{Kind: KindWorktree, Paths: []string{"/dev/null", "new.txt"}},
		},
		{
			name: "file in commit",
			argv: []string{"git", "diff", "abc^", "abc", "--", "src/x.go"},
			want: Selection{Kind: KindFileInCommit, Commit: "abc", Paths: []string{"src/x.go"}},
		},
		{
			name: "stash number three",
			argv: []string{"git", "stash", "show", "-p", "--stat", "stash@{3}"},
			want: Selection{Kind: KindStash, Stash: "stash@{3}"},
		},
		{
			name: "status is not a diff",
			argv: []string{"git", "status"},
			want: Selection{Kind: KindUnknown},
		},
		{
			name: "not git",
			argv: []string{"/usr/bin/vim", "x"},
			want: Selection{Kind: KindUnknown},
		},
		{
			name: "git alone",
			argv: []string{"git"},
			want: Selection{Kind: KindUnknown},
		},
		{
			name: "whole commit via dot pathspec",
			argv: []string{"git", "diff", "abc^", "abc", "--", "."},
			want: Selection{Kind: KindCommit, Commit: "abc"},
		},
		{
			name: "whole commit without pathspec",
			argv: []string{"git", "diff", "abc^", "abc", "--"},
			want: Selection{Kind: KindCommit, Commit: "abc"},
		},
		{
			name: "range",
			argv: []string{"git", "diff", "abc^", "def"},
			want: Selection{Kind: KindRange, From: "abc^", To: "def"},
		},
		{
			name: "diffmode",
			argv: []string{"git", "diff", "main", "feature"},
			want: Selection{Kind: KindDiffMode, From: "main", To: "feature"},
		},
		{
			name: "staged long flag",
			argv: []string{"git", "diff", "--staged"},
			want: Selection{Kind: KindWorktree, Cached: true},
		},
		{
			name: "spaced value option is not a positional",
			argv: []string{"git", "diff", "-U", "5", "--unified", "7", "--", "a.go"},
			want: Selection{Kind: KindWorktree, Paths: []string{"a.go"}},
		},
		{
			name: "git dashed binary name",
			argv: []string{"/usr/local/bin/git-foo", "diff", "--", "a"},
			want: Selection{Kind: KindWorktree, Paths: []string{"a"}},
		},
		{
			name: "show with no revision",
			argv: []string{"git", "show", "--stat"},
			want: Selection{Kind: KindUnknown},
		},
		{
			name: "one positional diff",
			argv: []string{"git", "diff", "HEAD"},
			want: Selection{Kind: KindUnknown},
		},
		{
			name: "stash list is not a stash diff",
			argv: []string{"git", "stash", "list"},
			want: Selection{Kind: KindUnknown},
		},
		{
			name: "global dash dash options",
			argv: []string{"git", "--no-pager", "--literal-pathspecs", "log", "--oneline"},
			want: Selection{Kind: KindLog},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(Classify(tt.argv, ""))
			if !selEqual(got, normalize(tt.want)) {
				t.Fatalf("Classify(%q) = %+v; want %+v", tt.argv, got, tt.want)
			}
		})
	}
}

// TestClassifyFallback covers an unreadable parent argv: only the commit
// header of the diff text identifies anything.
func TestClassifyFallback(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		diff string
		want Selection
	}{
		{
			name: "commit header with SGR",
			argv: nil,
			diff: "\x1b[33mcommit 0123456789abcdef0123456789abcdef01234567\x1b[m (HEAD)\n" +
				"Author: A <a@b>\n",
			want: Selection{Kind: KindCommit, Commit: "0123456789abcdef0123456789abcdef01234567"},
		},
		{
			name: "short sha in header",
			argv: nil,
			diff: "commit 6bdf738\n",
			want: Selection{Kind: KindCommit, Commit: "6bdf738"},
		},
		{
			name: "plain diff",
			argv: nil,
			diff: "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n",
			want: Selection{Kind: KindUnknown},
		},
		{
			name: "empty diff",
			argv: []string{},
			diff: "",
			want: Selection{Kind: KindUnknown},
		},
		{
			name: "header not on the first line",
			argv: nil,
			diff: "\ndiff --git a/a b/a\ncommit 0123456789abcdef0\n",
			want: Selection{Kind: KindUnknown},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(Classify(tt.argv, tt.diff))
			if !selEqual(got, normalize(tt.want)) {
				t.Fatalf("Classify(%q, diff) = %+v; want %+v", tt.argv, got, tt.want)
			}
		})
	}
}

// TestRefForms pins every Ref form.
func TestRefForms(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"commit", []string{"git", "show", sha6bdf738, "--"}, "commit:" + sha6bdf738},
		{"range", []string{"git", "diff", "abc^", "def"}, "range:abc^..def"},
		{"stash", []string{"git", "stash", "show", "refs/stash@{0}"}, "stash:refs/stash@{0}"},
		{"diffmode", []string{"git", "diff", "main", "feature"}, "diffmode:main..feature"},
		{"file", []string{"git", "diff", "abc^", "abc", "--", "a.go", "b.go"}, "file:abc:a.go,b.go"},
		{"worktree", []string{"git", "diff", "--", "src"}, "worktree:src"},
		{"cached", []string{"git", "diff", "--cached", "--", "a.go"}, "worktree:cached:a.go"},
		{"bare worktree", []string{"git", "diff"}, "worktree:"},
		{"log", []string{"git", "log", "--graph"}, "log"},
		{"unknown", []string{"git", "status"}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.argv, "").Ref; got != tt.want {
				t.Fatalf("Ref = %q; want %q", got, tt.want)
			}
		})
	}
	t.Run("fallback commit", func(t *testing.T) {
		got := Classify(nil, "commit "+sha6bdf738+"\n").Ref
		if want := "commit:" + sha6bdf738; got != want {
			t.Fatalf("Ref = %q; want %q", got, want)
		}
	})
	t.Run("fallback unknown", func(t *testing.T) {
		if got := Classify(nil, "diff --git a/a b/a\n").Ref; got != "unknown" {
			t.Fatalf("Ref = %q; want %q", got, "unknown")
		}
	})
}

// TestDecide pins the selection-to-action table.
func TestDecide(t *testing.T) {
	tests := []struct {
		name           string
		kind           Kind
		followWorktree bool
		want           Action
	}{
		{"commit", KindCommit, false, Follow},
		{"range", KindRange, false, Follow},
		{"stash", KindStash, false, Follow},
		{"diffmode", KindDiffMode, false, Follow},
		{"file in commit", KindFileInCommit, false, CursorOnly},
		{"file in commit follows nothing", KindFileInCommit, true, CursorOnly},
		{"worktree dim", KindWorktree, false, Dim},
		{"worktree followed", KindWorktree, true, Follow},
		{"log", KindLog, false, Dim},
		{"log with worktree on", KindLog, true, Dim},
		{"unknown", KindUnknown, false, Dim},
		{"unknown with worktree on", KindUnknown, true, Dim},
		{"zero value", Kind(""), false, Dim},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(Selection{Kind: tt.kind}, tt.followWorktree); got != tt.want {
				t.Fatalf("Decide(%q, followWorktree=%v) = %q; want %q",
					tt.kind, tt.followWorktree, got, tt.want)
			}
		})
	}
}

// normalize drops zero-valued fields so selections compare field by field.
func normalize(s Selection) Selection {
	s.Ref = ""
	if len(s.Paths) == 0 {
		s.Paths = nil
	}
	return s
}

// selEqual compares selections field by field.
func selEqual(a, b Selection) bool {
	if a.Kind != b.Kind || a.Commit != b.Commit || a.From != b.From || a.To != b.To ||
		a.Stash != b.Stash || a.Cached != b.Cached || len(a.Paths) != len(b.Paths) {
		return false
	}
	for i := range a.Paths {
		if a.Paths[i] != b.Paths[i] {
			return false
		}
	}
	return true
}
