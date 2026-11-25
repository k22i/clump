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
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/k22i/clump/internal/group"
	"github.com/k22i/clump/internal/provider"
)

// sampleResult is a small valid grouping for round-trip tests.
func sampleResult() group.Result {
	return group.Result{
		Grouping: group.Grouping{Groups: []group.Group{{ID: "g1", Title: "T", Hunks: []string{"h1"}}}},
		Notices:  []string{"n"},
	}
}

// sampleEntry is a complete result entry keyed by key.
func sampleEntry(key string) Entry {
	return Entry{
		Key:           key,
		DiffHash:      "d1",
		Provider:      "p",
		Model:         "m",
		PromptVersion: group.PromptVersion,
		MaxGroups:     12,
		CreatedAt:     time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC),
		Result:        sampleResult(),
		Usage:         provider.Usage{InputTokens: 10, OutputTokens: 20, CostUSD: 0.5, CostSource: provider.CostReported},
	}
}

// hex64 is a valid 64-character lower-case hex key/hash for tests.
func hex64(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

// openStore returns a Store in dir with a counting FS.
func openStore(t *testing.T, dir string, o Options) (*Store, *countingFS) {
	t.Helper()
	cfs := newCountingFS()
	o.Dir = dir
	o.FS = cfs
	s, err := Open(o)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, cfs
}

// fsCall is one recorded call on a countingFS.
type fsCall struct {
	Op   string
	Path string
}

// countingFS wraps OSFS and records every call it receives.
type countingFS struct {
	inner FS
	mu    sync.Mutex
	calls []fsCall
}

func newCountingFS() *countingFS { return &countingFS{inner: OSFS()} }

func (c *countingFS) record(op, path string) {
	c.mu.Lock()
	c.calls = append(c.calls, fsCall{Op: op, Path: path})
	c.mu.Unlock()
}

// reset empties the call log.
func (c *countingFS) reset() {
	c.mu.Lock()
	c.calls = nil
	c.mu.Unlock()
}

// snapshot returns a copy of the call log.
func (c *countingFS) snapshot() []fsCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]fsCall(nil), c.calls...)
}

func (c *countingFS) ReadFile(name string) ([]byte, error) {
	c.record("ReadFile", name)
	return c.inner.ReadFile(name)
}

func (c *countingFS) WriteFileAtomic(name string, data []byte) error {
	c.record("WriteFileAtomic", name)
	return c.inner.WriteFileAtomic(name, data)
}

func (c *countingFS) Remove(name string) error {
	c.record("Remove", name)
	return c.inner.Remove(name)
}

func (c *countingFS) Stat(name string) (fs.FileInfo, error) {
	c.record("Stat", name)
	return c.inner.Stat(name)
}

func TestDefaultDir(t *testing.T) {
	t.Run("xdg absolute", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "/x/y")
		t.Setenv("HOME", "/home/tester")
		got, err := DefaultDir()
		if err != nil {
			t.Fatalf("DefaultDir: %v", err)
		}
		if want := "/x/y/clump"; got != want {
			t.Errorf("DefaultDir() = %q, want %q", got, want)
		}
	})

	t.Run("xdg relative", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "rel")
		t.Setenv("HOME", "/home/tester")
		got, err := DefaultDir()
		if err != nil {
			t.Fatalf("DefaultDir: %v", err)
		}
		if want := filepath.Join("/home/tester", ".cache", "clump"); got != want {
			t.Errorf("DefaultDir() = %q, want %q", got, want)
		}
	})

	t.Run("xdg unset", func(t *testing.T) {
		old, had := os.LookupEnv("XDG_CACHE_HOME")
		if err := os.Unsetenv("XDG_CACHE_HOME"); err != nil {
			t.Fatalf("Unsetenv: %v", err)
		}
		t.Cleanup(func() {
			if had {
				if err := os.Setenv("XDG_CACHE_HOME", old); err != nil {
					t.Errorf("restore XDG_CACHE_HOME: %v", err)
				}
			}
		})
		t.Setenv("HOME", "/home/tester")
		got, err := DefaultDir()
		if err != nil {
			t.Fatalf("DefaultDir: %v", err)
		}
		if want := filepath.Join("/home/tester", ".cache", "clump"); got != want {
			t.Errorf("DefaultDir() = %q, want %q", got, want)
		}
	})
}

func TestOpenTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	s, cfs := openStore(t, dir, Options{})
	if got := cfs.snapshot(); len(got) != 0 {
		t.Fatalf("Open made %d FS calls, want 0: %+v", len(got), got)
	}
	if s.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", s.Dir(), dir)
	}
}

func TestKeyDeterministic(t *testing.T) {
	k := Key("a", "b", "c", 12)
	if len(k) != 64 {
		t.Fatalf("Key length = %d, want 64", len(k))
	}
	if again := Key("a", "b", "c", 12); again != k {
		t.Errorf("Key not deterministic: %q vs %q", k, again)
	}
	sum := sha256.Sum256([]byte("a\x00b\x00c\x00" + group.PromptVersion + "\x0012"))
	if want := hex.EncodeToString(sum[:]); k != want {
		t.Errorf("Key = %q, want %q", k, want)
	}
	others := map[string]string{
		"diffHash": Key("A", "b", "c", 12),
		"provider": Key("a", "B", "c", 12),
		"model":    Key("a", "b", "C", 12),
		"groups":   Key("a", "b", "c", 13),
	}
	for field, other := range others {
		if other == k {
			t.Errorf("Key unchanged when %s changes", field)
		}
	}
}

func TestSummaryKey(t *testing.T) {
	k := SummaryKey("a", "b", "c")
	if len(k) != 64 {
		t.Fatalf("SummaryKey length = %d, want 64", len(k))
	}
	if again := SummaryKey("a", "b", "c"); again != k {
		t.Errorf("SummaryKey not deterministic: %q vs %q", k, again)
	}
	sum := sha256.Sum256([]byte("a\x00b\x00c\x00" + group.PromptVersion))
	if want := hex.EncodeToString(sum[:]); k != want {
		t.Errorf("SummaryKey = %q, want %q", k, want)
	}
	others := map[string]string{
		"fileHash": SummaryKey("A", "b", "c"),
		"provider": SummaryKey("a", "B", "c"),
		"model":    SummaryKey("a", "b", "C"),
	}
	for field, other := range others {
		if other == k {
			t.Errorf("SummaryKey unchanged when %s changes", field)
		}
	}
}

func TestResultRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want := sampleEntry(hex64("result"))
	if err := s.PutResult(want); err != nil {
		t.Fatalf("PutResult: %v", err)
	}
	want.Version = FormatVersion // PutResult stamps the version

	got, ok, err := s.GetResult(want.Key)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if !ok {
		t.Fatal("GetResult missed after PutResult")
	}
	if got.Version != FormatVersion {
		t.Errorf("Version = %d, want %d", got.Version, FormatVersion)
	}
	got.CreatedAt = want.CreatedAt // time.Time keeps a monotonic reading the JSON lacks
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("entry mismatch:\n got %+v\nwant %+v", *got, want)
	}

	path := filepath.Join(dir, "results", want.Key[:2], want.Key+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat %s: %v", path, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o, want 600", perm)
	}
	dinfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := dinfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir mode = %o, want 700", perm)
	}
}

func TestResultMiss(t *testing.T) {
	dir := t.TempDir()
	s, cfs := openStore(t, dir, Options{})
	got, ok, err := s.GetResult(hex64("absent"))
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("GetResult = (%v, %v), want (nil, false)", got, ok)
	}
	if calls := cfs.snapshot(); len(calls) != 1 || calls[0].Op != "ReadFile" {
		t.Errorf("miss calls = %+v, want one ReadFile", calls)
	}
}

