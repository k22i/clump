package gitdiff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Loaded is a diff loaded from git or stdin, plus what the UI and the prompt
// need to describe it.
type Loaded struct {
	Diff *Diff
	// Raw is the patch text exactly as git printed it.
	Raw string
	// Context is the commit message(s) for the prompt, already capped
	// (MaxContextBytes). "" for a patch on stdin without a commit header.
	Context string
	// Title is a one-line description for the header: the commit subject, or
	// "<base>...<head> (N commits)".
	Title string
	// Ref is a short identity: a 12-char short hash, "<base>...<head>",
	// "<from>..<to>", or "stdin".
	Ref string
}

// MaxContextBytes caps Loaded.Context. Longer context is cut and ends with
// "\n[... truncated]\n".
const MaxContextBytes = 6000

// MaxRangeCommits caps how many commit messages LoadRange puts in Context.
const MaxRangeCommits = 60

// LogEntry is one commit in RecentCommits.
type LogEntry struct {
	Hash    string // full sha
	Short   string // 7+ char abbreviated sha (git's %h)
	Subject string
	RelDate string // git's %cr, e.g. "2 days ago"
}

// GitArgs are the arguments every diff-producing command gets, before the
// subcommand's own arguments. They pin the output format against the user's
// git config so the parser always sees a/ b/ prefixes and unquoted paths,
// and so the output matches what lazygit asks for.
var GitArgs = []string{
	"-c", "core.quotepath=off",
	"-c", "diff.noprefix=false",
	"-c", "diff.mnemonicPrefix=false",
	"-c", "diff.srcPrefix=a/",
	"-c", "diff.dstPrefix=b/",
	"-c", "color.ui=never",
}

// DiffFlags are added to every git show / git diff the loaders run.
var DiffFlags = []string{"--no-color", "--no-ext-diff", "--unified=3", "--find-renames", "--submodule"}

// truncMarker ends a Context cut by capContext.
const truncMarker = "\n[... truncated]\n"

// errEmptyRepoTexts are the messages git prints when a repository has no
// commits yet. The wording depends on the command form: "bad revision" with a
// trailing "--", "unknown revision" without it, and "does not have any
// commits" when only the current branch is implied.
var errEmptyRepoTexts = []string{
	"does not have any commits",
	"unknown revision",
	"bad revision",
}

// runGit runs one git command in repo and returns its stdout. Every call goes
// through here so the config overrides (GitArgs), the environment and the
// error text stay identical everywhere.
func runGit(ctx context.Context, repo string, args ...string) (string, error) {
	name := "git"
	if len(args) > 0 {
		name = args[0]
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("git %s: %w", name, err)
	}
	full := make([]string, 0, len(args)+len(GitArgs)+2)
	full = append(full, "-C", repo)
	full = append(full, GitArgs...)
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, "git", full...)
	// GIT_OPTIONAL_LOCKS=0: never take the index lock, lazygit runs alongside.
	// GIT_TERMINAL_PROMPT=0: never wait for credentials on a terminal.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return "", fmt.Errorf("git %s: %w", name, cerr)
		}
		return "", fmt.Errorf("git %s: %s", name, clip(strings.TrimSpace(stderr.String()), 500))
	}
	return stdout.String(), nil
}

// clip cuts s to at most n bytes on a UTF-8 rune boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// capContext enforces MaxContextBytes, marking the cut with truncMarker.
func capContext(s string) string {
	if len(s) <= MaxContextBytes {
		return s
	}
	return clip(s, MaxContextBytes) + truncMarker
}

