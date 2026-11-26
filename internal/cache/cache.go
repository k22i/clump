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

const FormatVersion = 1

var ErrVersion = errors.New("unsupported cache file version")

type FS interface {
	ReadFile(name string) ([]byte, error)
	WriteFileAtomic(name string, data []byte) error
	Remove(name string) error
	Stat(name string) (fs.FileInfo, error)
}

func OSFS() FS { return osFS{} }

type Options struct {
	Dir     string
	NoCache bool
	FS      FS
}

type Store struct {
	dir     string
	noCache bool
	fs      FS

	mu sync.Mutex
}

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

func (s *Store) Dir() string { return s.dir }

func Key(diffHash, providerName, modelID string, maxGroups int) string {
	return hashFields(diffHash, providerName, modelID, group.PromptVersion, strconv.Itoa(maxGroups))
}

func KeyWith(diffHash, providerName, modelID string, maxGroups int, skip string) string {
	if skip == "" {
		return Key(diffHash, providerName, modelID, maxGroups)
	}
	return hashFields(diffHash, providerName, modelID, group.PromptVersion, strconv.Itoa(maxGroups), "skip="+skip)
}

func SummaryKey(fileHash, providerName, modelID string) string {
	return hashFields(fileHash, providerName, modelID, group.PromptVersion)
}

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

var keyRE = regexp.MustCompile(`^[0-9a-f]{16,128}$`)

func checkName(name string) error {
	if !keyRE.MatchString(name) {
		return fmt.Errorf("cache: invalid key %q", name)
	}
	return nil
}

func (s *Store) sharded(kind, name string) string {
	return filepath.Join(s.dir, kind, name[:2], name+".json")
}

func (s *Store) resultPath(key string) string { return s.sharded("results", key) }

func (s *Store) summaryPath(key string) string { return s.sharded("summaries", key) }

func (s *Store) overridePath(hash string) string { return s.sharded("overrides", hash) }

func (s *Store) calibrationPath() string { return filepath.Join(s.dir, "calibration.json") }

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

type Summary struct {
	Version   int       `json:"version"`
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

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

type Snapshot struct {
	Name     string         `json:"name"`
	At       time.Time      `json:"at"`
	Grouping group.Grouping `json:"grouping"`
}

type Override struct {
	Version      int            `json:"version"`
	DiffHash     string         `json:"diff_hash"`
	BaseCacheKey string         `json:"base_cache_key"`
	EditedAt     time.Time      `json:"edited_at"`
	Grouping     group.Grouping `json:"grouping"`
	Snapshots    []Snapshot     `json:"snapshots"`
}

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

func (s *Store) Calibration(providerName string) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.readCalibration()[providerName]
	return v, ok
}

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