func TestHitIsOneRead(t *testing.T) {
	dir := t.TempDir()
	s, cfs := openStore(t, dir, Options{})
	e := sampleEntry(hex64("hit"))
	if err := s.PutResult(e); err != nil {
		t.Fatalf("PutResult: %v", err)
	}
	cfs.reset()

	if _, ok, err := s.GetResult(e.Key); err != nil || !ok {
		t.Fatalf("GetResult = (_, %v, %v), want a hit", ok, err)
	}
	calls := cfs.snapshot()
	if len(calls) != 1 {
		t.Fatalf("hit made %d FS calls, want 1: %+v", len(calls), calls)
	}
	if calls[0].Op != "ReadFile" {
		t.Errorf("hit call = %q, want ReadFile", calls[0].Op)
	}
	wantPath := filepath.Join(dir, "results", e.Key[:2], e.Key+".json")
	if calls[0].Path != wantPath {
		t.Errorf("hit read %q, want %q", calls[0].Path, wantPath)
	}
}

func TestNoCache(t *testing.T) {
	dir := t.TempDir()
	normal, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	e := sampleEntry(hex64("nocache"))
	if err := normal.PutResult(e); err != nil {
		t.Fatalf("PutResult: %v", err)
	}

	noCache, cfs := openStore(t, dir, Options{NoCache: true})
	got, ok, err := noCache.GetResult(e.Key)
	if err != nil {
		t.Fatalf("GetResult with NoCache: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("GetResult with NoCache = (%v, %v), want (nil, false)", got, ok)
	}
	if calls := cfs.snapshot(); len(calls) != 0 {
		t.Errorf("NoCache Get made %d FS calls, want 0: %+v", len(calls), calls)
	}

	e2 := sampleEntry(hex64("nocache-written"))
	if err := noCache.PutResult(e2); err != nil {
		t.Fatalf("PutResult with NoCache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "results", e2.Key[:2], e2.Key+".json")); err != nil {
		t.Fatalf("NoCache PutResult did not write the file: %v", err)
	}
	if _, ok, err := normal.GetResult(e2.Key); err != nil || !ok {
		t.Fatalf("normal GetResult = (_, %v, %v), want a hit", ok, err)
	}
	if _, ok, err := normal.GetSummary(e2.Key); err != nil || ok {
		t.Fatalf("GetSummary for unwritten key = (_, %v, %v), want a miss", ok, err)
	}
}

func TestCorruptAndVersionAreMisses(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	writeAt := func(path, body string) {
		t.Helper()
		if err := OSFS().WriteFileAtomic(path, []byte(body)); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	cases := []struct {
		name string
		body string
	}{
		{"garbage", "not json at all"},
		{"other version", `{"version":2,"key":"%s"}`},
		{"other key", `{"version":1,"key":"%s"}`},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := hex64(fmt.Sprintf("corrupt-%d", i))
			body := tc.body
			otherKey := key
			if tc.name == "other key" {
				otherKey = hex64("someone-else")
			}
			if strings.Contains(body, "%s") {
				body = fmt.Sprintf(body, otherKey)
			}
			path := filepath.Join(dir, "results", key[:2], key+".json")
			writeAt(path, body)

			got, ok, err := s.GetResult(key)
			if err != nil {
				t.Fatalf("GetResult: %v", err)
			}
			if ok || got != nil {
				t.Fatalf("GetResult = (%v, %v), want (nil, false)", got, ok)
			}
		})
	}

	t.Run("summary", func(t *testing.T) {
		key := SummaryKey("f", "p", "m")
		writeAt(filepath.Join(dir, "summaries", key[:2], key+".json"), `{"version":2,"key":"`+key+`"}`)
		if got, ok, err := s.GetSummary(key); err != nil || ok || got != nil {
			t.Fatalf("GetSummary = (%v, %v, %v), want (nil, false, nil)", got, ok, err)
		}
	})
}

func TestInvalidKey(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	traversal := filepath.Join(filepath.Dir(dir), "etc", "passwd")

	bad := []string{"../../etc/passwd", "ABC0123456789abcdef", "ab", ""}
	for _, key := range bad {
		t.Run(fmt.Sprintf("%q", key), func(t *testing.T) {
			if err := s.PutResult(Entry{Key: key}); err == nil {
				t.Fatalf("PutResult(%q) = nil, want an error", key)
			} else if !strings.Contains(err.Error(), "invalid key") {
				t.Errorf("PutResult(%q) error = %v, want it to mention invalid key", key, err)
			}
			if _, _, err := s.GetResult(key); err == nil {
				t.Errorf("GetResult(%q) = nil error, want an error", key)
			} else if !strings.Contains(err.Error(), "invalid key") {
				t.Errorf("GetResult(%q) error = %v, want it to mention invalid key", key, err)
			}
			if err := s.PutSummary(Summary{Key: key}); err == nil || !strings.Contains(err.Error(), "invalid key") {
				t.Errorf("PutSummary(%q) error = %v, want invalid key", key, err)
			}
			if _, _, err := s.LoadOverride(key); err == nil || !strings.Contains(err.Error(), "invalid key") {
				t.Errorf("LoadOverride(%q) error = %v, want invalid key", key, err)
			}
			if err := s.SaveOverride(Override{DiffHash: key}); err == nil || !strings.Contains(err.Error(), "invalid key") {
				t.Errorf("SaveOverride(%q) error = %v, want invalid key", key, err)
			}
			if err := s.DeleteOverride(key); err == nil || !strings.Contains(err.Error(), "invalid key") {
				t.Errorf("DeleteOverride(%q) error = %v, want invalid key", key, err)
			}
		})
	}

	if _, err := os.Stat(traversal); err == nil {
		t.Errorf("an invalid key created %s outside the root", traversal)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir root: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("invalid keys created %d entries under root", len(entries))
	}
}

func TestAtomicWriteLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < 20; i++ {
		if err := s.PutResult(sampleEntry(hex64(fmt.Sprintf("atomic-%d", i)))); err != nil {
			t.Fatalf("PutResult %d: %v", i, err)
		}
	}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".tmp-") {
			t.Errorf("leftover temp file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
}

func TestAtomicWriteFailureCleansUp(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Log("running as root: read-only directory does not block writes, skipping the assertion")
		return
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "sub")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(target, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(target, 0o700); err != nil {
			t.Errorf("restore mode: %v", err)
		}
	})

	err := OSFS().WriteFileAtomic(filepath.Join(target, "x.json"), []byte("x"))
	if err == nil {
		t.Fatal("WriteFileAtomic in a read-only directory = nil, want an error")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed write left %d entries: %v", len(entries), entries)
	}
}

func TestSummaryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want := Summary{
		Key:       SummaryKey("filehash", "p", "m"),
		Path:      "internal/a.go",
		Summary:   "adds a thing",
		CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	if err := s.PutSummary(want); err != nil {
		t.Fatalf("PutSummary: %v", err)
	}
	want.Version = FormatVersion // PutSummary stamps the version
	got, ok, err := s.GetSummary(want.Key)
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if !ok {
		t.Fatal("GetSummary missed after PutSummary")
	}
	if got.Version != FormatVersion {
		t.Errorf("Version = %d, want %d", got.Version, FormatVersion)
	}
	got.CreatedAt = want.CreatedAt
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("summary mismatch:\n got %+v\nwant %+v", *got, want)
	}
	path := filepath.Join(dir, "summaries", want.Key[:2], want.Key+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat %s: %v", path, err)
	}
}

func TestOverrideRoundTrip(t *testing.T) {
	dir := t.TempDir()
	hash := hex64("override")
	want := Override{
		DiffHash:     hash,
		BaseCacheKey: hex64("base"),
		EditedAt:     time.Date(2024, 6, 7, 8, 9, 10, 0, time.UTC),
		Grouping:     sampleResult().Grouping,
		Snapshots: []Snapshot{
			{
				Name:     "before regroup 12:00",
				At:       time.Date(2024, 6, 7, 8, 0, 0, 0, time.UTC),
				Grouping: group.Grouping{Groups: []group.Group{{ID: "g1", Title: "old", Hunks: []string{"h1"}}}},
			},
			{
				Name:     "before regroup 12:10",
				At:       time.Date(2024, 6, 7, 8, 10, 0, 0, time.UTC),
				Grouping: group.Grouping{Groups: []group.Group{{ID: "g2", Title: "older", Hunks: []string{"h2"}}}},
			},
		},
	}
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.SaveOverride(want); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	want.Version = FormatVersion // SaveOverride stamps the version
	got, ok, err := s.LoadOverride(hash)
	if err != nil {
		t.Fatalf("LoadOverride: %v", err)
	}
	if !ok {
		t.Fatal("LoadOverride missed after SaveOverride")
	}
	if got.Version != FormatVersion {
		t.Errorf("Version = %d, want %d", got.Version, FormatVersion)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("override mismatch:\n got %+v\nwant %+v", *got, want)
	}
	path := filepath.Join(dir, "overrides", hash[:2], hash+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat %s: %v", path, err)
	}

	noCache, _ := openStore(t, dir, Options{NoCache: true})
	got, ok, err = noCache.LoadOverride(hash)
	if err != nil || !ok {
		t.Fatalf("NoCache LoadOverride = (_, %v, %v), want a hit", ok, err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("NoCache override mismatch:\n got %+v\nwant %+v", *got, want)
	}
}

func TestOverrideErrors(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Run("missing", func(t *testing.T) {
		got, ok, err := s.LoadOverride(hex64("missing"))
		if err != nil {
			t.Fatalf("LoadOverride: %v", err)
		}
		if ok || got != nil {
			t.Fatalf("LoadOverride = (%v, %v), want (nil, false)", got, ok)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		hash := hex64("corrupt")
		path := filepath.Join(dir, "overrides", hash[:2], hash+".json")
		if err := OSFS().WriteFileAtomic(path, []byte("{not json")); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, ok, err := s.LoadOverride(hash)
		if err == nil {
			t.Fatal("LoadOverride of a corrupt file = nil error, want one")
		}
		if ok {
			t.Error("LoadOverride reported a hit for a corrupt file")
		}
		if !strings.Contains(err.Error(), hash) {
			t.Errorf("error %q does not name the hash %q", err, hash)
		}
	})

	t.Run("other version", func(t *testing.T) {
		hash := hex64("version")
		path := filepath.Join(dir, "overrides", hash[:2], hash+".json")
		body := fmt.Sprintf(`{"version":2,"diff_hash":%q}`, hash)
		if err := OSFS().WriteFileAtomic(path, []byte(body)); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, ok, err := s.LoadOverride(hash)
		if !errors.Is(err, ErrVersion) {
			t.Fatalf("LoadOverride error = %v, want ErrVersion", err)
		}
		if ok {
			t.Error("LoadOverride reported a hit for a future version")
		}
	})

	t.Run("delete", func(t *testing.T) {
		hash := hex64("delete")
		if err := s.SaveOverride(Override{DiffHash: hash}); err != nil {
			t.Fatalf("SaveOverride: %v", err)
		}
		if err := s.DeleteOverride(hash); err != nil {
			t.Fatalf("DeleteOverride: %v", err)
		}
		if got, ok, err := s.LoadOverride(hash); err != nil || ok || got != nil {
			t.Fatalf("LoadOverride after delete = (%v, %v, %v), want (nil, false, nil)", got, ok, err)
		}
		if err := s.DeleteOverride(hash); err != nil {
			t.Errorf("second DeleteOverride: %v, want nil", err)
		}
	})
}

func TestSeparateDirs(t *testing.T) {
	dir := t.TempDir()
	shared := hex64("shared")
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.PutResult(sampleEntry(shared)); err != nil {
		t.Fatalf("PutResult: %v", err)
	}
	if err := s.SaveOverride(Override{DiffHash: shared, Grouping: sampleResult().Grouping}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	resultPath := filepath.Join(dir, "results", shared[:2], shared+".json")
	overridePath := filepath.Join(dir, "overrides", shared[:2], shared+".json")
	if resultPath == overridePath {
		t.Fatalf("result and override share the path %q", resultPath)
	}
	for _, p := range []string{resultPath, overridePath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("Stat %s: %v", p, err)
		}
	}
	if _, ok, err := s.GetResult(shared); err != nil || !ok {
		t.Errorf("GetResult = (_, %v, %v), want a hit", ok, err)
	}
	if _, ok, err := s.LoadOverride(shared); err != nil || !ok {
		t.Errorf("LoadOverride = (_, %v, %v), want a hit", ok, err)
	}
}

func TestCalibration(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := filepath.Join(dir, "calibration.json")

	if v, ok := s.Calibration("p"); ok || v != 0 {
		t.Fatalf("Calibration on empty store = (%v, %v), want (0, false)", v, ok)
	}

	got, err := s.Calibrate("p", 2100, 1650, 650)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	if !closeTo(got, 2.1) {
		t.Fatalf("first Calibrate = %v, want 2.1", got)
	}
	if v, ok := s.Calibration("p"); !ok || !closeTo(v, 2.1) {
		t.Fatalf("Calibration = (%v, %v), want (2.1, true)", v, ok)
	}

	got, err = s.Calibrate("p", 3000, 1650, 650)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	if want := 0.7*2.1 + 0.3*3.0; !closeTo(got, want) {
		t.Fatalf("second Calibrate = %v, want %v", got, want)
	}

	// Denominator 50 is below the threshold: ignored.
	got, err = s.Calibrate("p", 1000, 700, 650)
	if err != nil {
		t.Fatalf("Calibrate with small denominator: %v", err)
	}
	if want := 0.7*2.1 + 0.3*3.0; !closeTo(got, want) {
		t.Errorf("ignored observation changed the value to %v, want %v", got, want)
	}

	// obs 20 is outside [0.5, 10]: ignored.
	got, err = s.Calibrate("p", 20000, 1650, 650)
	if err != nil {
		t.Fatalf("Calibrate with large observation: %v", err)
	}
	if want := 0.7*2.1 + 0.3*3.0; !closeTo(got, want) {
		t.Errorf("out-of-range observation changed the value to %v, want %v", got, want)
	}

	// Unobserved provider with an ignored observation returns 0.
	if got, err := s.Calibrate("q", 1000, 700, 650); err != nil || got != 0 {
		t.Errorf("Calibrate on a new provider with a bad observation = (%v, %v), want (0, nil)", got, err)
	}

	// Providers are independent.
	if _, err := s.Calibrate("r", 4000, 1650, 650); err != nil {
		t.Fatalf("Calibrate r: %v", err)
	}
	if v, ok := s.Calibration("r"); !ok || !closeTo(v, 4.0) {
		t.Errorf("Calibration(r) = (%v, %v), want (4, true)", v, ok)
	}
	if v, ok := s.Calibration("p"); !ok || !closeTo(v, 0.7*2.1+0.3*3.0) {
		t.Errorf("Calibration(p) after calibrating r = (%v, %v), want 2.37", v, ok)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var obj map[string]float64
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("calibration.json is not a JSON object: %v\n%s", err, data)
	}
	if _, ok := obj["p"]; !ok {
		t.Errorf("calibration.json lacks provider p: %v", obj)
	}
}

func TestConcurrentCalibrate(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const n = 50
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.Calibrate(fmt.Sprintf("p%d", i), 2100, 1650, 650)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("Calibrate p%d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("p%d", i)
		if v, ok := s.Calibration(name); !ok || !closeTo(v, 2.1) {
			t.Errorf("Calibration(%s) = (%v, %v), want (2.1, true)", name, v, ok)
		}
	}
}

func TestConcurrentPutGet(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const n = 20
	keys := make([]string, n)
	for i := range keys {
		keys[i] = Key(fmt.Sprintf("diff-%d", i), "p", "m", 12)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2*n)
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			errs[i] = s.PutResult(sampleEntry(key))
		}(i, key)
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			// A miss before the Put lands is fine; the entry must still be
			// retrievable afterwards.
			if _, _, err := s.GetResult(key); err != nil {
				errs[n+i] = err
			}
		}(i, key)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("operation %d: %v", i, err)
		}
	}
	for _, key := range keys {
		got, ok, err := s.GetResult(key)
		if err != nil {
			t.Fatalf("GetResult %s: %v", key, err)
		}
		if !ok {
			t.Fatalf("GetResult %s missed after PutResult", key)
		}
		if got.Key != key {
			t.Errorf("GetResult %s returned key %q", key, got.Key)
		}
	}
}

// closeTo reports whether a and b are within a float epsilon of each other.
func closeTo(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}
