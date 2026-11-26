package lazygit

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestPushCommand(t *testing.T) {
	tests := []struct {
		name string
		then string
		want string
	}{
		{"none", "", "clump push"},
		{"plain", "delta --dark --paging=never", "clump push --then 'delta --dark --paging=never'"},
		{"one quote", "sh -c 'x'", `clump push --then 'sh -c '\''x'\'''`},
		{"two quotes", "a 'b' c", `clump push --then 'a '\''b'\'' c'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PushCommand(tt.then); got != tt.want {
				t.Fatalf("PushCommand(%q) = %q; want %q", tt.then, got, tt.want)
			}
		})
	}
}

func TestSnippet(t *testing.T) {
	tests := []struct {
		name string
		then string
		want string
	}{
		{
			name: "none",
			then: "",
			want: "git:\n" +
				"  diffRenderers:\n" +
				"    - command: \"clump push\"\n" +
				"      name: clump\n",
		},
		{
			name: "wrapping delta",
			then: "delta --dark",
			want: "git:\n" +
				"  diffRenderers:\n" +
				"    - command: \"clump push --then 'delta --dark'\"\n" +
				"      name: clump\n",
		},
		{
			name: "escapes a double quote",
			then: `delta --file-link-format="{path}:{line}"`,
			want: "git:\n" +
				"  diffRenderers:\n" +
				`    - command: "clump push --then 'delta --file-link-format=\"{path}:{line}\"'"` + "\n" +
				"      name: clump\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Snippet(tt.then); got != tt.want {
				t.Fatalf("Snippet(%q) = %q; want %q", tt.then, got, tt.want)
			}
		})
	}
}

func TestConfigPath(t *testing.T) {
	t.Run("LG_CONFIG_FILE first entry", func(t *testing.T) {
		t.Setenv("LG_CONFIG_FILE", "/etc/lazygit/a.yml,/etc/lazygit/b.yml")
		got, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath: %v", err)
		}
		if want := "/etc/lazygit/a.yml"; got != want {
			t.Fatalf("ConfigPath() = %q; want %q", got, want)
		}
	})
	t.Run("LG_CONFIG_FILE single", func(t *testing.T) {
		t.Setenv("LG_CONFIG_FILE", "config.yml")
		got, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath: %v", err)
		}
		if want := "config.yml"; got != want {
			t.Fatalf("ConfigPath() = %q; want %q", got, want)
		}
	})
	t.Run("XDG_CONFIG_HOME absolute", func(t *testing.T) {
		xdg := t.TempDir()
		t.Setenv("LG_CONFIG_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", xdg)
		got, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath: %v", err)
		}
		if want := filepath.Join(xdg, "lazygit", "config.yml"); got != want {
			t.Fatalf("ConfigPath() = %q; want %q", got, want)
		}
	})
	t.Run("XDG_CONFIG_HOME relative ignored", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("LG_CONFIG_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "relative/dir")
		got, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath: %v", err)
		}
		if want := filepath.Join(home, ".config", "lazygit", "config.yml"); got != want {
			t.Fatalf("ConfigPath() = %q; want %q", got, want)
		}
	})
	t.Run("home fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("LG_CONFIG_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		got, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath: %v", err)
		}
		if want := filepath.Join(home, ".config", "lazygit", "config.yml"); got != want {
			t.Fatalf("ConfigPath() = %q; want %q", got, want)
		}
	})
	t.Run("no library file on darwin", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("LG_CONFIG_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		got, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath: %v", err)
		}
		if want := filepath.Join(home, ".config", "lazygit", "config.yml"); got != want {
			t.Fatalf("ConfigPath() = %q; want %q", got, want)
		}
	})
	t.Run("library file on darwin", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("LG_CONFIG_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		lib := filepath.Join(home, "Library", "Application Support", "lazygit")
		if err := os.MkdirAll(lib, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		libConfig := filepath.Join(lib, "config.yml")
		if err := os.WriteFile(libConfig, []byte("gui: {}\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath: %v", err)
		}
		want := filepath.Join(home, ".config", "lazygit", "config.yml")
		if runtime.GOOS == "darwin" {
			want = libConfig
		}
		if got != want {
			t.Fatalf("ConfigPath() = %q; want %q", got, want)
		}
	})
}

func TestInstallShellQuoting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	in := golden(t, "install-quoted-in.yml")
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	var out bytes.Buffer
	opts := InstallOpts{
		Path:  path,
		Write: true,
		In:    strings.NewReader("y\n"),
		Out:   &out,
	}
	if err := Install(opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := readFile(t, path)
	if want := golden(t, "install-quoted-want.yml"); got != want {
		t.Fatalf("config is\n%q\nwant\n%q", got, want)
	}
	checkCommandEscapes(t, path, got)
	line := commandLineOf(t, got)
	start, end := strings.Index(line, `"`), strings.LastIndex(line, `"`)
	if start < 0 || end <= start {
		t.Fatalf("command value in %q is not a YAML double-quoted scalar", line)
	}
	raw := line[start : end+1]
	words := shellWords(t, unescapeYAML(raw))
	if len(words) != 4 {
		t.Fatalf("command %q parsed into %d words %q; want 4", raw, len(words), words)
	}
	if want := []string{"clump", "push", "--then", "sh -c 'x'"}; !equalStrings(words, want) {
		t.Fatalf("command words = %q; want %q", words, want)
	}
}

func TestInstall(t *testing.T) {
	snip := Snippet("")
	tests := []struct {
		name   string
		in     string
		want   string
		write  bool
		answer string
		prompt bool
		out    string
		mode   fs.FileMode
		backup bool
	}{
		{
			name:   "missing file",
			want:   "install-missing-want.yml",
			write:  true,
			answer: "y\n",
			prompt: true,
			out: "lazygit config %s does not exist; it will contain:\n" + snip +
				"Write this to %s? [y/N] \n" +
				"wrote %s\n",
		},
		{
			name:   "empty file",
			in:     "install-empty-in.yml",
			want:   "install-empty-want.yml",
			write:  true,
			answer: "y\n",
			prompt: true,
			out: "lazygit config %s does not exist; it will contain:\n" + snip +
				"Write this to %s? [y/N] \n" +
				"wrote %s (backup: %s.clump-backup-TS)\n",
			backup: true,
		},
		{
			name: "no git section",
			in:   "install-no-git-in.yml",
			out:  "add this to %s:\n" + snip,
		},
		{
			name: "git without diffRenderers",
			in:   "install-git-block-in.yml",
			want: "install-git-block-want.yml",
			out: "insert under git: in %s:\n" +
				"  diffRenderers:\n" +
				"    - command: \"clump push\"\n" +
				"      name: clump\n",
		},
		{
			name: "git with 4 space indent",
			in:   "install-indent4-in.yml",
			want: "install-indent4-want.yml",
			out: "insert under git: in %s:\n" +
				"    diffRenderers:\n" +
				"      - command: \"clump push\"\n" +
				"        name: clump\n",
		},
		{
			name: "existing delta, print only",
			in:   "install-delta-in.yml",
			want: "install-delta-want.yml",
			out: "change the diff renderer in %s:\n" +
				"-    - command: delta --dark --paging=never\n" +
				"+    - command: \"clump push --then 'delta --dark --paging=never'\"\n",
		},
		{
			name: "existing quoted command",
			in:   "install-quoted-in.yml",
			want: "install-quoted-want.yml",
			out: "change the diff renderer in %s:\n" +
				"-    - command: \"sh -c 'x'\"\n" +
				`+    - command: "clump push --then 'sh -c '\\''x'\\'''"` + "\n",
		},
		{
			name: "first item is extDiff",
			in:   "install-extdiff-in.yml",
			want: "install-extdiff-want.yml",
			out: "change the diff renderer in %s:\n" +
				"+    - command: \"clump push\"\n" +
				"+      name: clump\n",
		},
		{
			name:  "already installed",
			in:    "install-already-in.yml",
			write: true,
			out:   "clump push is already configured in %s\n",
		},
		{
			name:   "refuse inline git",
			in:     "install-inline-git-in.yml",
			write:  true,
			answer: "y\n",
			out:    "cannot edit %s automatically: git: has an inline value; add this by hand:\n" + snip,
		},
		{
			name:   "refuse colorArg never",
			in:     "install-colorarg-never-in.yml",
			write:  true,
			answer: "y\n",
			out: "cannot edit %s automatically: the first diff renderer sets colorArg: never; " +
				"add this by hand:\n" + snip,
		},
		{
			name:   "refuse legacy pager",
			in:     "install-legacy-pager-in.yml",
			write:  true,
			answer: "y\n",
			out: "cannot edit %s automatically: the git block sets git.paging.pager; " +
				"add this by hand:\n" + snip,
		},
		{
			name:   "write refused by n",
			in:     "install-no-git-in.yml",
			write:  true,
			answer: "n\n",
			prompt: true,
			out:    "add this to %s:\n" + snip + "Write this to %s? [y/N] \nnot written\n",
		},
		{
			name:   "write with backup keeps the mode",
			in:     "install-delta-in.yml",
			want:   "install-delta-want.yml",
			write:  true,
			answer: "yes\n",
			prompt: true,
			mode:   0o640,
			out: "change the diff renderer in %s:\n" +
				"-    - command: delta --dark --paging=never\n" +
				"+    - command: \"clump push --then 'delta --dark --paging=never'\"\n" +
				"Write this to %s? [y/N] \n" +
				"wrote %s (backup: %s.clump-backup-TS)\n",
			backup: true,
		},
		{
			name: "first item is rawGit",
			in:   "install-rawgit-in.yml",
			want: "install-rawgit-want.yml",
			out: "change the diff renderer in %s:\n" +
				"+    - command: \"clump push\"\n" +
				"+      name: clump\n",
		},
		{
			name: "git with an empty block",
			in:   "install-empty-git-in.yml",
			want: "install-empty-git-want.yml",
			out: "insert under git: in %s:\n" +
				"  diffRenderers:\n" +
				"    - command: \"clump push\"\n" +
				"      name: clump\n",
		},
		{
			name: "no trailing newline",
			in:   "install-no-eol-in.yml",
			want: "install-no-eol-want.yml",
			out:  "add this to %s:\n" + snip,
		},
		{
			name: "explicit stdinFilter is wrapped",
			in:   "install-stdinfilter-in.yml",
			want: "install-stdinfilter-want.yml",
			out: "change the diff renderer in %s:\n" +
				"-      command: delta\n" +
				`+      command: "clump push --then 'delta'"` + "\n",
		},
		{
			name:   "refuse flow style renderers",
			in:     "install-flow-renderers-in.yml",
			write:  true,
			answer: "y\n",
			out: "cannot edit %s automatically: diffRenderers has an inline value; " +
				"add this by hand:\n" + snip,
		},
		{
			name:   "refuse anchors",
			in:     "install-anchor-in.yml",
			write:  true,
			answer: "y\n",
			out: "cannot edit %s automatically: the git block uses a YAML anchor or alias; " +
				"add this by hand:\n" + snip,
		},
		{
			name: "comments and blank lines kept",
			in:   "install-comments-in.yml",
			want: "install-comments-want.yml",
			out: "insert under git: in %s:\n" +
				"  diffRenderers:\n" +
				"    - command: \"clump push\"\n" +
				"      name: clump\n",
		},
	}

	tsRE := regexp.MustCompile(`clump-backup-\d{8}-\d{6}`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yml")
			perm := tt.mode
			if perm == 0 {
				perm = 0o644
			}
			var orig []byte
			if tt.in != "" {
				orig = []byte(golden(t, tt.in))
				if err := os.WriteFile(path, orig, perm); err != nil {
					t.Fatalf("write config: %v", err)
				}
				if err := os.Chmod(path, perm); err != nil {
					t.Fatalf("chmod config: %v", err)
				}
			}

			var in io.Reader = strings.NewReader(tt.answer)
			if !tt.prompt {
				in = failReader{t: t}
			}
			var out bytes.Buffer
			if err := Install(InstallOpts{
				Path:  path,
				Write: tt.write,
				In:    in,
				Out:   &out,
			}); err != nil {
				t.Fatalf("Install: %v", err)
			}

			gotOut := tsRE.ReplaceAllString(out.String(), "clump-backup-TS")
			wantOut := strings.ReplaceAll(tt.out, "%s", path)
			if gotOut != wantOut {
				t.Fatalf("printed\n%q\nwant\n%q", gotOut, wantOut)
			}

			if !tt.write || tt.want == "" {
				after, err := os.ReadFile(path)
				if tt.in == "" {
					if !os.IsNotExist(err) {
						t.Fatalf("config was created: err=%v content=%q", err, after)
					}
				} else if err != nil {
					t.Fatalf("read config: %v", err)
				} else if !bytes.Equal(after, orig) {
					t.Fatalf("config changed to\n%q\nwant\n%q", after, orig)
				}
			} else {
				if got, want := readFile(t, path), golden(t, tt.want); got != want {
					t.Fatalf("config is\n%q\nwant\n%q", got, want)
				}
			}

			if strings.Contains(out.String(), "command:") {
				checkCommandEscapes(t, "printed change", out.String())
			}
			if tt.write && tt.want != "" {
				checkCommandEscapes(t, path, readFile(t, path))
			}

			if tt.mode != 0 {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatalf("stat config: %v", err)
				}
				if got := info.Mode().Perm(); got != tt.mode {
					t.Fatalf("config mode = %v; want %v", got, tt.mode)
				}
			}

			backups, err := filepath.Glob(path + ".clump-backup-*")
			if err != nil {
				t.Fatalf("glob backups: %v", err)
			}
			if !tt.backup {
				if len(backups) != 0 {
					t.Fatalf("backups made: %q", backups)
				}
				return
			}
			if len(backups) != 1 {
				t.Fatalf("backups = %q; want exactly one", backups)
			}
			back, err := os.ReadFile(backups[0])
			if err != nil {
				t.Fatalf("read backup: %v", err)
			}
			if !bytes.Equal(back, orig) {
				t.Fatalf("backup is\n%q\nwant the original\n%q", back, orig)
			}
			info, err := os.Stat(backups[0])
			if err != nil {
				t.Fatalf("stat backup: %v", err)
			}
			if tt.mode != 0 && info.Mode().Perm() != tt.mode {
				t.Fatalf("backup mode = %v; want %v", info.Mode().Perm(), tt.mode)
			}
		})
	}
}

type failReader struct {
	t *testing.T
}

func (r failReader) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Fatal("Install read the y/N confirmation when nothing should be written")
	return 0, io.EOF
}

func golden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(b)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func commandLineOf(t *testing.T, config string) string {
	t.Helper()
	for _, l := range strings.Split(config, "\n") {
		if strings.Contains(l, "command:") {
			return strings.TrimSpace(l)
		}
	}
	t.Fatalf("no command line in\n%s", config)
	return ""
}

func shellWords(t *testing.T, s string) []string {
	t.Helper()
	var words []string
	var cur strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s):
			cur.WriteByte(s[i+1])
			inWord = true
			i++
		case c == '\'' || c == '"':
			end := strings.IndexByte(s[i+1:], c)
			if end < 0 {
				t.Fatalf("unbalanced %q in %q", string(c), s)
			}
			cur.WriteString(s[i+1 : i+1+end])
			inWord = true
			i += 1 + end
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func unescapeYAML(quoted string) string {
	inner := quoted[1 : len(quoted)-1]
	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\\' && i+1 < len(inner) {
			switch inner[i+1] {
			case '\\', '"':
				b.WriteByte(inner[i+1])
				i++
				continue
			}
		}
		b.WriteByte(inner[i])
	}
	return b.String()
}

func checkCommandEscapes(t *testing.T, where, text string) {
	t.Helper()
	checked := 0
	for _, l := range strings.Split(text, "\n") {
		line := strings.TrimSpace(l)
		line = strings.TrimSpace(strings.TrimPrefix(line, "+"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		idx := strings.Index(line, "command:")
		if idx < 0 {
			continue
		}
		raw, _ := splitScalar(strings.TrimSpace(line[idx+len("command:"):]))
		if !strings.HasPrefix(raw, `"`) {
			continue
		}
		if len(raw) < 2 || !strings.HasSuffix(raw, `"`) {
			t.Fatalf("%s: command value %q is not a complete double-quoted scalar", where, raw)
		}
		checked++
		inner := raw[1 : len(raw)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] != '\\' {
				continue
			}
			if i+1 >= len(inner) || (inner[i+1] != '\\' && inner[i+1] != '"') {
				t.Fatalf("%s: command value %q has a backslash that is not followed by \\\\ or \\\": %s",
					where, raw, l)
			}
			i++
		}
	}
	if checked == 0 {
		t.Fatalf("%s: no double-quoted command value to check in %q", where, text)
	}
}
