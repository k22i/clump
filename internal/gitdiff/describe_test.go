package gitdiff

import (
	"context"
	"strings"
	"testing"
)

func TestDescribeHelpers(t *testing.T) {
	ctx := context.Background()
	repo := fxRepo
	sha := strings.TrimSpace(gitOut(t, repo, "log", "--format=%H", "-F", "--grep=Initial commit", "main"))
	hash, subject, msg, err := CommitMessage(ctx, repo, sha)
	if err != nil || hash != sha || subject != "Initial commit" || msg != "Initial commit\n\nSeed the fixture repo." {
		t.Fatalf("CommitMessage = %q %q %q %v", hash, subject, msg, err)
	}
	if got := CommitContext(msg); got != "COMMIT MESSAGE:\nInitial commit\n\nSeed the fixture repo.\n" {
		t.Fatalf("CommitContext = %q", got)
	}
	if got := RangeContext(ctx, repo, "main", "feature2"); !strings.HasPrefix(got, "COMMITS IN THIS RANGE (newest first):\n- ") || !strings.Contains(got, "Start feature two") {
		t.Fatalf("RangeContext = %q", got)
	}
	if got := RangeContext(ctx, repo, "nope", "main"); got != "" {
		t.Fatalf("RangeContext of a bad rev = %q", got)
	}
	if ShortRev(sha+"^") != sha[:12]+"^" || ShortRev("main") != "main" {
		t.Fatal("ShortRev")
	}
	if _, _, _, err := CommitMessage(ctx, repo, "nope"); err == nil {
		t.Fatal("want error")
	}
}
