package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease points this package's update source at a fake GitHub: its
// latest release is tag, and this platform's archive holds bin.
func fakeRelease(t *testing.T, tag string, bin []byte) {
	t.Helper()
	asset := "clump_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"

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
	archive := buf.Bytes()
	sum := sha256.Sum256(archive)

	// The fake repo name is fixed: handlers are registered before
	// updateRepo names it.
	const fakeRepo = "k22i/clump-fake"
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+fakeRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		body := struct {
			TagName string `json:"tag_name"`
			Assets  []struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			} `json:"assets"`
		}{TagName: tag}
		for _, name := range []string{asset, "checksums.txt"} {
			body.Assets = append(body.Assets, struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			}{name, "http://" + r.Host + "/" + name})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prevAPI, prevRepo, prevExe, prevVersion := updateAPI, updateRepo, selfExe, version
	t.Setenv("GITHUB_TOKEN", "") // keep the API call anonymous
	updateAPI, updateRepo = srv.URL, fakeRepo
	t.Cleanup(func() { updateAPI, updateRepo, selfExe, version = prevAPI, prevRepo, prevExe, prevVersion })
}

// selfExePath returns a clump stand-in for the update to replace.
func selfExePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clump")
	if err := os.WriteFile(path, []byte("old clump"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunUpdate(t *testing.T) {
	exe := selfExePath(t)
	selfExe = func() (string, error) { return exe, nil }

	// A newer release replaces the binary, checksummed.
	version = "v0.1.0"
	fakeRelease(t, "v0.2.0", []byte("new clump"))
	var out, errOut strings.Builder
	if err := runUpdate(nil, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new clump" {
		t.Errorf("binary: got %q, want new clump", got)
	}
	if !strings.Contains(out.String(), "v0.1.0 → v0.2.0") {
		t.Errorf("output: got %q", out.String())
	}

	// The latest release leaves an up-to-date binary alone.
	selfExe = func() (string, error) { return exe, nil }
	version = "v0.2.0"
	out.Reset()
	before, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := runUpdate(nil, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(exe); !bytes.Equal(before, after) {
		t.Error("up to date: the binary was replaced")
	}
	if !strings.Contains(out.String(), "is up to date") {
		t.Errorf("output: got %q", out.String())
	}

	// A dev build installs the latest release over itself.
	selfExe = func() (string, error) { return exe, nil }
	version = "dev"
	out.Reset()
	if err := runUpdate(nil, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new clump" {
		t.Errorf("dev: binary: got %q", got)
	}
	if !strings.Contains(out.String(), "dev → v0.2.0") {
		t.Errorf("dev: output: got %q", out.String())
	}

	// Arguments are a usage error.
	if err := runUpdate([]string{"--now"}, &out, io.Discard); err == nil {
		t.Error("args: want a usage error")
	}
}

// TestRunUpdateUnavailable keeps the messages short when GitHub is dark.
func TestRunUpdateUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	prevAPI, prevVersion := updateAPI, version
	updateAPI, version = srv.URL, "v0.1.0"
	defer func() { updateAPI, version = prevAPI, prevVersion }()
	if err := runUpdate(nil, io.Discard, io.Discard); err == nil {
		t.Error("404: want an error naming the missing release")
	}
}
