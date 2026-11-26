package rex

import (
	"os"
	"testing"
	"time"
)

func TestThemeFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want Theme
		ok   bool
	}{
		{"upper case dark", "DARK", ThemeDark, true},
		{"padded light", " light ", ThemeLight, true},
		{"unknown value", "blue", ThemeUnknown, false},
		{"empty", "", ThemeUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLUMP_THEME", tt.env)
			got, ok := ThemeFromEnv(os.Getenv)
			if got != tt.want || ok != tt.ok {
				t.Errorf("ThemeFromEnv(CLUMP_THEME=%q) = (%q, %v), want (%q, %v)", tt.env, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseThemeReply(t *testing.T) {
	const (
		esc  = "\x1b"
		dark = esc + "[?997;1n"
		lit  = esc + "[?997;2n"
		da1  = esc + "[?62;22c"
	)
	tests := []struct {
		name  string
		in    string
		want  Theme
		state ReplyState
	}{
		{"dark then DA1", dark + da1, ThemeDark, ReplyDone},
		{"light then DA1", lit + da1, ThemeLight, ReplyDone},
		{"DA1 only", da1, ThemeUnknown, ReplyDone},
		{"997 without DA1", dark, ThemeDark, ReplyNoise},
		{"partial sequence", esc + "[?99", ThemeUnknown, ReplyIncomplete},
		{"cut off after ESC", "junk" + esc, ThemeUnknown, ReplyIncomplete},
		{"cut off after CSI", "junk" + esc + "[", ThemeUnknown, ReplyIncomplete},
		{"cut off mid-params", esc + "[?997;", ThemeUnknown, ReplyIncomplete},
		{"done wins over a trailing partial", dark + da1 + esc + "[?9", ThemeDark, ReplyDone},
		{
			"garbage, OSC 11 reply, 997, DA1",
			"junk" + esc + "]11;rgb:0d0d/1111/1717" + esc + `\` + dark + da1,
			ThemeDark, ReplyDone,
		},
		{"two reports: the last wins", dark + lit + da1, ThemeLight, ReplyDone},
		{"unknown 997 parameter", esc + "[?997;3n" + da1, ThemeUnknown, ReplyDone},
		{"our own query echoed", ThemeQuery, ThemeUnknown, ReplyNoise},
		{"empty", "", ThemeUnknown, ReplyNoise},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, state := ParseThemeReply([]byte(tt.in))
			if got != tt.want || state != tt.state {
				t.Errorf("ParseThemeReply(%q) = (%q, %v), want (%q, %v)", tt.in, got, state, tt.want, tt.state)
			}
		})
	}

	t.Run("split across two reads", func(t *testing.T) {
		full := []byte(dark + da1)
		for i := 0; i < len(full)-1; i++ {
			got, state := ParseThemeReply(full[:i])
			if state == ReplyDone {
				t.Fatalf("ParseThemeReply(%q) = (%q, %v) for a prefix, want anything but done", full[:i], got, state)
			}
		}
		if got, state := ParseThemeReply(full[:len(dark)-1]); got != ThemeUnknown || state != ReplyIncomplete {
			t.Errorf("ParseThemeReply(prefix %q) = (%q, %v), want (%q, %v)", full[:len(dark)-1], got, state, ThemeUnknown, ReplyIncomplete)
		}
		if got, state := ParseThemeReply(full); got != ThemeDark || state != ReplyDone {
			t.Errorf("ParseThemeReply(full) = (%q, %v), want (%q, %v)", got, state, ThemeDark, ReplyDone)
		}
	})
}

func TestDetectThemeEnvAndNonTTY(t *testing.T) {
	t.Run("env override wins with a nil tty", func(t *testing.T) {
		t.Setenv("CLUMP_THEME", "light")
		if got, err := DetectTheme(nil, os.Getenv, time.Second); got != ThemeLight || err != nil {
			t.Errorf("DetectTheme(nil, CLUMP_THEME=light) = (%q, %v), want (%q, nil)", got, err, ThemeLight)
		}
	})

	t.Run("pipe is not a terminal", func(t *testing.T) {
		t.Setenv("CLUMP_THEME", "")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		defer r.Close()
		defer w.Close()

		start := time.Now()
		got, err := DetectTheme(r, os.Getenv, time.Second)
		elapsed := time.Since(start)
		if got != ThemeUnknown {
			t.Errorf("DetectTheme(pipe) = %q, want %q", got, ThemeUnknown)
		}
		if err != nil {
			t.Errorf("DetectTheme(pipe) = %v, want nil: a pipe cannot be queried, which is not a failure", err)
		}
		if elapsed > 200*time.Millisecond {
			t.Errorf("DetectTheme(pipe) took %v, want < 200ms", elapsed)
		}
	})
}

func TestDetectThemeTimeout(t *testing.T) {
	t.Setenv("CLUMP_THEME", "")
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Logf("cannot open /dev/tty (%v), so DetectTheme cannot be exercised on a live terminal", err)
		return
	}
	defer tty.Close()

	fd := int(tty.Fd())
	before, isTTY := termiosSnapshot(fd)
	if !isTTY {
		t.Logf("/dev/tty is not a terminal here, so DetectTheme cannot be exercised on a live terminal")
		return
	}

	start := time.Now()
	got, err := DetectTheme(tty, os.Getenv, 50*time.Millisecond)
	elapsed := time.Since(start)
	if err != nil {
		t.Errorf("DetectTheme(tty) = %v, want the terminal put back the way it was found", err)
	}

	if got != ThemeUnknown && got != ThemeDark && got != ThemeLight {
		t.Errorf("DetectTheme(tty) = %q, want one of %q, %q, %q", got, ThemeUnknown, ThemeDark, ThemeLight)
	}
	if elapsed > time.Second {
		t.Errorf("DetectTheme(tty, 50ms) took %v, want < 1s", elapsed)
	}

	after, isTTY := termiosSnapshot(fd)
	if !isTTY {
		t.Fatalf("/dev/tty stopped being a terminal after DetectTheme")
	}
	if after != before {
		t.Errorf("termios after DetectTheme = %s, want the saved %s", after, before)
	}
}
