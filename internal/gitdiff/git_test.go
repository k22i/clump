package gitdiff

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// fxRepo is the scratch repository built once in TestMain by
// scripts/mkfixture.sh. Its commits have fixed authors and dates, so their
// diffs are identical to the golden files under testdata/git.
var fxRepo string

// TestMain builds the fixture repository. The loaders shell out to git, so a
// missing git is a hard failure: there is nothing meaningful to test without
// it.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(os.Stderr, "git loader tests: git is not on PATH; the loaders shell out to git")
		os.Exit(1)
	}
	root, err := os.MkdirTemp("", "clump-gitdiff-fixture-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "git loader tests: temp dir: %v\n", err)
		os.Exit(1)
	}
	fxRepo = filepath.Join(root, "fx")
	cmd := exec.Command("sh", "../../scripts/mkfixture.sh", fxRepo, "3")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "git loader tests: mkfixture.sh: %v\n%s", err, out)
		os.RemoveAll(root)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}

// gitArgs returns the argv every direct git call in these tests uses: the same
// config overrides the loaders pin, so their output is comparable.
func gitArgs(repo string, args ...string) []string {
	full := make([]string, 0, len(args)+len(GitArgs)+2)
	full = append(full, "-C", repo)
	full = append(full, GitArgs...)
	return append(full, args...)
}

// gitRaw runs git directly (not through the loader's runner) and returns its
// stdout untouched.
func gitRaw(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", gitArgs(repo, args...)...).Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// gitOut runs git directly and returns its trimmed stdout.
func gitOut(t *testing.T, repo string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(gitRaw(t, repo, args...))
}

// gitRun runs a git command whose result the test does not read.
func gitRun(t *testing.T, repo string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", gitArgs(repo, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// copyFixture copies the fixture repository, so a test can check out branches
// and add refs without disturbing the tests that expect main.
func copyFixture(t *testing.T) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "fx")
	if out, err := exec.Command("cp", "-R", fxRepo, dest).CombinedOutput(); err != nil {
		t.Fatalf("cp -R %s %s: %v\n%s", fxRepo, dest, err, out)
	}
	return dest
}

// commitSHA finds the fixture commit whose message contains subject.
func commitSHA(t *testing.T, subject string) string {
	t.Helper()
	out := gitOut(t, fxRepo, "log", "--format=%H", "-F", "--grep="+subject, "main")
	sha := strings.Split(out, "\n")[0]
	if sha == "" {
		t.Fatalf("no fixture commit matches %q", subject)
	}
	return sha
}

// fixtureIndex reads testdata/git/INDEX.tsv as name -> subject.
func fixtureIndex(t *testing.T) map[string]string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(readFixture(t, "git/INDEX.tsv")), "\n")
	m := make(map[string]string, len(lines))
	for _, line := range lines[1:] { // skip the header
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			t.Fatalf("bad INDEX.tsv line %q", line)
		}
		m[f[0]] = f[2]
	}
	return m
}

func TestRepoRoot(t *testing.T) {
	ctx := context.Background()
	got, err := RepoRoot(ctx, filepath.Join(fxRepo, "src"))
	if err != nil {
		t.Fatalf("RepoRoot(fixture/src): %v", err)
	}
	gotPath, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", got, err)
	}
	wantPath, err := filepath.EvalSymlinks(fxRepo)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", fxRepo, err)
	}
	if gotPath != wantPath {
		t.Errorf("RepoRoot(fixture/src) = %q, want %q", gotPath, wantPath)
	}

	if root, err := RepoRoot(ctx, t.TempDir()); err == nil {
		t.Errorf("RepoRoot(non-repo) = %q, want an error", root)
	}
}

func TestLoadRevMatchesGolden(t *testing.T) {
	ctx := context.Background()
	index := fixtureIndex(t)
	for _, name := range []string{"mixed", "rename", "binary-mode", "unicode-spaces", "delete", "merge"} {
		t.Run(name, func(t *testing.T) {
			sha := commitSHA(t, index[name])
			l, err := LoadRev(ctx, fxRepo, sha)
			if err != nil {
				t.Fatalf("LoadRev(%s): %v", sha, err)
			}
			golden := readFixture(t, "git/"+name+".show.txt")
			if l.Raw != golden {
				t.Errorf("Raw differs from git/%s.show.txt (%d vs %d bytes)", name, len(l.Raw), len(golden))
			}
			if got, want := l.Diff.Hash(), parseFixture(t, "git/"+name+".show.txt").Hash(); got != want {
				t.Errorf("Hash() = %s, want %s", got, want)
			}
		})
	}
}

