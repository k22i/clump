package gitdiff

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// mustParse parses text, failing the test on error.
func mustParse(t *testing.T, text string) *Diff {
	t.Helper()
	d, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return d
}

// sha256HexOf is the lower-case hex SHA-256 of s, computed independently of
// the implementation under test.
func sha256HexOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestCanonicalText(t *testing.T) {
	cases := []struct{ file, want string }{
		{"edge/rename-with-edit.diff",
			"diff --git a/src/x.go b/lib/x.go\nstatus R 80\nmode  \n@@ -1 +1 @@\n-a\n+b\n"},
		{"edge/git-binary-patch.diff",
			"diff --git a/b.png b/b.png\nstatus M 0\nmode  \nbinary 1..2 100644\n"},
		{"edge/quoted-unicode.diff",
			"diff --git a/café.txt b/café.txt\nstatus A 0\nmode  100644\n@@ -0,0 +1 @@\n+hi\n"},
		{"edge/deleted.diff",
			"diff --git a/gone.go b/gone.go\nstatus D 0\nmode 100644 \n@@ -1,2 +0,0 @@\n-package x\n-var y = 1\n"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			d := parseFixture(t, c.file)
			if got := d.Canonical(); got != c.want {
				t.Errorf("Canonical() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestHashSameChange(t *testing.T) {
	pairs := [][2]string{
		{"git/mixed.show.txt", "git/mixed.lazygit.txt"},
		{"git/rename.show.txt", "git/rename.lazygit.txt"},
		{"git/binary-mode.show.txt", "git/binary-mode.lazygit.txt"},
		{"git/unicode-spaces.show.txt", "git/unicode-spaces.lazygit.txt"},
		{"git/delete.show.txt", "git/delete.lazygit.txt"},
		{"lazygit/commit-6bdf738.txt", "lazygit/commit-6bdf738-colorArg-always.txt"},
		{"lazygit/commit-6bdf738.txt", "lazygit/commit-file-in-commit.txt"},
		{"git/merge.lazygit.txt", "git/merge-cc.lazygit.txt"},
	}
	for _, p := range pairs {
		t.Run(p[0]+" == "+p[1], func(t *testing.T) {
			a, b := parseFixture(t, p[0]), parseFixture(t, p[1])
			if a.Hash() != b.Hash() {
				t.Fatalf("%s -> %s\n%s -> %s", p[0], a.Hash(), p[1], b.Hash())
			}
		})
	}
}

func TestHashDifferentChange(t *testing.T) {
	pairs := [][2]string{
		{"git/mixed.show.txt", "git/rename.show.txt"},
		{"lazygit/range-selection.txt", "lazygit/commit-6bdf738.txt"},
	}
	for _, p := range pairs {
		t.Run(p[0]+" != "+p[1], func(t *testing.T) {
			a, b := parseFixture(t, p[0]), parseFixture(t, p[1])
			if a.Hash() == b.Hash() {
				t.Fatalf("%s and %s hash equally: %s", p[0], p[1], a.Hash())
			}
		})
	}
}

func TestHashIgnoresOneBodyLine(t *testing.T) {
	const tmpl = "diff --git a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1,2 +1,2 @@\n a\n-b\n+%s\n"
	first := mustParse(t, strings.Replace(tmpl, "%s", "one", 1))
	second := mustParse(t, strings.Replace(tmpl, "%s", "two", 1))
	if first.Hash() == second.Hash() {
		t.Fatalf("one changed body line did not change the hash: %s", first.Hash())
	}
	// The same input twice must of course agree.
	again := mustParse(t, strings.Replace(tmpl, "%s", "one", 1))
	if first.Hash() != again.Hash() {
		t.Fatalf("same input, different hash: %s vs %s", first.Hash(), again.Hash())
	}
}

func TestHashFormat(t *testing.T) {
	d := parseFixture(t, "git/mixed.show.txt")
	h := d.Hash()
	if len(h) != 64 {
		t.Fatalf("Hash() = %q, want 64 hex characters", h)
	}
	if lower := strings.ToLower(h); lower != h {
		t.Fatalf("Hash() = %q, want lower case", h)
	}
	sum := sha256.Sum256([]byte(d.Canonical()))
	if want := hex.EncodeToString(sum[:]); h != want {
		t.Fatalf("Hash() = %q, want hex(sha256(Canonical())) = %q", h, want)
	}
	if want := sha256HexOf(d.Canonical()); h != want {
		t.Fatalf("Hash() = %q, want %q", h, want)
	}
}

func TestFileHash(t *testing.T) {
	d := parseFixture(t, "edge/two-files.diff")
	if len(d.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(d.Files))
	}
	// The second file's section, written out by hand.
	const second = "diff --git a/b.txt b/b.txt\nstatus M 0\nmode  \n@@ -1 +1 @@\n-x\n+y\n"
	const first = "diff --git a/a.txt b/a.txt\nstatus M 0\nmode  \n@@ -1,2 +1,2 @@\n keep\n-old\n+new\n"
	if got, want := d.Canonical(), first+second; got != want {
		t.Fatalf("Canonical() = %q, want %q", got, want)
	}
	if got, want := d.FileHash(1), sha256HexOf(second); got != want {
		t.Fatalf("FileHash(1) = %q, want sha256(<file 2's section>) = %q", got, want)
	}
	if got, want := d.FileHash(0), sha256HexOf(first); got != want {
		t.Fatalf("FileHash(0) = %q, want sha256(<file 1's section>) = %q", got, want)
	}
	if got := d.FileHash(5); got != "" {
		t.Fatalf("FileHash(5) = %q, want \"\"", got)
	}
	if got := d.FileHash(-1); got != "" {
		t.Fatalf("FileHash(-1) = %q, want \"\"", got)
	}
}

func TestCanonicalEmpty(t *testing.T) {
	cases := map[string]*Diff{
		"zero value":     {},
		"no diff at all": mustParse(t, "lazygit/branch-log-main.txt"),
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			if got := d.Canonical(); got != "" {
				t.Fatalf("Canonical() = %q, want \"\"", got)
			}
			if got := d.FileHash(0); got != "" {
				t.Fatalf("FileHash(0) = %q, want \"\"", got)
			}
		})
	}
}
