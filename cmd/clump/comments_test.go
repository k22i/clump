package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	g := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=clump", "GIT_AUTHOR_EMAIL=clump@example.com",
			"GIT_COMMITTER_NAME=clump", "GIT_COMMITTER_EMAIL=clump@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	g("init", "-q", "-b", "main")
	g("config", "user.email", "clump@example.com")
	g("config", "user.name", "clump")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g("add", ".")
	g("commit", "-q", "-m", "first")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\nTWO\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g("add", ".")
	g("commit", "-q", "-m", "second")
	t.Setenv("CLUMP_CONFIG", filepath.Join(dir, "missing-config.toml"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	return dir
}

func commentRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runComments(args, os.Stdin, &out, &out)
	return out.String(), err
}

func TestCommentsAddListDelete(t *testing.T) {
	dir := gitRepo(t)
	got, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD", "--file", "a.txt", "--line", "2", "name this change")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "added comment") || !strings.Contains(got, "name this change") {
		t.Fatalf("add out: %q", got)
	}
	list, err := commentRun(t, "--repo", dir, "--rev", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "1. `a.txt:2` (new side) — name this change") {
		t.Fatalf("list:\n%s", list)
	}
	var buf bytes.Buffer
	if err := runComments([]string{"add", "--repo", dir, "--rev", "HEAD", "--file", "a.txt", "--json",
		"json comment"}, os.Stdin, &buf, &buf); err != nil {
		t.Fatal(err)
	}
	var added struct {
		Comment struct {
			ID string `json:"id"`
		} `json:"comment"`
	}
	if err := json.Unmarshal(buf.Bytes(), &added); err != nil {
		t.Fatalf("json: %v: %s", err, buf.String())
	}
	if added.Comment.ID == "" {
		t.Fatalf("no id: %s", buf.String())
	}
	list2, err := commentRun(t, "--repo", dir, "--rev", "HEAD", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list2, `"id": "`+added.Comment.ID+`"`) ||
		!strings.Contains(list2, `"kind": "line"`) || !strings.Contains(list2, `"diff_hash"`) {
		t.Fatalf("json list:\n%s", list2)
	}
	if _, err := commentRun(t, "delete", "--id", added.Comment.ID); err != nil {
		t.Fatal(err)
	}
	list3, err := commentRun(t, "--repo", dir, "--rev", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list3, "1. `a.txt:2` (new side) — name this change") {
		t.Fatalf("after delete:\n%s", list3)
	}
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD", "--file", "a.txt", "--line", "2",
		"--side", "old", "two was fine"); err != nil {
		t.Fatal(err)
	}
	list4, _ := commentRun(t, "--repo", dir, "--rev", "HEAD")
	if !strings.Contains(list4, "`a.txt:2` (old side) — two was fine") {
		t.Fatalf("old side:\n%s", list4)
	}
}

func TestCommentsWholeFileAndErrors(t *testing.T) {
	dir := gitRepo(t)
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD", "--file", "a.txt", "needs tests"); err != nil {
		t.Fatal(err)
	}
	list, err := commentRun(t, "--repo", dir, "--rev", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "1. `a.txt` (file) — needs tests") {
		t.Fatalf("file comment:\n%s", list)
	}
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD", "--file", "nope.txt", "x"); err == nil {
		t.Fatal("unknown file should fail")
	}
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD", "--file", "a.txt", "--line", "9", "x"); err == nil {
		t.Fatal("unknown line should fail")
	}
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD", "--group", "g1", "x"); err == nil ||
		!strings.Contains(err.Error(), "no saved grouping") {
		t.Fatalf("group without grouping: %v", err)
	}
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD~1", "--file", "a.txt", "--line", "1",
		"first line"); err != nil {
		t.Fatal(err)
	}
	if _, err := commentRun(t, "add", "--repo", dir, "--rev", "HEAD~1", "--file", "a.txt", "--line", "1",
		"--side", "old", "no old side in a new file"); err == nil {
		t.Fatal("old side of a new file should not exist")
	}
}