func TestLoadRevFields(t *testing.T) {
	ctx := context.Background()
	index := fixtureIndex(t)

	mixed := commitSHA(t, index["mixed"])
	l, err := LoadRev(ctx, fxRepo, mixed)
	if err != nil {
		t.Fatalf("LoadRev(%s): %v", mixed, err)
	}
	if want := index["mixed"]; l.Title != want {
		t.Errorf("Title = %q, want %q", l.Title, want)
	}
	if len(l.Ref) != 12 {
		t.Errorf("len(Ref) = %d (%q), want 12", len(l.Ref), l.Ref)
	}
	if !strings.HasPrefix(mixed, l.Ref) {
		t.Errorf("Ref %q is not a prefix of %q", l.Ref, mixed)
	}
	if want := "COMMIT MESSAGE:\n" + index["mixed"] + "\n"; l.Context != want {
		t.Errorf("Context = %q, want %q", l.Context, want)
	}

	initial := commitSHA(t, "Initial commit")
	l, err = LoadRev(ctx, fxRepo, initial)
	if err != nil {
		t.Fatalf("LoadRev(%s): %v", initial, err)
	}
	if want := "COMMIT MESSAGE:\nInitial commit\n\nSeed the fixture repo.\n"; l.Context != want {
		t.Errorf("Context = %q, want %q", l.Context, want)
	}
	if l.Title != "Initial commit" {
		t.Errorf("Title = %q, want %q", l.Title, "Initial commit")
	}
}

func TestLoadRevMerge(t *testing.T) {
	ctx := context.Background()
	sha := commitSHA(t, "Merge branch 'feature'")
	l, err := LoadRev(ctx, fxRepo, sha)
	if err != nil {
		t.Fatalf("LoadRev(%s): %v", sha, err)
	}
	if n := len(l.Diff.Files); n != 1 {
		t.Fatalf("merge commit has %d files, want 1 (first-parent diff)", n)
	}
	f := l.Diff.Files[0]
	if f.Path() != "src/feature.go" {
		t.Errorf("file path = %q, want %q", f.Path(), "src/feature.go")
	}
	if f.Status != Added {
		t.Errorf("file status = %q, want %q", f.Status, Added)
	}
}

func TestLoadRevBad(t *testing.T) {
	l, err := LoadRev(context.Background(), fxRepo, "nope")
	if err == nil {
		t.Fatalf("LoadRev(nope) = %+v, want an error", l)
	}
	if !strings.Contains(err.Error(), "git show") {
		t.Errorf("error = %q, want it to mention %q", err, "git show")
	}
}

func TestLoadRange(t *testing.T) {
	ctx := context.Background()
	l, err := LoadRange(ctx, fxRepo, "main", "feature2")
	if err != nil {
		t.Fatalf("LoadRange(main, feature2): %v", err)
	}
	if got, want := l.Diff.Hash(), parseFixture(t, "git/range-main-feature2.show.txt").Hash(); got != want {
		t.Errorf("Hash() = %s, want %s", got, want)
	}
	if want := "main...feature2 (1 commits)"; l.Title != want {
		t.Errorf("Title = %q, want %q", l.Title, want)
	}
	if want := "main...feature2"; l.Ref != want {
		t.Errorf("Ref = %q, want %q", l.Ref, want)
	}
	if !strings.HasPrefix(l.Context, "COMMITS IN THIS RANGE (newest first):\n- ") {
		t.Errorf("Context = %q, want the commits header", l.Context)
	}
	if !strings.Contains(l.Context, "Start feature two") {
		t.Errorf("Context = %q, want it to mention %q", l.Context, "Start feature two")
	}
}

