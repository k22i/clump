// Package viewtest has the golden-file and fixture helpers shared by the
// TUI render packages' tests. Import it from _test.go files only.
package viewtest

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/tui/view"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden files")

// Golden compares got with testdata/<name>.golden in the calling package's
// directory; with -update it rewrites the file instead.
func Golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run go test -run %s -update): %v", path, t.Name(), err)
	}
	if got != string(want) {
		t.Fatalf("%s differs from golden (run with -update to accept):\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// FixtureDiff parses a file from internal/gitdiff/testdata, e.g.
// FixtureDiff(t, "lazygit/commit-67a7d22.txt").
func FixtureDiff(t *testing.T, rel string) *gitdiff.Diff {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "gitdiff", "testdata")
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	d, err := gitdiff.Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// JoinPlain joins the Plain text of lines, each followed by "\n".
func JoinPlain(lines []view.Line) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Plain)
		b.WriteByte('\n')
	}
	return b.String()
}
