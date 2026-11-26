package cache

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	if err := s.DeleteComment(a, c1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "comments", "a.json")); !os.IsNotExist(err) {
		t.Fatalf("shard a kept: %v", err)
	}
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
	later := now.Add(CommentTTL + time.Hour)
	if _, err := s.AddComment(f, "fresh", later); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Comments([]string{f})
	if len(got[f]) != 1 || got[f][0].Text != "fresh" {
		t.Fatalf("expired comment kept: %+v", got[f])
	}
	if err := os.WriteFile(filepath.Join(dir, "comments", "a.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Comments([]string{a}); err != nil || len(got) != 0 {
		t.Fatalf("corrupt shard: %v %v", got, err)
	}
	if _, err := s.AddComment(a, "over corrupt", now); err != nil {
		t.Fatal(err)
	}
	all, err := s.AllComments(later)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[f][0].Text != "fresh" {
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
}

func TestStampHandoffMarksExistingCommentsOnlyOnce(t *testing.T) {
	s, _ := Open(Options{Dir: t.TempDir()})
	a, b := key('a'), key('b')
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if _, err := s.AddComment(a, "one", now); err != nil {
		t.Fatal(err)
	}
	later := now.Add(time.Hour)
	if _, err := s.AddComment(b, "two", later); err != nil {
		t.Fatal(err)
	}
	stampAt := now.Add(2 * time.Hour)
	n, err := s.StampHandoff([]string{a, key('c')}, stampAt)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("stamped %d, want 1", n)
	}
	got, _ := s.Comments([]string{a, b})
	if got[a][0].HandedOff.IsZero() {
		t.Fatalf("a was not stamped: %+v", got[a])
	}
	if want := stampAt.UTC().Truncate(time.Second); got[a][0].HandedOff != want {
		t.Fatalf("HandedOff = %v, want %v", got[a][0].HandedOff, want)
	}
	if !got[b][0].HandedOff.IsZero() {
		t.Fatalf("b was stamped without being asked: %+v", got[b])
	}
	n, err = s.StampHandoff([]string{a, b}, stampAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("second pass stamped %d, want only b", n)
	}
	got, _ = s.Comments([]string{a})
	if got[a][0].HandedOff != stampAt.UTC().Truncate(time.Second) {
		t.Fatalf("re-stamping moved the first record: %+v", got[a])
	}
	all, _ := s.AllComments(later.Add(24 * time.Hour))
	if all[b][0].HandedOff.IsZero() {
		t.Fatalf("AllComments lost the stamp: %+v", all[b])
	}
}

func TestCommentShardWithoutHandedOffStillLoads(t *testing.T) {
	dir := t.TempDir()
	old := "{\"version\":" + strconv.Itoa(FormatVersion) + ",\"comments\":{\"" + key('a') + "\":[{\"id\":\"ab12cd\",\"text\":\"before handoff existed\",\"at\":\"2026-01-02T03:04:05Z\"}]}}"
	if err := os.Mkdir(filepath.Join(dir, "comments"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "comments", "a.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Comments([]string{key('a')})
	if err != nil {
		t.Fatal(err)
	}
	cs := got[key('a')]
	if len(cs) != 1 || cs[0].ID != "ab12cd" || !cs[0].HandedOff.IsZero() {
		t.Fatalf("old shard read: %+v", cs)
	}
}
