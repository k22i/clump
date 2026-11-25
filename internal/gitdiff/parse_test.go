package gitdiff

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFixture reads one file under testdata.
func readFixture(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return string(b)
}

// parseFixture parses one fixture and fails the test on error.
func parseFixture(t *testing.T, rel string) *Diff {
	t.Helper()
	d, err := Parse(readFixture(t, rel))
	if err != nil {
		t.Fatalf("Parse(%s): %v", rel, err)
	}
	return d
}

// hunkWant is one expected hunk: its header text and added/deleted counts.
type hunkWant struct {
	header string
	add    int
	del    int
}

// fileWant is one expected file of a fixture.
type fileWant struct {
	oldPath    string
	newPath    string
	path       string
	status     Status
	binary     bool
	oldMode    string
	newMode    string
	similarity int
	index      string // asserted only when non-empty
	hunks      []hunkWant
}

// fixtureWant is the expectation for one fixture.
type fixtureWant struct {
	commit  string // first 7 chars of Commits[0].Hash, "" when there is no commit
	subject string
	merge   bool
	files   []fileWant
	extra   func(t *testing.T, d *Diff)
}

// errorFixture lists a fixture Parse must reject.
type errorFixture struct {
	file     string
	want     string // substring of the error, or "" when only errors.Is matters
	combined bool
}