func TestLoadBranch(t *testing.T) {
	ctx := context.Background()
	cp := copyFixture(t)
	gitRun(t, cp, "checkout", "-q", "feature2")
	l, err := LoadBranch(ctx, cp)
	if err != nil {
		t.Fatalf("LoadBranch: %v", err)
	}
	if want := "feature2 vs main (1 commits)"; l.Title != want {
		t.Errorf("Title = %q, want %q", l.Title, want)
	}
	if want := "main...HEAD"; l.Ref != want {
		t.Errorf("Ref = %q, want %q", l.Ref, want)
	}
	if got, want := l.Diff.Hash(), parseFixture(t, "git/range-main-feature2.show.txt").Hash(); got != want {
		t.Errorf("Hash() = %s, want %s", got, want)
	}
}

func TestDefaultBase(t *testing.T) {
	ctx := context.Background()
	t.Run("main", func(t *testing.T) {
		cp := copyFixture(t)
		base, err := DefaultBase(ctx, cp)
		if err != nil {
			t.Fatalf("DefaultBase: %v", err)
		}
		if base != "main" {
			t.Errorf("DefaultBase = %q, want %q", base, "main")
		}
	})
	t.Run("origin", func(t *testing.T) {
		cp := copyFixture(t)
		gitRun(t, cp, "update-ref", "refs/remotes/origin/main", "HEAD")
		gitRun(t, cp, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
		base, err := DefaultBase(ctx, cp)
		if err != nil {
			t.Fatalf("DefaultBase: %v", err)
		}
		if base != "origin/main" {
			t.Errorf("DefaultBase = %q, want %q", base, "origin/main")
		}
	})
	t.Run("master", func(t *testing.T) {
		cp := copyFixture(t)
		gitRun(t, cp, "branch", "-m", "main", "master")
		base, err := DefaultBase(ctx, cp)
		if err != nil {
			t.Fatalf("DefaultBase: %v", err)
		}
		if base != "master" {
			t.Errorf("DefaultBase = %q, want %q", base, "master")
		}
	})
	t.Run("none", func(t *testing.T) {
		cp := copyFixture(t)
		gitRun(t, cp, "branch", "-m", "main", "trunk")
		base, err := DefaultBase(ctx, cp)
		if err == nil {
			t.Fatalf("DefaultBase = %q, want an error", base)
		}
		if want := "no default base branch: origin/HEAD, main and master do not exist"; err.Error() != want {
			t.Errorf("error = %q, want %q", err, want)
		}
	})
}

func TestCurrentBranch(t *testing.T) {
	ctx := context.Background()
	br, err := CurrentBranch(ctx, fxRepo)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if br != "main" {
		t.Errorf("CurrentBranch = %q, want %q", br, "main")
	}

	cp := copyFixture(t)
	gitRun(t, cp, "checkout", "-q", "--detach")
	br, err = CurrentBranch(ctx, cp)
	if err != nil {
		t.Fatalf("CurrentBranch(detached): %v", err)
	}
	if br != "HEAD" {
		t.Errorf("CurrentBranch(detached) = %q, want %q", br, "HEAD")
	}
}

func TestResolveCommit(t *testing.T) {
	ctx := context.Background()
	got, err := ResolveCommit(ctx, fxRepo, "refs/stash@{0}")
	if err != nil {
		t.Fatalf("ResolveCommit(refs/stash@{0}): %v", err)
	}
	if want := gitOut(t, fxRepo, "rev-parse", "refs/stash@{0}"); got != want {
		t.Errorf("ResolveCommit(refs/stash@{0}) = %q, want %q", got, want)
	}
	if len(got) != 40 {
		t.Errorf("len(ResolveCommit(refs/stash@{0})) = %d (%q), want 40", len(got), got)
	}

	head, err := ResolveCommit(ctx, fxRepo, "HEAD~1")
	if err != nil {
		t.Fatalf("ResolveCommit(HEAD~1): %v", err)
	}
	if len(head) != 40 || !isHex(head) {
		t.Errorf("ResolveCommit(HEAD~1) = %q, want 40 hex chars", head)
	}

	if _, err := ResolveCommit(ctx, fxRepo, "nope"); err == nil {
		t.Error("ResolveCommit(nope) succeeded, want an error")
	} else if want := "unknown revision nope"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestLoadTwoDot(t *testing.T) {
	ctx := context.Background()
	from := commitSHA(t, "Rename old.txt to new.txt and append")
	to := commitSHA(t, "Delete nonl.txt")
	l, err := LoadTwoDot(ctx, fxRepo, from, to)
	if err != nil {
		t.Fatalf("LoadTwoDot: %v", err)
	}

	args := append([]string{"diff"}, DiffFlags...)
	args = append(args, from, to, "--")
	raw := gitRaw(t, fxRepo, args...)
	wantDiff, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(git diff %s %s): %v", from, to, err)
	}
	if got, want := l.Diff.Hash(), wantDiff.Hash(); got != want {
		t.Errorf("Hash() = %s, want %s", got, want)
	}
	if got, want := l.Ref, from[:12]+".."+to[:12]; got != want {
		t.Errorf("Ref = %q, want %q", got, want)
	}
	if got, want := l.Title, from[:12]+" vs "+to[:12]; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}

	var got []string
	for _, line := range strings.Split(l.Context, "\n") {
		if s, ok := strings.CutPrefix(line, "- "); ok {
			// Drop the abbreviated sha git puts before the subject.
			if _, rest, found := strings.Cut(s, " "); found {
				got = append(got, rest)
			}
		}
	}
	want := []string{
		"Delete nonl.txt",
		"Copy main.go",
		"Unicode and spaced paths",
		"Binary, mode change, no-newline file",
	}
	if len(got) != len(want) {
		t.Fatalf("Context lists %d commits (%q), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("commit %d in Context = %q, want %q", i, got[i], want[i])
		}
	}

	t.Run("shortRev", func(t *testing.T) {
		sha := strings.Repeat("a", 40)
		cases := []struct{ in, want string }{
			{sha, sha[:12]},
			{sha + "^", sha[:12] + "^"},
			{sha + "~3", sha[:12] + "~3"},
			{sha[:12], sha[:12]},
			{"main", "main"},
			{"HEAD", "HEAD"},
			{"v1.0.0", "v1.0.0"},
			{"main...feature2", "main...feature2"},
		}
		for _, c := range cases {
			t.Run(c.in, func(t *testing.T) {
				if got := shortRev(c.in); got != c.want {
					t.Errorf("shortRev(%q) = %q, want %q", c.in, got, c.want)
				}
			})
		}
	})
}

