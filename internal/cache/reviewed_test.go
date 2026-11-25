package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewed(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := strings.Repeat("a", 64), strings.Repeat("b", 64), "a"+strings.Repeat("c", 63)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := s.SetReviewed([]string{a, b, c}, true, now); err != nil {
		t.Fatal(err)
	}
	got, err := s.Reviewed([]string{a, b, c, strings.Repeat("d", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if !got[a] || !got[b] || !got[c] || len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	if err := s.SetReviewed([]string{a}, false, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Reviewed([]string{a, c}); got[a] || !got[c] {
		t.Fatalf("after unmark: %v", got)
	}
	// Unmarking the last key of a shard removes the shard file.
	if err := s.SetReviewed([]string{b}, false, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "reviewed", "b.json")); !os.IsNotExist(err) {
		t.Fatalf("shard b kept: %v", err)
	}
	// Old marks are pruned when their shard is written.
	later := now.Add(ReviewedTTL + time.Hour)
	if err := s.SetReviewed([]string{"a" + strings.Repeat("e", 63)}, true, later); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Reviewed([]string{c}); got[c] {
		t.Fatal("expired mark kept")
	}
	// A corrupt shard reads as empty and is rewritten.
	if err := os.WriteFile(filepath.Join(dir, "reviewed", "f.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := strings.Repeat("f", 64)
	if got, err := s.Reviewed([]string{f}); err != nil || got[f] {
		t.Fatalf("corrupt shard: %v %v", got, err)
	}
	if err := s.SetReviewed([]string{f}, true, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Reviewed([]string{f}); !got[f] {
		t.Fatal("rewrite of a corrupt shard")
	}
	// Keys that could escape the directory are refused.
	if _, err := s.Reviewed([]string{"../x"}); err == nil {
		t.Fatal("bad key accepted")
	}
}
