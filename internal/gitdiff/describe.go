package gitdiff

import (
	"context"
	"strings"
)

func CommitMessage(ctx context.Context, repo, rev string) (hash, subject, message string, err error) {
	out, err := runGit(ctx, repo, "log", "-1", "--format=%H%x00%s%x00%B", rev, "--")
	if err != nil {
		return "", "", "", err
	}
	hash, subject, message = splitLogRecord(out)
	return hash, subject, strings.TrimSpace(message), nil
}

func RangeContext(ctx context.Context, repo, from, to string) string {
	log, err := rangeLog(ctx, repo, from, to)
	if err != nil || strings.TrimSpace(log) == "" {
		return ""
	}
	return capContext("COMMITS IN THIS RANGE (newest first):\n" + log)
}

func CommitContext(message string) string {
	return capContext("COMMIT MESSAGE:\n" + strings.TrimSpace(message) + "\n")
}

func ShortRev(rev string) string { return shortRev(rev) }
