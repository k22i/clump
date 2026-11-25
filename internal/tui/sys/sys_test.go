package sys

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/k22i/clump/internal/gitdiff"
)

func TestClipboardArgv(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	lookPath := func(found ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, f := range found {
				if f == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		}
	}

	tests := []struct {
		name string
		goos string
		env  map[string]string
		have []string
		want []string
	}{
		{
			name: "darwin with pbcopy",
			goos: "darwin",
			have: []string{"pbcopy"},
			want: []string{"pbcopy"},
		},
		{
			name: "darwin without pbcopy and no display",
			goos: "darwin",
			have: nil,
			want: nil,
		},
		{
			name: "linux wayland with wl-copy",
			goos: "linux",
			env:  map[string]string{"WAYLAND_DISPLAY": "wayland-0"},
			have: []string{"wl-copy"},
			want: []string{"wl-copy"},
		},
		{
			name: "linux wayland without wl-copy falls back to xclip",
			goos: "linux",
			env:  map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"},
			have: []string{"xclip"},
			want: []string{"xclip", "-selection", "clipboard"},
		},
		{
			name: "display with only xsel",
			goos: "linux",
			env:  map[string]string{"DISPLAY": ":0"},
			have: []string{"xsel"},
			want: []string{"xsel", "--clipboard", "--input"},
		},
		{
			name: "nothing available",
			goos: "linux",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClipboardArgv(tc.goos, env(tc.env), lookPath(tc.have...))
			if len(got) != len(tc.want) {
				t.Fatalf("ClipboardArgv() = %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ClipboardArgv() = %q, want %q", got, tc.want)
				}
			}
		})
	}
}

func TestRunCopy(t *testing.T) {
	t.Run("reads stdin", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "clip.txt")
		msg := runCopy([]string{"sh", "-c", "cat > " + file}, "héllo\n")
		if msg.Err != nil {
			t.Fatalf("runCopy() error = %v", msg.Err)
		}
		if msg.Via != "osc52+sh" {
			t.Errorf("Via = %q, want %q", msg.Via, "osc52+sh")
		}
		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if string(got) != "héllo\n" {
			t.Errorf("clipboard file = %q, want %q", got, "héllo\n")
		}
	})

	t.Run("failing tool", func(t *testing.T) {
		msg := runCopy([]string{"sh", "-c", "exit 3"}, "x")
		if msg.Err == nil {
			t.Fatal("runCopy() error = nil, want non-nil")
		}
		if msg.Via != "osc52" {
			t.Errorf("Via = %q, want %q", msg.Via, "osc52")
		}
	})

	t.Run("no tool", func(t *testing.T) {
		msg := runCopy(nil, "x")
		if msg.Via != "osc52" || msg.Err != nil {
			t.Errorf("runCopy(nil) = %+v, want {Via: osc52, Err: nil}", msg)
		}
	})
}

func TestRunCopyTimeout(t *testing.T) {
	start := time.Now()
	msg := runCopy([]string{"sh", "-c", "sleep 10"}, "x")
	elapsed := time.Since(start)
	if msg.Err == nil {
		t.Fatal("runCopy() error = nil, want non-nil on timeout")
	}
	if elapsed > 5*time.Second {
		t.Errorf("runCopy() took %v, want under 5s", elapsed)
	}
}