// shortSHA is the 12-char identity of a commit sha.
func shortSHA(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// shortRev abbreviates a revision for display: a sha keeps its first 12 hex
// chars plus any suffix that followed it at full length (deadbeef^(40 hex) ->
// 12 chars + "^"), everything else ("main", "HEAD", "v1.0") is unchanged.
func shortRev(rev string) string {
	const shaLen = 40
	if len(rev) >= shaLen && isHex(rev[:shaLen]) {
		return shortSHA(rev[:shaLen]) + rev[shaLen:]
	}
	return rev
}

// isHex reports whether s is non-empty lower- or upper-case hexadecimal.
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// isNoCommits reports whether err is git saying the repository has no commits.
func isNoCommits(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, s := range errEmptyRepoTexts {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// RepoRoot returns the top-level directory of the work tree containing dir
// (git rev-parse --show-toplevel).
func RepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// LoadRev loads one commit: git show --format= <DiffFlags>
// --diff-merges=first-parent <rev>, so a merge commit is diffed against its
// first parent. Context is "COMMIT MESSAGE:\n" + the full message; Title is
// the subject; Ref is the 12-char short hash.
func LoadRev(ctx context.Context, repo, rev string) (*Loaded, error) {
	args := make([]string, 0, len(DiffFlags)+5)
	args = append(args, "show", "--format=")
	args = append(args, DiffFlags...)
	args = append(args, "--diff-merges=first-parent", rev, "--")
	raw, err := runGit(ctx, repo, args...)
	if err != nil {
		return nil, err
	}
	d, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rev, err)
	}
	msg, err := runGit(ctx, repo, "log", "-1", "--format=%H%x00%s%x00%B", rev, "--")
	if err != nil {
		return nil, err
	}
	hash, subject, body := splitLogRecord(msg)
	return &Loaded{
		Diff:    d,
		Raw:     raw,
		Context: capContext("COMMIT MESSAGE:\n" + strings.TrimSpace(body) + "\n"),
		Title:   subject,
		Ref:     shortSHA(hash),
	}, nil
}

// splitLogRecord splits a "%H%x00%s%x00%B" record into sha, subject and full
// message. The message may itself contain NUL-free multi-line text.
func splitLogRecord(rec string) (hash, subject, body string) {
	fields := strings.SplitN(strings.TrimSuffix(rec, "\n"), "\x00", 3)
	hash = fields[0]
	if len(fields) > 1 {
		subject = fields[1]
	}
	if len(fields) > 2 {
		body = fields[2]
	}
	return hash, subject, body
}

// LoadRange loads a merge-base comparison like a pull request:
// git diff <DiffFlags> <base>...<head>. Context is
// "COMMITS IN THIS RANGE (newest first):\n" + up to MaxRangeCommits messages
// from git log --no-merges <base>..<head>. Title is "<base>...<head> (N commits)".
func LoadRange(ctx context.Context, repo, base, head string) (*Loaded, error) {
	l, _, err := loadRange(ctx, repo, base, head)
	return l, err
}

// loadRange is LoadRange plus the commit count it put in Title, which
// LoadBranch reuses for its own title.
func loadRange(ctx context.Context, repo, base, head string) (*Loaded, int, error) {
	args := make([]string, 0, len(DiffFlags)+4)
	args = append(args, "diff")
	args = append(args, DiffFlags...)
	args = append(args, base+"..."+head, "--")
	raw, err := runGit(ctx, repo, args...)
	if err != nil {
		return nil, 0, err
	}
	d, err := Parse(raw)
	if err != nil {
		return nil, 0, fmt.Errorf("parse %s...%s: %w", base, head, err)
	}
	countOut, err := runGit(ctx, repo, "rev-list", "--count", "--no-merges", base+".."+head)
	if err != nil {
		return nil, 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(countOut))
	if err != nil {
		return nil, 0, fmt.Errorf("git rev-list --count %s..%s: bad count %q", base, head, strings.TrimSpace(countOut))
	}
	log, err := rangeLog(ctx, repo, base, head)
	if err != nil {
		return nil, 0, err
	}
	return &Loaded{
		Diff:    d,
		Raw:     raw,
		Context: capContext("COMMITS IN THIS RANGE (newest first):\n" + log),
		Title:   fmt.Sprintf("%s...%s (%d commits)", base, head, count),
		Ref:     base + "..." + head,
	}, count, nil
}

// rangeLog is the commit list LoadRange and LoadTwoDot put in Context.
func rangeLog(ctx context.Context, repo, from, to string) (string, error) {
	return runGit(ctx, repo, "log", "--no-merges", "-n", strconv.Itoa(MaxRangeCommits),
		"--format=- %h %s%n%w(0,2,2)%b", from+".."+to, "--")
}

// LoadBranch is LoadRange(DefaultBase(repo), "HEAD"), with Title
// "<branch> vs <base> (N commits)".
func LoadBranch(ctx context.Context, repo string) (*Loaded, error) {
	base, err := DefaultBase(ctx, repo)
	if err != nil {
		return nil, err
	}
	branch, err := CurrentBranch(ctx, repo)
	if err != nil {
		return nil, err
	}
	l, count, err := loadRange(ctx, repo, base, "HEAD")
	if err != nil {
		return nil, err
	}
	l.Title = fmt.Sprintf("%s vs %s (%d commits)", branch, base, count)
	l.Ref = base + "...HEAD"
	return l, nil
}

// LoadTwoDot loads git diff <DiffFlags> <from> <to>: a plain two-tree
// comparison, as lazygit's range selection and diffing mode show it. Context
// lists the commit messages of git log --no-merges <from>..<to> (capped as in
// LoadRange); Ref is "<short from>..<short to>".
func LoadTwoDot(ctx context.Context, repo, from, to string) (*Loaded, error) {
	args := make([]string, 0, len(DiffFlags)+4)
	args = append(args, "diff")
	args = append(args, DiffFlags...)
	args = append(args, from, to, "--")
	raw, err := runGit(ctx, repo, args...)
	if err != nil {
		return nil, err
	}
	d, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s..%s: %w", from, to, err)
	}
	// A range whose from-end has no ancestry, e.g. "X^" of a root commit, has
	// no commits to list. That is normal here (lazygit's diffing mode), so an
	// empty Context is the answer, not an error.
	commitCtx := ""
	log, logErr := rangeLog(ctx, repo, from, to)
	if logErr != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, fmt.Errorf("git log: %w", cerr)
		}
	} else {
		commitCtx = capContext("COMMITS IN THIS RANGE (newest first):\n" + log)
	}
	return &Loaded{
		Diff:    d,
		Raw:     raw,
		Context: commitCtx,
		Title:   fmt.Sprintf("%s vs %s", shortRev(from), shortRev(to)),
		Ref:     shortRev(from) + ".." + shortRev(to),
	}, nil
}