func TestLoadPatch(t *testing.T) {
	t.Run("commit header", func(t *testing.T) {
		patch := readFixture(t, "git/mixed.lazygit.txt")
		l, err := LoadPatch(patch)
		if err != nil {
			t.Fatalf("LoadPatch: %v", err)
		}
		if want := "Mixed: greet the world, add helper, readme tweak"; l.Title != want {
			t.Errorf("Title = %q, want %q", l.Title, want)
		}
		if want := "0cf1d65695bf"; l.Ref != want {
			t.Errorf("Ref = %q, want %q", l.Ref, want)
		}
		if want := "COMMIT MESSAGE:\nMixed: greet the world, add helper, readme tweak\n"; l.Context != want {
			t.Errorf("Context = %q, want %q", l.Context, want)
		}
		if l.Raw != patch {
			t.Errorf("Raw is not the given text")
		}
	})
	t.Run("no header", func(t *testing.T) {
		patch := readFixture(t, "edge/two-files.diff")
		l, err := LoadPatch(patch)
		if err != nil {
			t.Fatalf("LoadPatch: %v", err)
		}
		if want := "patch from stdin"; l.Title != want {
			t.Errorf("Title = %q, want %q", l.Title, want)
		}
		if want := "stdin"; l.Ref != want {
			t.Errorf("Ref = %q, want %q", l.Ref, want)
		}
		if l.Context != "" {
			t.Errorf("Context = %q, want %q", l.Context, "")
		}
	})
	t.Run("bad patch", func(t *testing.T) {
		if l, err := LoadPatch(readFixture(t, "edge/bad-body.diff")); err == nil {
			t.Fatalf("LoadPatch(bad-body.diff) = %+v, want an error", l)
		}
	})
}