func TestParseFixtures(t *testing.T) {
	// The three files of lazygit/commit-6bdf738.txt, reused by the fixtures
	// that show the same commit.
	binaryModeCommitFiles := []fileWant{
		{oldPath: "", newPath: "logo.bin", path: "logo.bin", status: Added, binary: true, newMode: "100644",
			index: "0000000..8317895", hunks: []hunkWant{{"(binary new file)", 0, 0}}},
		{oldPath: "", newPath: "nonl.txt", path: "nonl.txt", status: Added, newMode: "100644",
			index: "0000000..20cbb4d", hunks: []hunkWant{{"@@ -0,0 +1 @@", 1, 0}}},
		{oldPath: "src/main.go", newPath: "src/main.go", path: "src/main.go", status: ModeOnly,
			oldMode: "100644", newMode: "100755", hunks: []hunkWant{{"(mode change 100644 -> 100755)", 0, 0}}},
	}
	// The three files of git/binary-mode.show.txt.
	binaryModeShowFiles := []fileWant{
		{oldPath: "", newPath: "logo.bin", path: "logo.bin", status: Added, binary: true, newMode: "100644",
			index: "0000000..029ace0", hunks: []hunkWant{{"(binary new file)", 0, 0}}},
		{oldPath: "", newPath: "nonl.txt", path: "nonl.txt", status: Added, newMode: "100644",
			index: "0000000..2802503", hunks: []hunkWant{{"@@ -0,0 +1 @@", 1, 0}}},
		{oldPath: "src/main.go", newPath: "src/main.go", path: "src/main.go", status: ModeOnly,
			oldMode: "100644", newMode: "100755", hunks: []hunkWant{{"(mode change 100644 -> 100755)", 0, 0}}},
	}
	// The two files of git/mixed.show.txt.
	mixedFiles := []fileWant{
		{oldPath: "README.md", newPath: "README.md", path: "README.md", status: Modified,
			hunks: []hunkWant{{"@@ -1,3 +1,3 @@", 1, 1}}},
		{oldPath: "src/main.go", newPath: "src/main.go", path: "src/main.go", status: Modified,
			hunks: []hunkWant{{"@@ -3,5 +3,7 @@ package main", 3, 1}}},
	}
	// The one renamed file of git/rename.show.txt.
	renameFiles := []fileWant{
		{oldPath: "docs/old.txt", newPath: "docs/new.txt", path: "docs/new.txt", status: Renamed,
			similarity: 57, hunks: []hunkWant{{"@@ -1 +1,2 @@", 1, 0}}},
	}
	// The two files of git/unicode-spaces.show.txt.
	unicodeFiles := []fileWant{
		{oldPath: "", newPath: "café.txt", path: "café.txt", status: Added, newMode: "100644",
			hunks: []hunkWant{{"@@ -0,0 +1 @@", 1, 0}}},
		{oldPath: "", newPath: "dir a/f b.txt", path: "dir a/f b.txt", status: Added, newMode: "100644",
			hunks: []hunkWant{{"@@ -0,0 +1 @@", 1, 0}}},
	}
	// The one deleted file of git/delete.show.txt.
	deleteFiles := []fileWant{
		{oldPath: "nonl.txt", newPath: "", path: "nonl.txt", status: Deleted, oldMode: "100644",
			hunks: []hunkWant{{"@@ -1 +0,0 @@", 0, 1}}},
	}

	cases := map[string]fixtureWant{
		"edge/body-header-lookalike.diff": {
			files: []fileWant{{oldPath: "x.md", newPath: "x.md", path: "x.md", status: Modified,
				hunks: []hunkWant{{"@@ -1,3 +1,3 @@", 1, 1}}}},
		},
		"edge/deleted.diff": {
			files: []fileWant{{oldPath: "gone.go", newPath: "", path: "gone.go", status: Deleted,
				oldMode: "100644", hunks: []hunkWant{{"@@ -1,2 +0,0 @@", 0, 2}}}},
		},
		"edge/empty-context-line.diff": {
			files: []fileWant{{oldPath: "blank.txt", newPath: "blank.txt", path: "blank.txt", status: Modified,
				hunks: []hunkWant{{"@@ -1,3 +1,3 @@", 1, 1}}}},
			extra: func(t *testing.T, d *Diff) {
				lines := d.Hunks[0].Lines
				want := []string{" a", "", "-b", "+c"}
				if len(lines) != len(want) {
					t.Fatalf("blank context line: Lines = %q, want %q", lines, want)
				}
				for i := range want {
					if lines[i] != want[i] {
						t.Fatalf("blank context line: Lines = %q, want %q", lines, want)
					}
				}
			},
		},
		"edge/empty-new-file.diff": {
			files: []fileWant{{oldPath: "", newPath: "empty.txt", path: "empty.txt", status: Added,
				newMode: "100644", index: "0000000..e69de29",
				hunks: []hunkWant{{"(new file)", 0, 0}}}},
		},
		"edge/git-binary-patch.diff": {
			files: []fileWant{{oldPath: "b.png", newPath: "b.png", path: "b.png", status: Modified,
				binary: true, index: "1..2 100644", hunks: []hunkWant{{"(binary change)", 0, 0}}}},
		},
		"edge/injection.diff": {
			files: []fileWant{{oldPath: "r.md", newPath: "r.md", path: "r.md", status: Modified,
				hunks: []hunkWant{{"@@ -1 +1,2 @@", 1, 0}}}},
		},
		"edge/mode-only-spaces.diff": {
			files: []fileWant{{oldPath: "dir a/f b.txt", newPath: "dir a/f b.txt", path: "dir a/f b.txt",
				status: ModeOnly, oldMode: "100644", newMode: "100755",
				hunks: []hunkWant{{"(mode change 100644 -> 100755)", 0, 0}}}},
			extra: func(t *testing.T, d *Diff) {
				if len(d.Hunks) != 1 || !d.Hunks[0].Pseudo {
					t.Fatalf("mode-only file: hunks = %+v, want one pseudo-hunk", d.Hunks)
				}
			},
		},
		"edge/nonewline-both.diff": {
			files: []fileWant{{oldPath: "n", newPath: "n", path: "n", status: Modified,
				hunks: []hunkWant{{"@@ -1 +1 @@", 1, 1}}}},
			extra: func(t *testing.T, d *Diff) {
				want := []string{"-old", `\ No newline at end of file`, "+new", `\ No newline at end of file`}
				got := d.Hunks[0].Lines
				if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
					t.Fatalf("no-newline-both: Lines = %q, want %q", got, want)
				}
			},
		},
		"edge/nonewline-new.diff": {
			files: []fileWant{{oldPath: "s.txt", newPath: "s.txt", path: "s.txt", status: Modified,
				hunks: []hunkWant{{"@@ -1 +1 @@", 1, 1}}}},
			extra: func(t *testing.T, d *Diff) {
				lines := d.Hunks[0].Lines
				if len(lines) != 3 {
					t.Fatalf("no-newline-new: Lines = %q, want 3 lines", lines)
				}
				if lines[2] != `\ No newline at end of file` {
					t.Fatalf("no-newline-new: last line = %q", lines[2])
				}
			},
		},
		"edge/omitted-counts.diff": {
			files: []fileWant{{oldPath: "a", newPath: "a", path: "a", status: Modified,
				hunks: []hunkWant{{"@@ -1 +1,2 @@", 1, 0}, {"@@ -10,0 +12 @@ func f() {", 1, 0}}}},
			extra: func(t *testing.T, d *Diff) {
				h := d.Hunks[1]
				if h.OldStart != 10 || h.OldLines != 0 || h.NewStart != 12 || h.NewLines != 1 {
					t.Fatalf("omitted counts: -%d,%d +%d,%d", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
				}
			},
		},
		"edge/quoted-unicode.diff": {
			files: []fileWant{{oldPath: "", newPath: "café.txt", path: "café.txt", status: Added,
				newMode: "100644", hunks: []hunkWant{{"@@ -0,0 +1 @@", 1, 0}}}},
		},
		"edge/rename-spaces-pure.diff": {
			files: []fileWant{{oldPath: "my file.txt", newPath: "your file.txt", path: "your file.txt",
				status: Renamed, similarity: 100, hunks: []hunkWant{{"(rename)", 0, 0}}}},
		},
		"edge/rename-with-edit.diff": {
			files: []fileWant{{oldPath: "src/x.go", newPath: "lib/x.go", path: "lib/x.go", status: Renamed,
				similarity: 80, hunks: []hunkWant{{"@@ -1 +1 @@", 1, 1}}}},
		},
		"edge/two-files.diff": {
			files: []fileWant{
				{oldPath: "a.txt", newPath: "a.txt", path: "a.txt", status: Modified,
					hunks: []hunkWant{{"@@ -1,2 +1,2 @@", 1, 1}}},
				{oldPath: "b.txt", newPath: "b.txt", path: "b.txt", status: Modified,
					hunks: []hunkWant{{"@@ -1 +1 @@", 1, 1}}},
			},
			extra: func(t *testing.T, d *Diff) {
				if d.Hunks[1].FileIdx != 1 || d.Hunks[0].FileIdx != 0 {
					t.Fatalf("two files: FileIdx = %d,%d", d.Hunks[0].FileIdx, d.Hunks[1].FileIdx)
				}
			},
		},
		"git/binary-mode.show.txt": {files: binaryModeShowFiles},
		"git/binary-mode.lazygit.txt": {commit: "44b46f1", subject: "Binary, mode change, no-newline file", files: []fileWant{
			{oldPath: "", newPath: "logo.bin", path: "logo.bin", status: Added, binary: true, newMode: "100644",
				index: "0000000..029ace0", hunks: []hunkWant{{"(binary new file)", 0, 0}}},
			{oldPath: "", newPath: "nonl.txt", path: "nonl.txt", status: Added, newMode: "100644",
				index: "0000000..2802503", hunks: []hunkWant{{"@@ -0,0 +1 @@", 1, 0}}},
			{oldPath: "src/main.go", newPath: "src/main.go", path: "src/main.go", status: ModeOnly,
				oldMode: "100644", newMode: "100755", hunks: []hunkWant{{"(mode change 100644 -> 100755)", 0, 0}}},
		}},
		"git/copy.show.txt": {
			files: []fileWant{{oldPath: "src/main.go", newPath: "src/main_copy.go", path: "src/main_copy.go",
				status: Copied, similarity: 92,
				hunks: []hunkWant{{"@@ -7,3 +7,5 @@ func main() {", 2, 0}}}},
		},
		"git/delete.show.txt":    {files: deleteFiles},
		"git/delete.lazygit.txt": {commit: "95a035d", subject: "Delete nonl.txt", files: deleteFiles},
		"git/merge.show.txt": {
			files: []fileWant{{oldPath: "", newPath: "src/feature.go", path: "src/feature.go", status: Added,
				newMode: "100644", hunks: []hunkWant{{"@@ -0,0 +1,5 @@", 5, 0}}}},
		},
		"git/merge.lazygit.txt": {
			commit: "944948e", subject: "Merge branch 'feature'", merge: true,
		},
		"git/merge-cc.lazygit.txt": {
			commit: "944948e", subject: "Merge branch 'feature'", merge: true,
		},
		"git/mixed.show.txt":    {files: mixedFiles},
		"git/mixed.lazygit.txt": {commit: "0cf1d65", subject: "Mixed: greet the world, add helper, readme tweak", files: mixedFiles},
		"git/range-main-feature2.show.txt": {
			files: []fileWant{{oldPath: "", newPath: "src/two.go", path: "src/two.go", status: Added,
				newMode: "100644", hunks: []hunkWant{{"@@ -0,0 +1,4 @@", 4, 0}}}},
		},
		"git/rename.show.txt":            {files: renameFiles},
		"git/rename.lazygit.txt":         {commit: "3fc2e4a", subject: "Rename old.txt to new.txt and append", files: renameFiles},
		"git/unicode-spaces.show.txt":    {files: unicodeFiles},
		"git/unicode-spaces.lazygit.txt": {commit: "8514dd1", subject: "Unicode and spaced paths", files: unicodeFiles},
		"lazygit/branch-log-main.txt":    {},
		"lazygit/commit-67a7d22.txt": {
			commit: "67a7d22", subject: "Mixed: fmt in main, readme tweak",
			files: []fileWant{
				{oldPath: "README.md", newPath: "README.md", path: "README.md", status: Modified,
					hunks: []hunkWant{{"@@ -1,3 +1,5 @@", 3, 1}}},
				{oldPath: "src/main.go", newPath: "src/main.go", path: "src/main.go", status: Modified,
					hunks: []hunkWant{{"@@ -1,5 +1,9 @@", 5, 1}}},
			},
		},
		"lazygit/commit-6bdf738.txt": {
			commit: "6bdf738", subject: "Binary, mode change, no-newline file",
			files: binaryModeCommitFiles,
		},
		"lazygit/commit-6bdf738-colorArg-always.txt": {
			commit: "6bdf738", subject: "Binary, mode change, no-newline file",
			files: binaryModeCommitFiles,
		},
		"lazygit/commit-bdd913d.txt": {
			commit: "bdd913d", subject: "Rename old.txt to new.txt and append",
			files: []fileWant{{oldPath: "docs/old.txt", newPath: "docs/new.txt", path: "docs/new.txt",
				status: Renamed, similarity: 75, hunks: []hunkWant{{"@@ -1,3 +1,4 @@", 1, 0}}}},
		},
		"lazygit/commit-file-in-commit.txt": {files: binaryModeCommitFiles},
		"lazygit/diffmode-main-vs-bdd913d8.txt": {
			files: []fileWant{
				{oldPath: "logo.bin", newPath: "", path: "logo.bin", status: Deleted, binary: true,
					oldMode: "100644", hunks: []hunkWant{{"(binary deleted file)", 0, 0}}},
				{oldPath: "nonl.txt", newPath: "", path: "nonl.txt", status: Deleted, oldMode: "100644",
					hunks: []hunkWant{{"@@ -1 +0,0 @@", 0, 1}}},
				{oldPath: "src/main.go", newPath: "src/main.go", path: "src/main.go", status: ModeOnly,
					oldMode: "100755", newMode: "100644",
					hunks: []hunkWant{{"(mode change 100755 -> 100644)", 0, 0}}},
			},
		},
		"lazygit/diffmode-main-vs-feature.txt": {
			files: []fileWant{
				{oldPath: "README.md", newPath: "README.md", path: "README.md", status: Modified,
					hunks: []hunkWant{{"@@ -1,5 +1,5 @@", 1, 1}}},
				{oldPath: "", newPath: "src/extra.go", path: "src/extra.go", status: Added, newMode: "100644",
					hunks: []hunkWant{{"@@ -0,0 +1,3 @@", 3, 0}}},
			},
		},
		"lazygit/range-selection.txt": {
			files: []fileWant{
				{oldPath: "docs/old.txt", newPath: "docs/new.txt", path: "docs/new.txt", status: Renamed,
					similarity: 75, hunks: []hunkWant{{"@@ -1,3 +1,4 @@", 1, 0}}},
				{oldPath: "", newPath: "logo.bin", path: "logo.bin", status: Added, binary: true,
					newMode: "100644", hunks: []hunkWant{{"(binary new file)", 0, 0}}},
				{oldPath: "", newPath: "nonl.txt", path: "nonl.txt", status: Added, newMode: "100644",
					hunks: []hunkWant{{"@@ -0,0 +1 @@", 1, 0}}},
				{oldPath: "src/main.go", newPath: "src/main.go", path: "src/main.go", status: ModeOnly,
					oldMode: "100644", newMode: "100755",
					hunks: []hunkWant{{"(mode change 100644 -> 100755)", 0, 0}}},
			},
		},
		"lazygit/stash-entry.txt": {
			files: []fileWant{{oldPath: "README.md", newPath: "README.md", path: "README.md", status: Modified,
				hunks: []hunkWant{{"@@ -3,3 +3,4 @@", 1, 0}}}},
		},
		"lazygit/worktree-unstaged-dir.txt": {
			files: []fileWant{{oldPath: "src/main.go", newPath: "src/main.go", path: "src/main.go",
				status: Modified,
				hunks:  []hunkWant{{"@@ -7,3 +7,4 @@ func main() {", 1, 0}}}},
		},
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			d := parseFixture(t, name)
			if want.commit == "" {
				if len(d.Commits) != 0 {
					t.Fatalf("commits: got %+v, want none", d.Commits)
				}
			} else {
				if len(d.Commits) != 1 {
					t.Fatalf("commits: got %+v, want exactly one", d.Commits)
				}
				c := d.Commits[0]
				if got := shortHash(c.Hash); got != want.commit {
					t.Errorf("commit hash: got %q, want %q", got, want.commit)
				}
				if c.Subject != want.subject {
					t.Errorf("commit subject: got %q, want %q", c.Subject, want.subject)
				}
			}
			if want.merge {
				if len(d.Commits) != 1 || !d.Commits[0].Merge {
					t.Errorf("commit Merge: got %+v, want true", d.Commits)
				}
			}
			if len(d.Files) != len(want.files) {
				t.Fatalf("files: got %d (%s), want %d", len(d.Files), fileNames(d.Files), len(want.files))
			}
			hunkNum := 0 // IDs are "h1".."hN" in file order across the whole diff
			for i, w := range want.files {
				f := d.Files[i]
				if f.Path() != w.path {
					t.Errorf("file %d: Path() = %q, want %q", i, f.Path(), w.path)
				}
				if f.OldPath != w.oldPath {
					t.Errorf("file %d (%s): OldPath = %q, want %q", i, w.path, f.OldPath, w.oldPath)
				}
				if f.NewPath != w.newPath {
					t.Errorf("file %d (%s): NewPath = %q, want %q", i, w.path, f.NewPath, w.newPath)
				}
				if f.Status != w.status {
					t.Errorf("file %d (%s): Status = %q, want %q", i, w.path, f.Status, w.status)
				}
				if f.Binary != w.binary {
					t.Errorf("file %d (%s): Binary = %v, want %v", i, w.path, f.Binary, w.binary)
				}
				if f.OldMode != w.oldMode || f.NewMode != w.newMode {
					t.Errorf("file %d (%s): modes %q > %q, want %q > %q", i, w.path,
						f.OldMode, f.NewMode, w.oldMode, w.newMode)
				}
				if f.Similarity != w.similarity {
					t.Errorf("file %d (%s): Similarity = %d, want %d", i, w.path, f.Similarity, w.similarity)
				}
				if w.index != "" && f.Index != w.index {
					t.Errorf("file %d (%s): Index = %q, want %q", i, w.path, f.Index, w.index)
				}
				if len(f.Hunks) != len(w.hunks) {
					t.Fatalf("file %d (%s): %d hunks, want %d", i, w.path, len(f.Hunks), len(w.hunks))
				}
				for j, hw := range w.hunks {
					h := f.Hunks[j]
					hunkNum++
					wantID := fmt.Sprintf("h%d", hunkNum)
					if h.ID != wantID {
						t.Errorf("file %d (%s) hunk %d: ID = %q, want %q", i, w.path, j, h.ID, wantID)
					}
					if h.Header != hw.header {
						t.Errorf("file %d (%s) hunk %d: Header = %q, want %q", i, w.path, j, h.Header, hw.header)
					}
					if h.Add != hw.add || h.Del != hw.del {
						t.Errorf("file %d (%s) hunk %d: +%d -%d, want +%d -%d", i, w.path, j,
							h.Add, h.Del, hw.add, hw.del)
					}
					if h.Pseudo != strings.HasPrefix(hw.header, "(") {
						t.Errorf("file %d (%s) hunk %d: Pseudo = %v for header %q", i, w.path, j,
							h.Pseudo, hw.header)
					}
				}
			}
			if want.extra != nil {
				want.extra(t, d)
			}
		})
	}
}

