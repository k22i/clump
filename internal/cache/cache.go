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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
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
func OSFS() FS { return osFS{} }

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

	// mu serialises the read-modify-write of calibration.json, the only
	// multi-field file written by the Store.
	mu sync.Mutex
}

// DefaultDir is $XDG_CACHE_HOME/clump, or ~/.cache/clump when
// XDG_CACHE_HOME is unset or not absolute.
func DefaultDir() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "clump"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cache: home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "clump"), nil
}

// Open returns a Store. It does not touch the filesystem.
func Open(o Options) (*Store, error) {
	dir := o.Dir
	if dir == "" {
		d, err := DefaultDir()
		if err != nil {
			return nil, err
		}
		dir = d
	}
	f := o.FS
	if f == nil {
		f = OSFS()
	}
	return &Store{dir: dir, noCache: o.NoCache, fs: f}, nil
}

// Dir returns the cache root.
func (s *Store) Dir() string { return s.dir }

// Key is the result cache key: lower-case hex SHA-256 of
// diffHash + "\x00" + providerName + "\x00" + modelID + "\x00" +
// group.PromptVersion + "\x00" + decimal(maxGroups).
func Key(diffHash, providerName, modelID string, maxGroups int) string {
	return hashFields(diffHash, providerName, modelID, group.PromptVersion, strconv.Itoa(maxGroups))
}

// KeyWith is Key for a request that left some files out (generated files):
// skip names them (generated.Signature). An empty skip gives Key, so caches
// written before files could be left out stay valid.
func KeyWith(diffHash, providerName, modelID string, maxGroups int, skip string) string {
	if skip == "" {
		return Key(diffHash, providerName, modelID, maxGroups)
	}
	return hashFields(diffHash, providerName, modelID, group.PromptVersion, strconv.Itoa(maxGroups), "skip="+skip)
}

// SummaryKey keys a tier-2 summary: SHA-256 hex of
// fileHash + "\x00" + providerName + "\x00" + modelID + "\x00" + group.PromptVersion.
func SummaryKey(fileHash, providerName, modelID string) string {
	return hashFields(fileHash, providerName, modelID, group.PromptVersion)
}

// hashFields is the lower-case hex SHA-256 of fields joined with "\x00".
func hashFields(fields ...string) string {
	h := sha256.New()
	for i, f := range fields {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// keyRE is what a cache key or diff hash may look like: lower-case hex, long
// enough to be a real digest. Anything else could escape Dir through the
// shard or file name, so it is rejected.
var keyRE = regexp.MustCompile(`^[0-9a-f]{16,128}$`)

// checkName reports whether name is usable as a sharded file name.
func checkName(name string) error {
	if !keyRE.MatchString(name) {
		return fmt.Errorf("cache: invalid key %q", name)
	}
	return nil
}

// sharded returns <dir>/<kind>/<name[0:2]>/<name>.json.
func (s *Store) sharded(kind, name string) string {
	return filepath.Join(s.dir, kind, name[:2], name+".json")
}

// resultPath is where the result for key lives.
func (s *Store) resultPath(key string) string { return s.sharded("results", key) }

// summaryPath is where the summary for key lives.
func (s *Store) summaryPath(key string) string { return s.sharded("summaries", key) }

// overridePath is where the override for diffHash lives.
func (s *Store) overridePath(hash string) string { return s.sharded("overrides", hash) }

// calibrationPath is the single calibration file.
func (s *Store) calibrationPath() string { return filepath.Join(s.dir, "calibration.json") }

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
	if err := checkName(key); err != nil {
		return nil, false, err
	}
	if s.noCache {
		return nil, false, nil
	}
	data, err := s.fs.ReadFile(s.resultPath(key))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cache: read result %s: %w", key, err)
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, false, nil
	}
	if e.Version != FormatVersion || e.Key != key {
		return nil, false, nil
	}
	return &e, true, nil
}

