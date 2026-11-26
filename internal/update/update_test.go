package update_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k22i/clump/internal/update"
)

func buildArchive(t *testing.T, bin []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "clump", Mode: 0o755, Size: int64(len(bin))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(bin); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fakeRelease(t *testing.T, tag, goos, goarch string, bin []byte) (*httptest.Server, string, update.Release) {
	t.Helper()
	mux := http.NewServeMux()
	archive := buildArchive(t, bin)
	sum := sha256.Sum256(archive)
	rel := update.Release{
		Tag: tag,
		Assets: map[string]string{
			update.AssetName(goos, goarch): "",
			"checksums.txt":                "",
		},
	}
	mux.HandleFunc("/repos/k22i/clump/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		body := struct {
			TagName string `json:"tag_name"`
			Assets  []struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			} `json:"assets"`
		}{TagName: tag}
		for name := range rel.Assets {
			body.Assets = append(body.Assets, struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			}{name, "http://" + r.Host + "/" + name})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/"+update.AssetName(goos, goarch), func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(hex.EncodeToString(sum[:]) + "  " + update.AssetName(goos, goarch) + "\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	for name := range rel.Assets {
		rel.Assets[name] = srv.URL + "/" + name
	}
	return srv, srv.URL, rel
}

func TestLatest(t *testing.T) {
	_, api, _ := fakeRelease(t, "v0.1.0", "darwin", "arm64", []byte("bin"))
	rel, err := update.Latest(t.Context(), http.DefaultClient, api, "k22i/clump")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.1.0" {
		t.Errorf("tag: got %q, want v0.1.0", rel.Tag)
	}
	if _, ok := rel.Assets["clump_darwin_arm64.tar.gz"]; !ok {
		t.Errorf("assets: clump_darwin_arm64.tar.gz missing from %v", rel.Assets)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := update.Latest(t.Context(), http.DefaultClient, srv.URL, "k22i/clump"); err == nil {
		t.Error("404: want an error")
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		current, tag string
		want         bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v1.2.3", "v1.2.4", true},
		{"v1.10.0", "v1.9.0", false},
		{"v0.2.0", "v0.1.0", false},
		{"dev", "v0.0.1", true},
		{"v0.1.0", "garbage", false},
		{"v0.1.0-rc1", "v0.1.0", true},
	} {
		if got := update.Newer(tc.current, tc.tag); got != tc.want {
			t.Errorf("Newer(%q, %q): got %v, want %v", tc.current, tc.tag, got, tc.want)
		}
	}
}

func TestDownload(t *testing.T) {
	_, api, want := fakeRelease(t, "v0.1.0", "darwin", "arm64", []byte("the binary"))
	rel, err := update.Latest(t.Context(), http.DefaultClient, api, "k22i/clump")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != want.Tag {
		t.Errorf("tag: got %q, want %q", rel.Tag, want.Tag)
	}
	bin, err := update.Download(t.Context(), http.DefaultClient, rel, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if string(bin) != "the binary" {
		t.Errorf("binary: got %q", bin)
	}

	bad := update.Release{Tag: rel.Tag, Assets: map[string]string{}}
	for k, v := range rel.Assets {
		bad.Assets[k] = v
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  clump_darwin_arm64.tar.gz\n"))
			return
		}
		w.Write([]byte("the binary"))
	}))
	defer srv.Close()
	bad.Assets["checksums.txt"] = srv.URL + "/checksums.txt"
	if _, err := update.Download(t.Context(), http.DefaultClient, bad, "darwin", "arm64"); err == nil {
		t.Error("checksum mismatch: want an error")
	}

	if _, err := update.Download(t.Context(), http.DefaultClient, rel, "windows", "386"); err == nil {
		t.Error("missing asset: want an error")
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clump")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := update.Install(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("binary: got %q, want new", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode()&0o111 == 0 {
		t.Errorf("mode: %v, %v", info.Mode(), err)
	}

	if err := update.Install(filepath.Join(dir, "no", "such", "clump"), []byte("new")); err == nil {
		t.Error("missing dir: want an error")
	}
}
