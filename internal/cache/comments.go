package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/k22i/clump/internal/comment"
)

const CommentTTL = 180 * 24 * time.Hour

type commentShard struct {
	Version  int                        `json:"version"`
	Comments map[string][]commentStored `json:"comments"`
}

type commentStored struct {
	ID   string    `json:"id"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

func (s *Store) commentsPath(shard byte) string {
	return filepath.Join(s.dir, "comments", string(shard)+".json")
}

func (s *Store) readCommentShard(shard byte) commentShard {
	sh := commentShard{Version: FormatVersion, Comments: map[string][]commentStored{}}
	data, err := s.fs.ReadFile(s.commentsPath(shard))
	if err != nil {
		return sh
	}
	var got commentShard
	if json.Unmarshal(data, &got) != nil || got.Version != FormatVersion || got.Comments == nil {
		return sh
	}
	return got
}

func CommentID(key string, at time.Time, text string) string {
	sum := sha256.Sum256([]byte(key + "\x00" + at.UTC().Format(time.RFC3339Nano) + "\x00" + text))
	return hex.EncodeToString(sum[:6])
}

func (s *Store) Comments(keys []string) (map[string][]comment.Comment, error) {
	shards, err := byShard(keys)
	if err != nil {
		return nil, err
	}
	out := map[string][]comment.Comment{}
	for shard, ks := range shards {
		sh := s.readCommentShard(shard)
		for _, k := range ks {
			if len(sh.Comments[k]) == 0 {
				continue
			}
			cs := make([]comment.Comment, 0, len(sh.Comments[k]))
			for _, c := range sh.Comments[k] {
				cs = append(cs, comment.Comment{ID: c.ID, Text: c.Text, At: c.At})
			}
			out[k] = cs
		}
	}
	return out, nil
}

func (s *Store) AddComment(key, text string, now time.Time) (comment.Comment, error) {
	if strings.TrimSpace(text) == "" {
		return comment.Comment{}, errors.New("cache: empty comment")
	}
	if err := checkName(key); err != nil {
		return comment.Comment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	shard := key[0]
	sh := s.readCommentShard(shard)
	at := now.UTC().Truncate(time.Second)
	c := comment.Comment{ID: CommentID(key, at, text), Text: text, At: at}
	kept := sh.Comments[key][:0]
	for _, old := range sh.Comments[key] {
		if now.Sub(old.At) <= CommentTTL && old.ID != c.ID {
			kept = append(kept, old)
		}
	}
	sh.Comments[key] = append(kept, commentStored{ID: c.ID, Text: c.Text, At: c.At})
	if err := s.writeCommentShard(shard, sh); err != nil {
		return comment.Comment{}, err
	}
	return c, nil
}

func (s *Store) DeleteComment(key, id string) error {
	if err := checkName(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	shard := key[0]
	sh := s.readCommentShard(shard)
	cs, ok := sh.Comments[key]
	if !ok {
		return nil
	}
	kept := make([]commentStored, 0, len(cs))
	for _, c := range cs {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	if len(kept) == len(cs) {
		return nil
	}
	if len(kept) > 0 {
		sh.Comments[key] = kept
	} else {
		delete(sh.Comments, key)
	}
	return s.writeCommentShard(shard, sh)
}

func (s *Store) AllComments(now time.Time) (map[string][]comment.Comment, error) {
	out := map[string][]comment.Comment{}
	for _, shard := range []byte("0123456789abcdef") {
		sh := s.readCommentShard(shard)
		for k, cs := range sh.Comments {
			for _, c := range cs {
				if now.Sub(c.At) > CommentTTL {
					continue
				}
				out[k] = append(out[k], comment.Comment{ID: c.ID, Text: c.Text, At: c.At})
			}
		}
	}
	return out, nil
}

func (s *Store) writeCommentShard(shard byte, sh commentShard) error {
	sh.Version = FormatVersion
	path := s.commentsPath(shard)
	if len(sh.Comments) == 0 {
		if err := s.fs.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("cache: comments: %w", err)
		}
		return nil
	}
	data, err := json.Marshal(sh)
	if err != nil {
		return fmt.Errorf("cache: encode comments: %w", err)
	}
	if err := s.fs.WriteFileAtomic(path, data); err != nil {
		return fmt.Errorf("cache: write comments: %w", err)
	}
	return nil
}
