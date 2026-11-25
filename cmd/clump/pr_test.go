package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stubGh answers clump's gh calls from the gitdiff package's fixture files.
// Every invocation appends "<cwd> <args>" to $CLUMP_GH_LOG.
func stubGh(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	fx, err := filepath.Abs("../../internal/gitdiff/testdata")
	if err != nil {
		t.Fatal(err)
	}
	sh := `#!/bin/sh
printf '%s %s\n' "$(pwd)" "$*" >> "$CLUMP_GH_LOG"
case "$2" in
diff) cat "$CLUMP_GH_DIFF" ;;
view) cat "$CLUMP_GH_VIEW" ;;
*) echo "gh: unexpected subcommand $2" >&2; exit 9 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLUMP_GH_LOG", filepath.Join(dir, "gh.log"))
	t.Setenv("CLUMP_GH_DIFF", filepath.Join(fx, "github/pr-42.patch"))
	t.Setenv("CLUMP_GH_VIEW", filepath.Join(fx, "github/pr-42.json"))
	return dir
}

func TestParseOneShotPR(t *testing.T) {
	o, err := parseOneShot([]string{"--plain", "--pr", "k22i/clump#42"})
	if err != nil {
		t.Fatalf("parseOneShot: %v", err)
	}
	if o.pr != "k22i/clump#42" || o.plain != true || len(o.args) != 0 {
		t.Errorf("opts = %+v", o)
	}
}

func TestParseOneShotPRErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--pr", "42", "main"}, want: "--pr takes no revision"},
		{args: []string{"--pr", "42", "--branch"}, want: "--branch and --pr are exclusive"},
		{args: []string{"--pr", "42", "--pick"}, want: "--pick and --pr are exclusive"},
		{args: []string{"--pr", "nope"}, want: "is not a pull request"},
	}
	for _, tt := range tests {
		o, err := parseOneShot(tt.args)
		if err == nil {
			t.Errorf("parseOneShot(%q) = %+v, want an error", tt.args, o)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("parseOneShot(%q) err = %q, want %q", tt.args, err, tt.want)
		}
	}
}

func TestLoadInputPR(t *testing.T) {
	stubGh(t)
	o, err := parseOneShot([]string{"--pr", "#42"})
	if err != nil {
		t.Fatalf("parseOneShot: %v", err)
	}
	ctx := context.Background()
	l, repo, err := loadInput(ctx, o, os.Stdin)
	if err != nil {
		t.Fatalf("loadInput: %v", err)
	}
	if repo == "" {
		t.Error("loadInput returned no repo root for a number-only --pr")
	}
	if l.Title != "#42 Add the retry policy" || l.Ref != "pr/42" {
		t.Errorf("loaded = Title %q Ref %q", l.Title, l.Ref)
	}
}

// TestRunPRBin runs the real binary end to end against a stub gh: --dry-run
// sends nothing, so no provider is needed, yet every step that would run
// before a real call does: flag parsing, gh loading, generated marking,
// prompt preparation.
func TestRunPRBin(t *testing.T) {
	ghDir := stubGh(t)
	bin := clumpBin(t)
	home := t.TempDir()                         // not a git clone: the URL spec needs none
	cfgPath := filepath.Join(home, "none.toml") // absent: the defaults apply
	buf := &bytes.Buffer{}
	cmd := exec.Command(bin, "--dry-run", "--pr", "https://github.com/k22i/clump/pull/42")
	cmd.Dir = home
	cmd.Stdout, cmd.Stderr = buf, buf
	cmd.Env = os.Environ()
	setEnv := func(k, v string) {
		for i, e := range cmd.Env {
			if strings.HasPrefix(e, k+"=") {
				cmd.Env[i] = k + "=" + v
				return
			}
		}
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// The stub's CLUMP_GH_* control comes through the inherited environment;
	// HOME and the config overrides must be replaced in place, not dropped.
	setEnv("PATH", ghDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	setEnv("HOME", home)
	setEnv("CLUMP_CONFIG", cfgPath)
	setEnv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	setEnv("XDG_RUNTIME_DIR", filepath.Join(home, "run"))
	if err := cmd.Run(); err != nil {
		t.Fatalf("clump --dry-run --pr: %v\n%s", err, buf.String())
	}
	for _, want := range []string{"PULL REQUEST: Add the retry policy", "retry.go", "Fixes #7."} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output misses %q:\n%s", want, buf.String())
		}
	}
}
