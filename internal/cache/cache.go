// Package cache stores model results, per-file summaries, the user's
// overrides (edited groupings) and bytes-per-token calibration on disk.
//
// Layout under Dir (default $XDG_CACHE_HOME/clump, else ~/.cache/clump):
//
//	results/<key[0:2]>/<key>.json        model results (Entry)
//	summaries/<key[0:2]>/<key>.json      tier-2 per-file summaries (Summary)
//	overrides/<hash[0:2]>/<hash>.json    user edits keyed by diff hash (Override)
//	calibration.json                     provider name -> bytes per token
//
// Every write goes to a temp file in the same directory, then os.Rename. A
// cache hit is exactly one ReadFile of the entry file, nothing else.
package cache

import (
	"errors"
	"io/fs"
	"time"

	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/provider"
)

// FormatVersion is written into every file; files with another version are
// treated as a miss (results, summaries) or an error (overrides).
const FormatVersion = 1

// ErrVersion is returned (wrapped) for an override file with another version.
var ErrVersion = errors.New("unsupported cache file version")

// FS is the filesystem the Store uses. Tests pass a counting fake.
type FS interface {
	ReadFile(name string) ([]byte, error)
	// WriteFileAtomic writes data to a temp file in filepath.Dir(name), then
	// renames it to name, creating parent directories (0700) as needed. The
	// file mode is 0600.
	WriteFileAtomic(name string, data []byte) error
	Remove(name string) error
	Stat(name string) (fs.FileInfo, error)
}

// OSFS is the real filesystem.
func OSFS() FS {
	panic("cache: OSFS not implemented")
}

// Options configures Open.
type Options struct {
	// Dir is the cache root; "" means DefaultDir().
	Dir string
	// NoCache makes GetResult and GetSummary always miss (--no-cache).
	// Writes still happen. Overrides and calibration are unaffected.
	NoCache bool
	// FS defaults to OSFS().
	FS FS
}

// Store is a handle on the cache directory. Safe for concurrent use.
type Store struct {
	dir     string
	noCache bool
	fs      FS
}

// DefaultDir is $XDG_CACHE_HOME/clump, or ~/.cache/clump when
// XDG_CACHE_HOME is unset or not absolute.
func DefaultDir() (string, error) {
	panic("cache: DefaultDir not implemented")
}

// Open returns a Store. It does not touch the filesystem.
func Open(o Options) (*Store, error) {
	panic("cache: Open not implemented")
}

// Dir returns the cache root.
func (s *Store) Dir() string { return s.dir }

// Key is the result cache key: lower-case hex SHA-256 of
// diffHash + "\x00" + providerName + "\x00" + modelID + "\x00" +
// group.PromptVersion + "\x00" + decimal(maxGroups).
func Key(diffHash, providerName, modelID string, maxGroups int) string {
	panic("cache: Key not implemented")
}

// SummaryKey keys a tier-2 summary: SHA-256 hex of
// fileHash + "\x00" + providerName + "\x00" + modelID + "\x00" + group.PromptVersion.
func SummaryKey(fileHash, providerName, modelID string) string {
	panic("cache: SummaryKey not implemented")
}

// Entry is one cached model result.
type Entry struct {
	Version       int            `json:"version"`
	Key           string         `json:"key"`
	DiffHash      string         `json:"diff_hash"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	PromptVersion string         `json:"prompt_version"`
	MaxGroups     int            `json:"max_groups"`
	CreatedAt     time.Time      `json:"created_at"`
	Result        group.Result   `json:"result"`
	Usage         provider.Usage `json:"usage"`
}

// GetResult returns the entry for key. A missing file, a corrupt file or
// another FormatVersion is a miss (nil, false, nil); only unexpected I/O
// errors are returned.
func (s *Store) GetResult(key string) (*Entry, bool, error) {
	panic("cache: GetResult not implemented")
}

// PutResult writes e under e.Key (which must be non-empty), setting
// e.Version = FormatVersion.
func (s *Store) PutResult(e Entry) error {
	panic("cache: PutResult not implemented")
}

// Summary is one cached per-file summary.
type Summary struct {
	Version   int       `json:"version"`
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

// GetSummary is GetResult for summaries.
func (s *Store) GetSummary(key string) (*Summary, bool, error) {
	panic("cache: GetSummary not implemented")
}

// PutSummary is PutResult for summaries (sets Version).
func (s *Store) PutSummary(sm Summary) error {
	panic("cache: PutSummary not implemented")
}

// Snapshot is a named copy of a grouping kept inside an Override.
type Snapshot struct {
	Name     string         `json:"name"`
	At       time.Time      `json:"at"`
	Grouping group.Grouping `json:"grouping"`
}

// Override is the user's edited grouping for one diff.
type Override struct {
	Version  int    `json:"version"`
	DiffHash string `json:"diff_hash"`
	// BaseCacheKey is the result Key the edits started from ("" if they
	// started from a fallback or an unsaved result).
	BaseCacheKey string         `json:"base_cache_key"`
	EditedAt     time.Time      `json:"edited_at"`
	Grouping     group.Grouping `json:"grouping"`
	Snapshots    []Snapshot     `json:"snapshots"`
}

// LoadOverride returns the override for diffHash; (nil, false, nil) if none.
// A corrupt file or another version is an error, so edits are never
// silently lost.
func (s *Store) LoadOverride(diffHash string) (*Override, bool, error) {
	panic("cache: LoadOverride not implemented")
}

// SaveOverride writes o (sets Version) under o.DiffHash.
func (s *Store) SaveOverride(o Override) error {
	panic("cache: SaveOverride not implemented")
}

// DeleteOverride removes the override for diffHash; no error if absent.
func (s *Store) DeleteOverride(diffHash string) error {
	panic("cache: DeleteOverride not implemented")
}

// Calibration returns the stored bytes-per-token for a provider name.
func (s *Store) Calibration(providerName string) (float64, bool) {
	panic("cache: Calibration not implemented")
}

// Calibrate folds one observation into the stored bytes-per-token for a
// provider: obs = promptBytes / (inputTokens - overheadTokens), ignored when
// that denominator is < 100 or obs is outside [0.5, 10]. The stored value
// becomes obs if none existed, else 0.7*old + 0.3*obs. It returns the new
// value (or the old one when the observation was ignored).
func (s *Store) Calibrate(providerName string, promptBytes, inputTokens, overheadTokens int) (float64, error) {
	panic("cache: Calibrate not implemented")
}
