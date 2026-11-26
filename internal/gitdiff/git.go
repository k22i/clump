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

type Loaded struct {
	Diff    *Diff
	Raw     string
	Context string
	Title   string
	Ref     string
}

const MaxContextBytes = 6000

const MaxRangeCommits = 60

type LogEntry struct {
	Hash    string
	Short   string
	Subject string
	RelDate string
}

var GitArgs = []string{
	"-c", "core.quotepath=off",
	"-c", "diff.noprefix=false",
	"-c", "diff.mnemonicPrefix=false",
	"-c", "diff.srcPrefix=a/",
	"-c", "diff.dstPrefix=b/",
	"-c", "color.ui=never",
}

var DiffFlags = []string{"--no-color", "--no-ext-diff", "--unified=3", "--find-renames", "--submodule"}

const truncMarker = "\n[... truncated]\n"

var errEmptyRepoTexts = []string{
	"does not have any commits",
	"unknown revision",
	"bad revision",
}

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

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func capContext(s string) string {
	if len(s) <= MaxContextBytes {
		return s
	}
	return clip(s, MaxContextBytes) + truncMarker
}

func shortSHA(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func shortRev(rev string) string {
	const shaLen = 40
	if len(rev) >= shaLen && isHex(rev[:shaLen]) {
		return shortSHA(rev[:shaLen]) + rev[shaLen:]
	}
	return rev
}

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

func RepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

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

func LoadRange(ctx context.Context, repo, base, head string) (*Loaded, error) {
	l, _, err := loadRange(ctx, repo, base, head)
	return l, err
}

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

func rangeLog(ctx context.Context, repo, from, to string) (string, error) {
	return runGit(ctx, repo, "log", "--no-merges", "-n", strconv.Itoa(MaxRangeCommits),
		"--format=- %h %s%n%w(0,2,2)%b", from+".."+to, "--")
}

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

func CurrentBranch(ctx context.Context, repo string) (string, error) {
	out, err := runGit(ctx, repo, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return "", fmt.Errorf("git symbolic-ref: %w", cerr)
		}
		return "HEAD", nil
	}
	return strings.TrimSpace(out), nil
}

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

func RecentCommits(ctx context.Context, repo string, n int) ([]LogEntry, error) {
	if n <= 0 {
		return nil, nil
	}
	out, err := runGit(ctx, repo, "log", "-n", strconv.Itoa(n), "--format=%H%x00%h%x00%s%x00%cr", "HEAD", "--")
	if err != nil {
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
