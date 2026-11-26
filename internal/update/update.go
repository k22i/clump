package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const Repo = "k22i/clump"

const API = "https://api.github.com"

const checksumsAsset = "checksums.txt"

const maxArchive = 64 << 20

type Release struct {
	Tag    string
	Assets map[string]string
}

func AssetName(goos, goarch string) string {
	return "clump_" + goos + "_" + goarch + ".tar.gz"
}

func Latest(ctx context.Context, client *http.Client, apiURL, repo string) (Release, error) {
	url := strings.TrimSuffix(apiURL, "/") + "/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Release{}, fmt.Errorf("no release of %s (publish one first: git tag vX.Y.Z)", repo)
	case resp.StatusCode != http.StatusOK:
		return Release{}, fmt.Errorf("github: %s: %s", url, resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxArchive)).Decode(&body); err != nil {
		return Release{}, err
	}
	rel := Release{Tag: body.TagName, Assets: map[string]string{}}
	for _, a := range body.Assets {
		rel.Assets[a.Name] = a.BrowserDownloadURL
	}
	if rel.Tag == "" {
		return Release{}, errors.New("release of " + repo + " has no tag")
	}
	return rel, nil
}

func Newer(current, tag string) bool {
	cur, curPre := parseVersion(current)
	if cur == nil {
		return true
	}
	latest, latestPre := parseVersion(tag)
	if latest == nil {
		return false
	}
	for i := range cur {
		if latest[i] != cur[i] {
			return latest[i] > cur[i]
		}
	}
	return curPre && !latestPre
}

func parseVersion(s string) ([]int, bool) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	pre := false
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		pre = s[i] == '-'
		s = s[:i]
	}
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		nums[i] = n
	}
	return nums, pre
}

func Download(ctx context.Context, client *http.Client, rel Release, goos, goarch string) ([]byte, error) {
	name := AssetName(goos, goarch)
	archiveURL, ok := rel.Assets[name]
	if !ok {
		return nil, fmt.Errorf("release %s has no %s (assets: %s)", rel.Tag, name, strings.Join(keys(rel.Assets), ", "))
	}
	checksumsURL, ok := rel.Assets[checksumsAsset]
	if !ok {
		return nil, fmt.Errorf("release %s has no %s", rel.Tag, checksumsAsset)
	}
	archive, err := get(ctx, client, archiveURL)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", name, err)
	}
	checksums, err := get(ctx, client, checksumsURL)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", checksumsAsset, err)
	}
	want, err := checksum(checksums, name)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != want {
		return nil, fmt.Errorf("%s: checksum mismatch: got %s, want %s", name, hex.EncodeToString(sum[:]), want)
	}
	return untar(archive)
}

func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxArchive))
}

func checksum(checksums []byte, name string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%s lists no checksum for %s", checksumsAsset, name)
}

func untar(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("archive holds no clump binary")
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "clump" {
			continue
		}
		return io.ReadAll(tr)
	}
}

func Install(path string, bin []byte) error {
	mode := os.FileMode(0o755)
	if old, err := os.Stat(path); err == nil {
		mode = old.Mode()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".clump-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func keys(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