// LoadPatch parses a patch read from stdin. If it has a commit header, Title
// and Context come from it; otherwise Title is "patch from stdin".
func LoadPatch(text string) (*Loaded, error) {
	d, err := Parse(text)
	if err != nil {
		return nil, err
	}
	l := &Loaded{Diff: d, Raw: text, Title: "patch from stdin", Ref: "stdin"}
	if len(d.Commits) == 0 {
		return l, nil
	}
	c := d.Commits[0]
	l.Title = c.Subject
	l.Ref = shortSHA(c.Hash)
	msg := "COMMIT MESSAGE:\n" + c.Subject
	if c.Body != "" {
		msg += "\n\n" + strings.TrimSuffix(c.Body, "\n")
	}
	l.Context = capContext(msg + "\n")
	return l, nil
}

// DefaultBase returns the branch to compare the current branch against:
// origin/HEAD's target (e.g. "origin/main"), else "main", else "master".
// It errors if none exists.
func DefaultBase(ctx context.Context, repo string) (string, error) {
	if out, err := runGit(ctx, repo, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		if ref := strings.TrimSpace(out); strings.HasPrefix(ref, "refs/remotes/") {
			return strings.TrimPrefix(ref, "refs/remotes/"), nil
		}
	}
	for _, branch := range []string{"main", "master"} {
		if _, err := runGit(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
			return branch, nil
		}
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("git rev-parse: %w", err)
		}
	}
	return "", errors.New("no default base branch: origin/HEAD, main and master do not exist")
}

// CurrentBranch returns the checked-out branch name, or "HEAD" when detached.
func CurrentBranch(ctx context.Context, repo string) (string, error) {
	out, err := runGit(ctx, repo, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return "", fmt.Errorf("git symbolic-ref: %w", cerr)
		}
		// Detached HEAD: symbolic-ref exits non-zero and prints nothing.
		return "HEAD", nil
	}
	return strings.TrimSpace(out), nil
}

// ResolveCommit resolves any revision (a sha, a branch, refs/stash@{2}) to a
// full commit sha (git rev-parse --verify <ref>^{commit}).
func ResolveCommit(ctx context.Context, repo, ref string) (string, error) {
	out, err := runGit(ctx, repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return "", fmt.Errorf("git rev-parse: %w", cerr)
		}
		return "", fmt.Errorf("unknown revision %s", ref)
	}
	return strings.TrimSpace(out), nil
}

// RecentCommits returns the last n commits reachable from HEAD, newest first.
func RecentCommits(ctx context.Context, repo string, n int) ([]LogEntry, error) {
	if n <= 0 {
		return nil, nil
	}
	out, err := runGit(ctx, repo, "log", "-n", strconv.Itoa(n), "--format=%H%x00%h%x00%s%x00%cr", "HEAD", "--")
	if err != nil {
		// A repository without commits is empty history, not a failure.
		if isNoCommits(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []LogEntry
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\x00", 4)
		if len(f) != 4 {
			return nil, fmt.Errorf("git log: malformed record %q", line)
		}
		entries = append(entries, LogEntry{Hash: f[0], Short: f[1], Subject: f[2], RelDate: f[3]})
	}
	return entries, nil
}