// shortHash is the first 7 characters of a sha.
func shortHash(s string) string {
	if len(s) < 7 {
		return s
	}
	return s[:7]
}

// fileNames lists a diff's file paths for failure messages.
func fileNames(files []*File) string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Path()
	}
	return strings.Join(names, ", ")
}

func TestParseErrors(t *testing.T) {
	cases := []errorFixture{
		{"edge/bad-body.diff", `unexpected "GARBAGE LINE" inside hunk @@ -1,2 +1,2 @@`, false},
		{"edge/bad-header.diff", `bad hunk header "@@ -x +1 @@"`, false},
		{"edge/combined.diff", "", true},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			_, err := Parse(readFixture(t, c.file))
			if err == nil {
				t.Fatalf("Parse(%s) returned no error", c.file)
			}
			if c.combined {
				if !errors.Is(err, ErrCombined) {
					t.Fatalf("Parse(%s): err = %v, want one wrapping ErrCombined", c.file, err)
				}
				return
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Parse(%s): err = %q, want it to contain %q", c.file, err, c.want)
			}
			if !strings.HasPrefix(err.Error(), "line ") {
				t.Fatalf("Parse(%s): err = %q, want a \"line N:\" prefix", c.file, err)
			}
		})
	}
}

// TestFixturesHunkCounts checks every fixture for internal consistency: the
// hunks are numbered and linked to their file, and every hunk's body matches
// the counts in its "@@" header.
func TestFixturesHunkCounts(t *testing.T) {
	// The three files Parse must reject; every other fixture is a good diff.
	bad := map[string]bool{
		"bad-body.diff":   true,
		"bad-header.diff": true,
		"combined.diff":   true,
	}
	var files []string
	for _, dir := range []string{"edge", "git", "lazygit"} {
		entries, err := os.ReadDir(filepath.Join("testdata", dir))
		if err != nil {
			t.Fatalf("read testdata/%s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || e.Name() == "INDEX.tsv" || bad[e.Name()] {
				continue
			}
			files = append(files, dir+"/"+e.Name())
		}
	}
	if len(files) != 40 {
		t.Fatalf("found %d fixtures, want 40", len(files))
	}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			d := parseFixture(t, name)
			for i, h := range d.Hunks {
				if h.Index != i {
					t.Fatalf("hunk %d (%s): Index = %d", i, h.ID, h.Index)
				}
				if want := fmt.Sprintf("h%d", i+1); h.ID != want {
					t.Fatalf("hunk %d: ID = %q, want %q", i, h.ID, want)
				}
				if h.FileIdx < 0 || h.FileIdx >= len(d.Files) || h.File != d.Files[h.FileIdx] {
					t.Fatalf("hunk %s: FileIdx = %d, File = %p", h.ID, h.FileIdx, h.File)
				}
				if !containsHunk(d.Files[h.FileIdx].Hunks, h) {
					t.Fatalf("hunk %s is not in file %d's Hunks", h.ID, h.FileIdx)
				}
				if h.Pseudo {
					if len(h.Lines) != 0 {
						t.Fatalf("pseudo-hunk %s: Lines = %q, want none", h.ID, h.Lines)
					}
					continue
				}
				var context, add, del int
				for _, l := range h.Lines {
					switch {
					case l == "" || strings.HasPrefix(l, " "):
						context++
					case strings.HasPrefix(l, "+"):
						add++
					case strings.HasPrefix(l, "-"):
						del++
					}
				}
				if context+del != h.OldLines {
					t.Fatalf("hunk %s (%s): %d old lines, header says %d", h.ID, h.Header, context+del, h.OldLines)
				}
				if context+add != h.NewLines {
					t.Fatalf("hunk %s (%s): %d new lines, header says %d", h.ID, h.Header, context+add, h.NewLines)
				}
				if h.Add != add || h.Del != del {
					t.Fatalf("hunk %s: Add/Del = %d/%d, body has %d/%d", h.ID, h.Add, h.Del, add, del)
				}
			}
			for i, f := range d.Files {
				if len(f.Hunks) == 0 {
					t.Fatalf("file %d (%s) has no hunk", i, f.Path())
				}
			}
			if len(d.Files) > 0 && len(d.Hunks) == 0 {
				t.Fatalf("%d files but no hunks", len(d.Files))
			}
		})
	}
}

