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

// Review comments are stored by anchor key (internal/comment), not by diff:
// the same rules as reviewed marks — a comment follows the content it was
// written on across diffs, regroupings and edits, and disappears when that
// content changes. Several comments can sit on one anchor.
//
// They live in 16 shards, comments/<key[0]>.json, each
//
//	{"version":1,"comments":{"<anchorKey>":[{"id","text","at"}, ...]}}
//
// A comment's id is the first 12 hex digits of sha256(key \0 at \0 text), so
// it can be deleted without reading it first. Entries whose comments are
// all older than CommentTTL are dropped when their shard is written.

// CommentTTL is how long a comment is kept.
const CommentTTL = 180 * 24 * time.Hour

// commentShard is one disk file.
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

// readCommentShard reads one shard; a missing, corrupt or other-version
// file is an empty shard (comments are review notes, not data that must
// never be lost).
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

// CommentID builds the id of a comment written at at with text on key.
func CommentID(key string, at time.Time, text string) string {
	sum := sha256.Sum256([]byte(key + "\x00" + at.UTC().Format(time.RFC3339Nano) + "\x00" + text))
	return hex.EncodeToString(sum[:6])
}

// Comments returns the comments stored for each key that has any.
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

// AddComment stores a comment on key and returns it with its id filled in.
// The touched shard is re-read just before it is written, so two clump
// processes lose at most a comment made in the same instant.
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
	// TTL-drop old entries and new duplicates with the same content and
	// timestamp (the id covers them).
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

// DeleteComment removes the comment with id from key. Unknown keys and ids
// are not errors: deletion is idempotent.
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
		return nil // id unknown: nothing to do
	}
	if len(kept) > 0 {
		sh.Comments[key] = kept
	} else {
		delete(sh.Comments, key)
	}
	return s.writeCommentShard(shard, sh)
}

// AllComments returns every stored comment with its anchor key, for the
// comments CLI's delete-by-id (which knows no key). TTL-dropped entries are
// not returned.
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

// writeCommentShard writes the shard atomically, dropping empties.
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