// PutResult writes e under e.Key (which must be non-empty), setting
// e.Version = FormatVersion.
func (s *Store) PutResult(e Entry) error {
	if err := checkName(e.Key); err != nil {
		return err
	}
	e.Version = FormatVersion
	data, err := json.MarshalIndent(e, "", " ")
	if err != nil {
		return fmt.Errorf("cache: encode result %s: %w", e.Key, err)
	}
	return s.fs.WriteFileAtomic(s.resultPath(e.Key), data)
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
	if err := checkName(key); err != nil {
		return nil, false, err
	}
	if s.noCache {
		return nil, false, nil
	}
	data, err := s.fs.ReadFile(s.summaryPath(key))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cache: read summary %s: %w", key, err)
	}
	var sm Summary
	if err := json.Unmarshal(data, &sm); err != nil {
		return nil, false, nil
	}
	if sm.Version != FormatVersion || sm.Key != key {
		return nil, false, nil
	}
	return &sm, true, nil
}

// PutSummary is PutResult for summaries (sets Version).
func (s *Store) PutSummary(sm Summary) error {
	if err := checkName(sm.Key); err != nil {
		return err
	}
	sm.Version = FormatVersion
	data, err := json.MarshalIndent(sm, "", " ")
	if err != nil {
		return fmt.Errorf("cache: encode summary %s: %w", sm.Key, err)
	}
	return s.fs.WriteFileAtomic(s.summaryPath(sm.Key), data)
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
	if err := checkName(diffHash); err != nil {
		return nil, false, err
	}
	data, err := s.fs.ReadFile(s.overridePath(diffHash))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cache: read override %s: %w", diffHash, err)
	}
	var o Override
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, false, fmt.Errorf("cache: override %s: %w", diffHash, err)
	}
	if o.Version != FormatVersion {
		return nil, false, fmt.Errorf("cache: override %s: %w", diffHash, ErrVersion)
	}
	return &o, true, nil
}

// SaveOverride writes o (sets Version) under o.DiffHash.
func (s *Store) SaveOverride(o Override) error {
	if err := checkName(o.DiffHash); err != nil {
		return err
	}
	o.Version = FormatVersion
	data, err := json.MarshalIndent(o, "", " ")
	if err != nil {
		return fmt.Errorf("cache: encode override %s: %w", o.DiffHash, err)
	}
	return s.fs.WriteFileAtomic(s.overridePath(o.DiffHash), data)
}

// DeleteOverride removes the override for diffHash; no error if absent.
func (s *Store) DeleteOverride(diffHash string) error {
	if err := checkName(diffHash); err != nil {
		return err
	}
	err := s.fs.Remove(s.overridePath(diffHash))
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cache: delete override %s: %w", diffHash, err)
	}
	return nil
}

// Calibration returns the stored bytes-per-token for a provider name.
func (s *Store) Calibration(providerName string) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.readCalibration()[providerName]
	return v, ok
}

// Calibrate folds one observation into the stored bytes-per-token for a
// provider: obs = promptBytes / (inputTokens - overheadTokens), ignored when
// that denominator is < 100 or obs is outside [0.5, 10]. The stored value
// becomes obs if none existed, else 0.7*old + 0.3*obs. It returns the new
// value (or the old one when the observation was ignored).
func (s *Store) Calibrate(providerName string, promptBytes, inputTokens, overheadTokens int) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	values := s.readCalibration()
	old := values[providerName]
	den := inputTokens - overheadTokens
	if den < 100 {
		return old, nil
	}
	obs := float64(promptBytes) / float64(den)
	if obs < 0.5 || obs > 10 {
		return old, nil
	}
	newValue := obs
	if _, ok := values[providerName]; ok {
		newValue = 0.7*old + 0.3*obs
	}
	values[providerName] = newValue
	data, err := json.MarshalIndent(values, "", " ")
	if err != nil {
		return old, fmt.Errorf("cache: encode calibration: %w", err)
	}
	if err := s.fs.WriteFileAtomic(s.calibrationPath(), data); err != nil {
		return old, fmt.Errorf("cache: write calibration: %w", err)
	}
	return newValue, nil
}

// readCalibration returns the stored calibrations, or an empty map when the
// file is missing or unreadable/unparsable. Callers hold s.mu.
func (s *Store) readCalibration() map[string]float64 {
	values := map[string]float64{}
	data, err := s.fs.ReadFile(s.calibrationPath())
	if err != nil {
		return values
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return map[string]float64{}
	}
	return values
}