// containsHunk reports whether hs contains h.
func containsHunk(hs []*Hunk, h *Hunk) bool {
	for _, x := range hs {
		if x == h {
			return true
		}
	}
	return false
}

func TestCommitHeader(t *testing.T) {
	const withBody = `commit abcdef0123456789abcdef0123456789abcdef01 (HEAD -> main, tag: v1)
Merge: 1111111 2222222
Author: A U Thor <a@example.invalid>
Date:   Wed Nov 25 15:52:41 2026 +0800

    S

    body 1

    body 2

 src/a.go | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)

diff --git a/src/a.go b/src/a.go
index 111..222 100644
--- a/src/a.go
+++ b/src/a.go
@@ -1 +1 @@
-a
+b
`
	const noBody = `commit 1111111111111111111111111111111111111111
Author: A U Thor <a@example.invalid>
Date:   Wed Nov 25 15:52:41 2026 +0800

    Only subject

diff --git a/x b/x
new file mode 100644
--- /dev/null
+++ b/x
@@ -0,0 +1 @@
+x
`
	cases := []struct {
		name    string
		in      string
		hash    string
		subject string
		body    string
		merge   bool
	}{
		{"body and merge", withBody, "abcdef0123456789abcdef0123456789abcdef01", "S", "body 1\n\nbody 2\n", true},
		{"no body no merge", noBody, "1111111111111111111111111111111111111111", "Only subject", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := Parse(c.in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(d.Commits) != 1 {
				t.Fatalf("commits = %+v, want one", d.Commits)
			}
			got := d.Commits[0]
			if got.Hash != c.hash {
				t.Errorf("Hash = %q, want %q", got.Hash, c.hash)
			}
			if got.Subject != c.subject {
				t.Errorf("Subject = %q, want %q", got.Subject, c.subject)
			}
			if got.Body != c.body {
				t.Errorf("Body = %q, want %q", got.Body, c.body)
			}
			if got.Merge != c.merge {
				t.Errorf("Merge = %v, want %v", got.Merge, c.merge)
			}
			if len(d.Files) != 1 || len(d.Hunks) != 1 {
				t.Errorf("files/hunks = %d/%d, want 1/1", len(d.Files), len(d.Hunks))
			}
		})
	}
}

