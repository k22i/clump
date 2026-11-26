package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k22i/clump/internal/comment"
)

func key(hex byte) string { return strings.Repeat(string(hex), 64) }

func TestCommentsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	a, b := key('a'), key('b')
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c1, err := s.AddComment(a, "first note", now)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.AddComment(a, "second note", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddComment(b, "other anchor", now); err != nil {
		t.Fatal(err)
	}
	got, err := s.Comments([]string{a, b, key('c')})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[a]) != 2 || got[a][0].Text != "first note" ||
		got[a][1].Text != "second note" || got[a][1].ID != c2.ID || got[a][0].ID != c1.ID {
		t.Fatalf("comments: %+v", got)
	}
	if len(got[b]) != 1 || got[b][0].Text != "other anchor" {
		t.Fatalf("b: %+v", got[b])
	}
	// Empty text is refused.
	if _, err := s.AddComment(a, "  ", now); err == nil {
		t.Fatal("empty comment should fail")
	}
}

func TestCommentsOrderAndIDs(t *testing.T) {
	s, _ := Open(Options{Dir: t.TempDir()})
	a := key('a')
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	first, _ := s.AddComment(a, "one", now)
	second, _ := s.AddComment(a, "two...", now.Add(2*time.Second))
	if first.ID == second.ID {
		t.Fatal("different comments must have different ids")
	}
	got, _ := s.Comments([]string{a})
	if got[a][0].ID != first.ID || got[a][1].ID != second.ID {
		t.Fatalf("order: %+v", got[a])
	}
	if first.At != now.UTC() {
		t.Fatalf("time truncated wrong: %v", first.At)
	}
}

func TestCommentDelete(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(Options{Dir: dir})
	a := key('a')
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c1, _ := s.AddComment(a, "keep", now)
	c2, _ := s.AddComment(a, "drop", now.Add(time.Second))
	if err := s.DeleteComment(a, c2.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Comments([]string{a})
	if len(got[a]) != 1 || got[a][0].ID != c1.ID {
		t.Fatalf("after delete: %+v", got[a])
	}
	// Deleting the last comment removes the shard file.
	if err := s.DeleteComment(a, c1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "comments", "a.json")); !os.IsNotExist(err) {
		t.Fatalf("shard a kept: %v", err)
	}
	// Idempotent: unknown ids and keys are fine.
	if err := s.DeleteComment(a, "nope"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteComment(key('9'), "nope"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Comments([]string{a}); len(got) != 0 {
		t.Fatalf("comments left: %+v", got)
	}
}

func TestCommentsTTLAndCorruptShard(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(Options{Dir: dir})
	a, f := key('a'), key('f')
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.AddComment(f, "will expire", now); err != nil {
		t.Fatal(err)
	}
	// Adding to the same shard later prunes the expired entry.
	later := now.Add(CommentTTL + time.Hour)
	if _, err := s.AddComment(f, "fresh", later); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Comments([]string{f})
	if len(got[f]) != 1 || got[f][0].Text != "fresh" {
		t.Fatalf("expired comment kept: %+v", got[f])
	}
	// A corrupt shard reads as empty, and writing works over it.
	if err := os.WriteFile(filepath.Join(dir, "comments", "a.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Comments([]string{a}); err != nil || len(got) != 0 {
		t.Fatalf("corrupt shard: %v %v", got, err)
	}
	if _, err := s.AddComment(a, "over corrupt", now); err != nil {
		t.Fatal(err)
	}
	// AllComments returns by anchor, TTL-dropped.
	all, err := s.AllComments(later)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[f][0].Text != "fresh" { // "over corrupt" is past the TTL
		t.Fatalf("all comments: %+v", all)
	}
}

func TestCommentIDDeterministic(t *testing.T) {
	now := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	if CommentID("k", now, "t") != CommentID("k", now, "t") {
		t.Fatal("same input must give the same id")
	}
	if CommentID("k", now, "t") == CommentID("k2", now, "t") {
		t.Fatal("different anchors must differ")
	}
	if len(CommentID("k", now, "t")) != 12 {
		t.Fatal("id is 12 hex digits")
	}
}

func TestCommentShardVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(Options{Dir: dir})
	a := key('a')
	if err := os.MkdirAll(filepath.Join(dir, "comments"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "comments", "a.json"),
		[]byte(`{"version":2,"comments":{"`+a+`":[{"id":"x","text":"y","at":"2026-01-01T00:00:00Z"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s.Comments([]string{a})
	if err != nil || len(got) != 0 {
		t.Fatalf("other version should read as empty: %v %v", got, err)
	}
	var _ comment.Kind // keep the import shape stable
}