func TestContextCap(t *testing.T) {
	long := strings.Repeat("é", 3500) // 7000 bytes, 2 bytes per rune
	got := capContext(long)
	if len(got) > MaxContextBytes+len(truncMarker) {
		t.Errorf("len(capContext(7000 bytes)) = %d, want at most %d", len(got), MaxContextBytes+len(truncMarker))
	}
	if !utf8.ValidString(got) {
		t.Error("capContext returned invalid UTF-8")
	}
	if !strings.HasSuffix(got, truncMarker) {
		t.Errorf("capContext(...) does not end with %q", truncMarker)
	}
	if body := strings.TrimSuffix(got, truncMarker); !strings.HasPrefix(long, body) {
		t.Errorf("capContext kept %d bytes that are not a prefix of the input", len(body))
	}

	short := "COMMIT MESSAGE:\nInitial commit\n\nSeed the fixture repo.\n"
	if got := capContext(short); got != short {
		t.Errorf("capContext(short) = %q, want it unchanged", got)
	}
}

func TestRecentCommits(t *testing.T) {
	ctx := context.Background()
	entries, err := RecentCommits(ctx, fxRepo, 5)
	if err != nil {
		t.Fatalf("RecentCommits(5): %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("RecentCommits(5) returned %d entries, want 5", len(entries))
	}
	wantSubjects := strings.Split(gitOut(t, fxRepo, "log", "-5", "--format=%s", "HEAD", "--"), "\n")
	for i, e := range entries {
		if e.Subject != wantSubjects[i] {
			t.Errorf("entry %d Subject = %q, want %q", i, e.Subject, wantSubjects[i])
		}
		if !strings.HasPrefix(e.Hash, e.Short) {
			t.Errorf("entry %d: Short %q is not a prefix of Hash %q", i, e.Short, e.Hash)
		}
		if e.RelDate == "" {
			t.Errorf("entry %d has an empty RelDate", i)
		}
	}
	if want := gitOut(t, fxRepo, "log", "-1", "--format=%s"); entries[0].Subject != want {
		t.Errorf("[0].Subject = %q, want %q", entries[0].Subject, want)
	}

	if got, err := RecentCommits(ctx, fxRepo, 0); got != nil || err != nil {
		t.Errorf("RecentCommits(0) = %v, %v; want nil, nil", got, err)
	}

	empty := t.TempDir()
	gitRun(t, empty, "init", "-q")
	if got, err := RecentCommits(ctx, empty, 5); got != nil || err != nil {
		t.Errorf("RecentCommits(empty repo) = %v, %v; want nil, nil", got, err)
	}
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l, err := LoadRev(ctx, fxRepo, "HEAD")
	if err == nil {
		t.Fatalf("LoadRev(cancelled ctx) = %+v, want an error", l)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to wrap context.Canceled", err)
	}
}

func TestGitEnv(t *testing.T) {
	dir := t.TempDir()
	callPath := filepath.Join(dir, "call.txt")
	script := "#!/bin/sh\n" +
		"{\n" +
		"  echo \"GIT_OPTIONAL_LOCKS=$GIT_OPTIONAL_LOCKS\"\n" +
		"  echo \"GIT_TERMINAL_PROMPT=$GIT_TERMINAL_PROMPT\"\n" +
		"  for a in \"$@\"; do echo \"ARG $a\"; done\n" +
		"} > '" + callPath + "'\n" +
		"exit 0\n"
	gitPath := filepath.Join(dir, "git")
	if err := os.WriteFile(gitPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", dir)

	repo := t.TempDir()
	if br, err := CurrentBranch(context.Background(), repo); err != nil {
		t.Fatalf("CurrentBranch with fake git: %v (got %q)", err, br)
	}

	b, err := os.ReadFile(callPath)
	if err != nil {
		t.Fatalf("fake git did not run: %v", err)
	}
	var args, env []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		switch {
		case strings.HasPrefix(line, "ARG "):
			args = append(args, strings.TrimPrefix(line, "ARG "))
		default:
			env = append(env, line)
		}
	}

	wantEnv := []string{"GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0"}
	for _, want := range wantEnv {
		found := false
		for _, got := range env {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("git env %q missing; got %q", want, env)
		}
	}

	want := append([]string{"-C", repo}, GitArgs...)
	want = append(want, "symbolic-ref")
	if len(args) < len(want) {
		t.Fatalf("fake git got %d args (%q), want at least %d", len(args), args, len(want))
	}
	for i, w := range want {
		if args[i] != w {
			t.Fatalf("arg %d = %q, want %q (args %q)", i, args[i], w, args)
		}
	}
}
