package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"time"
)

// Reviewed hunks are kept by content (gitdiff.Hunk.ReviewKey), not by diff:
// a hunk reviewed in one commit stays reviewed in the branch range that
// contains it, after a regroup, an edit, a provider switch or a rebase that
// only moves it, and becomes unreviewed as soon as its text changes.
//
// They live in 16 shards, reviewed/<key[0]>.json, each
//
//	{"version":1,"hunks":{"<key>":"<RFC 3339 time>", ...}}
//
// Entries older than ReviewedTTL are dropped when their shard is written.

// ReviewedTTL is how long a reviewed mark is kept.
const ReviewedTTL = 180 * 24 * time.Hour

type reviewedShard struct {
	Version int                  `json:"version"`
	Hunks   map[string]time.Time `json:"hunks"`
}

func (s *Store) reviewedPath(shard byte) string {
	return filepath.Join(s.dir, "reviewed", string(shard)+".json")
}

// readReviewedShard reads one shard; a missing, corrupt or other-version
// file is an empty shard (reviewed marks are a convenience, not data that
// must never be lost).
func (s *Store) readReviewedShard(shard byte) reviewedShard {
	sh := reviewedShard{Version: FormatVersion, Hunks: map[string]time.Time{}}
	data, err := s.fs.ReadFile(s.reviewedPath(shard))
	if err != nil {
		return sh
	}
	var got reviewedShard
	if json.Unmarshal(data, &got) != nil || got.Version != FormatVersion || got.Hunks == nil {
		return sh
	}
	return got
}

// byShard groups valid keys by their shard; invalid keys are an error.
func byShard(keys []string) (map[byte][]string, error) {
	out := map[byte][]string{}
	for _, k := range keys {
		if err := checkName(k); err != nil {
			return nil, err
		}
		out[k[0]] = append(out[k[0]], k)
	}
	return out, nil
}

// Reviewed returns which of keys are marked reviewed.
func (s *Store) Reviewed(keys []string) (map[string]bool, error) {
	shards, err := byShard(keys)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(keys))
	for shard, ks := range shards {
		sh := s.readReviewedShard(shard)
		for _, k := range ks {
			if _, ok := sh.Hunks[k]; ok {
				out[k] = true
			}
		}
	}
	return out, nil
}

// SetReviewed marks keys reviewed (on) or not, at now. Each touched shard is
// re-read just before it is written, so two clump processes lose at most a
// mark made in the same instant.
func (s *Store) SetReviewed(keys []string, on bool, now time.Time) error {
	shards, err := byShard(keys)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order := make([]byte, 0, len(shards))
	for shard := range shards {
		order = append(order, shard)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	for _, shard := range order {
		sh := s.readReviewedShard(shard)
		for k, at := range sh.Hunks {
			if now.Sub(at) > ReviewedTTL {
				delete(sh.Hunks, k)
			}
		}
		for _, k := range shards[shard] {
			if on {
				sh.Hunks[k] = now.UTC().Truncate(time.Second)
			} else {
				delete(sh.Hunks, k)
			}
		}
		path := s.reviewedPath(shard)
		if len(sh.Hunks) == 0 {
			if err := s.fs.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("cache: reviewed: %w", err)
			}
			continue
		}
		sh.Version = FormatVersion
		data, err := json.Marshal(sh)
		if err != nil {
			return fmt.Errorf("cache: encode reviewed: %w", err)
		}
		if err := s.fs.WriteFileAtomic(path, data); err != nil {
			return fmt.Errorf("cache: write reviewed: %w", err)
		}
	}
	return nil
}