func TestStripSGR(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bold diff", "\x1b[1mdiff\x1b[m", "diff"},
		{"added line", "\x1b[32m+\x1b[m\x1b[32mx\x1b[m", "+x"},
		{"plain text", "diff --git a/x b/x\n", "diff --git a/x b/x\n"},
		{"non-SGR escape", "a\x1b[2Jb", "a\x1b[2Jb"},
		{"semicolon args", "\x1b[1;36mHEAD\x1b[m", "HEAD"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripSGR(c.in); got != c.want {
				t.Errorf("StripSGR(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestPseudoHeader(t *testing.T) {
	cases := []struct {
		name string
		f    File
		want string
	}{
		{"binary added", File{Status: Added, Binary: true, NewMode: "100644"}, "(binary new file)"},
		{"binary deleted", File{Status: Deleted, Binary: true, OldMode: "100644"}, "(binary deleted file)"},
		{"binary modified", File{Status: Modified, Binary: true}, "(binary change)"},
		{"binary rename", File{Status: Renamed, Binary: true}, "(binary rename)"},
		{"mode only", File{Status: ModeOnly, OldMode: "100644", NewMode: "100755"}, "(mode change 100644 -> 100755)"},
		{"mode only same", File{Status: ModeOnly, OldMode: "100644", NewMode: "100644"}, "(mode change)"},
		{"pure rename", File{Status: Renamed}, "(rename)"},
		{"rename and mode", File{Status: Renamed, OldMode: "100644", NewMode: "100755"},
			"(rename, mode 100644 -> 100755)"},
		{"copy", File{Status: Copied}, "(copy)"},
		{"empty new file", File{Status: Added, NewMode: "100644"}, "(new file)"},
		{"empty deleted file", File{Status: Deleted, OldMode: "100644"}, "(deleted file)"},
		{"modified without hunks", File{Status: Modified}, "(no text changes)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PseudoHeader(&c.f); got != c.want {
				t.Errorf("PseudoHeader(%+v) = %q, want %q", c.f, got, c.want)
			}
		})
	}
}

// TestParseDegenerateGitLines covers "diff --git" lines too short or too odd
// for the splitting rules: they must not panic, and both paths fall back to
// the whole remainder, prefix-stripped.
func TestParseDegenerateGitLines(t *testing.T) {
	cases := []struct{ in, oldPath, newPath string }{
		{"diff --git a", "a", "a"},
		{"diff --git \"", `"`, `"`},
		{"diff --git a/x", "x", "x"},
		{"diff --git a/ b", " b", " b"},
		{"diff --git \"a/x\"", "x", "x"},
		{"diff --git a//b/", "/b/", "/b/"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			d, err := Parse(c.in + "\n")
			if err != nil {
				t.Fatalf("Parse(%q): %v", c.in, err)
			}
			if len(d.Files) != 1 {
				t.Fatalf("files = %d, want 1", len(d.Files))
			}
			f := d.Files[0]
			if f.OldPath != c.oldPath || f.NewPath != c.newPath {
				t.Fatalf("paths = %q, %q; want %q, %q", f.OldPath, f.NewPath, c.oldPath, c.newPath)
			}
			if f.Status != Modified || len(f.Hunks) != 1 || !f.Hunks[0].Pseudo {
				t.Fatalf("file = %+v, want one Modified pseudo-hunk", f)
			}
		})
	}
}

// TestParseHunkEndsEarly checks the error for input that stops mid-hunk.
func TestParseHunkEndsEarly(t *testing.T) {
	const in = "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\n"
	_, err := Parse(in)
	if err == nil {
		t.Fatalf("Parse returned no error for a truncated hunk")
	}
	if want := "hunk @@ -1,2 +1,2 @@ ends early"; !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %q, want it to contain %q", err, want)
	}
}

// TestParseNonDiff checks that input with no "diff --git" line parses to a
// diff with no files and no hunks, keeping any commit header it finds.
func TestParseNonDiff(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		commits int
	}{
		{"empty", "", 0},
		{"log graph", "* commit abc123 (HEAD -> main)\n| Author: x\n| \n|     subject\n", 0},
		{"commit only", "commit abc1234\nAuthor: x\nDate: y\n\n    subject\n", 1},
		{"bare patch", "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := Parse(c.in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(d.Files) != 0 || len(d.Hunks) != 0 {
				t.Fatalf("files/hunks = %d/%d, want 0/0", len(d.Files), len(d.Hunks))
			}
			if len(d.Commits) != c.commits {
				t.Fatalf("commits = %+v, want %d", d.Commits, c.commits)
			}
		})
	}
}

