package gitdiff

import (
	"context"
	"strings"
)

// CommitMessage returns a commit's full sha, subject and full message
// (git log -1). Watch mode uses it to title diffs pushed by lazygit
// without re-running the diff.
func CommitMessage(ctx context.Context, repo, rev string) (hash, subject, message string, err error) {
	out, err := runGit(ctx, repo, "log", "-1", "--format=%H%x00%s%x00%B", rev, "--")
	if err != nil {
		return "", "", "", err
	}
	hash, subject, message = splitLogRecord(out)
	return hash, subject, strings.TrimSpace(message), nil
}

// RangeContext returns the prompt context for the commits in from..to
// ("COMMITS IN THIS RANGE (newest first):" plus their messages, capped like
// LoadRange's), or "" when git cannot list them.
func RangeContext(ctx context.Context, repo, from, to string) string {
	log, err := rangeLog(ctx, repo, from, to)
	if err != nil || strings.TrimSpace(log) == "" {
		return ""
	}
	return capContext("COMMITS IN THIS RANGE (newest first):\n" + log)
}

// CommitContext is the prompt context LoadRev uses for one commit message.
func CommitContext(message string) string {
	return capContext("COMMIT MESSAGE:\n" + strings.TrimSpace(message) + "\n")
}

// ShortRev shortens a leading 40-hex sha to 12 chars, keeping any suffix
// such as "^" (other revisions are returned unchanged).
func ShortRev(rev string) string { return shortRev(rev) }