func TestCopyCmdNotNil(t *testing.T) {
	cmd := CopyCmd("x")
	if cmd == nil {
		t.Fatal("CopyCmd() = nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("CopyCmd() message = %T, want tea.BatchMsg", msg)
	}
	if len(batch) != 2 {
		t.Fatalf("len(BatchMsg) = %d, want 2", len(batch))
	}
}

func TestEditorFromEnv(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "VISUAL wins", env: map[string]string{"VISUAL": "nvim", "EDITOR": "nano"}, want: "nvim"},
		{name: "only EDITOR", env: map[string]string{"EDITOR": "nano"}, want: "nano"},
		{name: "whitespace VISUAL is unset", env: map[string]string{"VISUAL": "  ", "EDITOR": "emacs -nw"}, want: "emacs -nw"},
		{name: "neither", env: nil, want: "vi"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EditorFromEnv(env(tc.env)); got != tc.want {
				t.Errorf("EditorFromEnv() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEditorArgv(t *testing.T) {
	tests := []struct {
		name   string
		editor string
		line   int
		want   []string
		err    bool
	}{
		{name: "vim", editor: "vim", line: 12, want: []string{"vim", "+12", "a.go"}},
		{name: "nvim without line", editor: "nvim", line: 0, want: []string{"nvim", "a.go"}},
		{name: "code --wait", editor: "code --wait", line: 12, want: []string{"code", "--wait", "--goto", "a.go:12"}},
		{name: "code by path", editor: "/usr/local/bin/code", line: 12, want: []string{"/usr/local/bin/code", "--goto", "a.go:12"}},
		{name: "subl", editor: "subl", line: 12, want: []string{"subl", "a.go:12"}},
		{name: "emacsclient -t", editor: "emacsclient -t", line: 12, want: []string{"emacsclient", "-t", "+12", "a.go"}},
		{name: "quoted editor", editor: `"my editor" -f`, line: 12, want: []string{"my editor", "-f", "+12", "a.go"}},
		{name: "escaped quote", editor: `'it'\''s'`, line: 12, want: []string{"it's", "+12", "a.go"}},
		{name: "unterminated quote", editor: `vim "unterminated`, line: 12, err: true},
		{name: "blank editor", editor: "   ", line: 12, err: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EditorArgv(tc.editor, "a.go", tc.line)
			if tc.err {
				if err == nil {
					t.Fatalf("EditorArgv(%q) error = nil, want non-nil", tc.editor)
				}
				return
			}
			if err != nil {
				t.Fatalf("EditorArgv(%q) error = %v", tc.editor, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("EditorArgv(%q) = %q, want %q", tc.editor, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("EditorArgv(%q) = %q, want %q", tc.editor, got, tc.want)
				}
			}
		})
	}
}

func TestHunkTarget(t *testing.T) {
	file := &gitdiff.File{OldPath: "a.go", NewPath: "a.go", Status: gitdiff.Modified}
	deleted := &gitdiff.File{OldPath: "gone.go", NewPath: "", Status: gitdiff.Deleted}

	tests := []struct {
		name     string
		hunk     *gitdiff.Hunk
		wantPath string
		wantLine int
		wantOK   bool
	}{
		{
			name: "first added line",
			hunk: &gitdiff.Hunk{
				Header: "@@ -1,3 +5,4 @@", File: file,
				OldStart: 1, OldLines: 3, NewStart: 5, NewLines: 4,
				Lines: []string{" a", "-b", "+c"},
			},
			wantPath: "a.go", wantLine: 6, wantOK: true,
		},
		{
			name: "only context and deletes",
			hunk: &gitdiff.Hunk{
				Header: "@@ -1,3 +5,2 @@", File: file,
				OldStart: 1, OldLines: 3, NewStart: 5, NewLines: 2,
				Lines: []string{"-x", " a", "-b", " c2"},
			},
			wantPath: "a.go", wantLine: 5, wantOK: true,
		},
		{
			name: "no body",
			hunk: &gitdiff.Hunk{
				Header: "@@ -1,3 +7,0 @@", File: file,
				OldStart: 1, OldLines: 3, NewStart: 7,
			},
			wantPath: "a.go", wantLine: 7, wantOK: true,
		},
		{
			name: "new file starts at zero",
			hunk: &gitdiff.Hunk{
				Header: "@@ -0,0 +1 @@", File: &gitdiff.File{NewPath: "new.go", Status: gitdiff.Added},
				NewStart: 1, NewLines: 1,
				Lines: []string{"+x"},
			},
			wantPath: "new.go", wantLine: 1, wantOK: true,
		},
		{
			name: "pseudo hunk",
			hunk: &gitdiff.Hunk{
				Header: "(binary new file)", File: &gitdiff.File{NewPath: "img.png", Status: gitdiff.Added},
				Pseudo: true,
			},
			wantOK: false,
		},
		{
			name: "deleted file",
			hunk: &gitdiff.Hunk{
				Header: "@@ -1,2 +0,0 @@", File: deleted,
				OldStart: 1, OldLines: 2, NewStart: 0,
				Lines: []string{"-a", "-b"},
			},
			wantOK: false,
		},
		{
			name: "renamed file uses new path",
			hunk: &gitdiff.Hunk{
				Header: "@@ -1,1 +1,2 @@", File: &gitdiff.File{OldPath: "old.go", NewPath: "new.go", Status: gitdiff.Renamed},
				OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 2,
			},
			wantPath: "new.go", wantLine: 1, wantOK: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, line, ok := HunkTarget(tc.hunk)
			if ok != tc.wantOK {
				t.Fatalf("HunkTarget() ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if path != tc.wantPath {
				t.Errorf("HunkTarget() path = %q, want %q", path, tc.wantPath)
			}
			if line != tc.wantLine {
				t.Errorf("HunkTarget() line = %d, want %d", line, tc.wantLine)
			}
		})
	}
}

func TestEditCmd(t *testing.T) {
	t.Run("empty argv", func(t *testing.T) {
		cmd := EditCmd(nil, "")
		if cmd == nil {
			t.Fatal("EditCmd(nil, \"\") = nil")
		}
		msg, ok := cmd().(EditedMsg)
		if !ok {
			t.Fatalf("EditCmd(nil, \"\") message = %T, want EditedMsg", msg)
		}
		if msg.Err == nil {
			t.Fatal("EditCmd(nil, \"\").Err = nil, want non-nil")
		}
		if want := "editor: empty command"; msg.Err.Error() != want {
			t.Errorf("EditCmd(nil, \"\").Err = %q, want %q", msg.Err, want)
		}
	})

	t.Run("real command", func(t *testing.T) {
		if cmd := EditCmd([]string{"true"}, t.TempDir()); cmd == nil {
			t.Fatal("EditCmd([true], dir) = nil")
		}
	})
}