// TestParseNeverPanics feeds Parse random bytes and mutated fixtures and only
// requires that it returns. The seed is fixed so a failure is reproducible.
func TestParseNeverPanics(t *testing.T) {
	corpus := make([]string, 0, 40)
	for _, dir := range []string{"edge", "git", "lazygit"} {
		entries, err := os.ReadDir(filepath.Join("testdata", dir))
		if err != nil {
			t.Fatalf("read testdata/%s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || e.Name() == "INDEX.tsv" {
				continue
			}
			corpus = append(corpus, readFixture(t, dir+"/"+e.Name()))
		}
	}
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 2000; i++ {
		in := randomInput(r, corpus)
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("Parse panicked on input %q: %v", in, p)
				}
			}()
			_, _ = Parse(in) // any error is acceptable here; only a panic is a bug
		}()
	}
}

// randomInput makes one random patch-shaped string: random bytes, random
// diff-ish lines, or a mutation of a fixture.
func randomInput(r *rand.Rand, corpus []string) string {
	switch r.IntN(3) {
	case 0:
		b := make([]byte, r.IntN(300))
		for i := range b {
			b[i] = byte(r.IntN(256))
		}
		return string(b)
	case 1:
		lines := []string{
			"diff --git ", "diff --git a/x b/x", "diff --git a b", "diff --git \"a/x\" \"b/x\"",
			"diff --git a/a b/a b/a", "diff --cc x", "diff --combined x", "@@ -1 +1 @@",
			"@@ -1,2 +1,2 @@ ctx", "@@ -x +1 @@", "@@@ -1 -1 +1 @@@", "--- a/x", "+++ b/x",
			"--- /dev/null", "+++ /dev/null", "--- \"b/caf\\303\\251\"\t", " context", "",
			"+add", "-del", `\ No newline at end of file`, "index 1..2 100644", "new file mode 100644",
			"deleted file mode 100644", "old mode 100644", "new mode 100755", "similarity index 80%",
			"similarity index x%", "rename from a", "rename to b", "copy from a", "copy to b",
			"Binary files a and b differ", "GIT binary patch", "literal 5", "commit abc", "Merge: 1 2",
			"    subject", "     body", " src/a.go | 2 +-", "---",
		}
		n := r.IntN(20)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(lines[r.IntN(len(lines))])
			if r.IntN(8) != 0 {
				b.WriteString("\n")
			}
		}
		return b.String()
	}
	in := corpus[r.IntN(len(corpus))]
	lines := strings.Split(in, "\n")
	for k := r.IntN(5); k > 0; k-- {
		switch {
		case len(lines) == 0:
			lines = append(lines, "")
		case r.IntN(2) == 0:
			at := r.IntN(len(lines))
			lines = append(lines[:at], lines[at+1:]...)
		default:
			at := r.IntN(len(lines) + 1)
			lines = append(lines, "")
			copy(lines[at+1:], lines[at:])
			lines[at] = "mutation " + fmt.Sprint(r.IntN(1000))
		}
	}
	return strings.Join(lines, "\n")
}
