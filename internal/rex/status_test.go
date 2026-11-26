package rex

import (
	"encoding/base64"
	"math/rand/v2"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestInRex(t *testing.T) {
	tests := []struct {
		name    string
		session string
		block   string
		want    bool
	}{
		{"both set", "session:1", "block:2", true},
		{"no session", "", "block:2", false},
		{"no block", "session:1", "", false},
		{"neither", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REX_SESSION", tt.session)
			t.Setenv("REX_BLOCK", tt.block)
			if got := InRex(os.Getenv); got != tt.want {
				t.Errorf("InRex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeMsg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"newline and tab", "a\nb\tc", "a b c"},
		{"escape sequence", "x\x1b[31my", "x [31my"},
		{"C1 control", "a\u0085b", "a b"},
		{"bidi override", "a\u202eb", "a b"},
		{"invalid UTF-8", "\xff", "\uFFFD"},
		{"surrounding and repeated spaces", "  a   b  ", "a b"},
		{"no change", "Grouping with haiku", "Grouping with haiku"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeMsg(tt.in); got != tt.want {
				t.Errorf("SanitizeMsg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	t.Run("cut on a rune boundary", func(t *testing.T) {
		in := strings.Repeat("é", 2500)
		got := SanitizeMsg(in)
		if len(got) > MaxMsgDecoded {
			t.Fatalf("len(SanitizeMsg(5000 bytes)) = %d, want <= %d", len(got), MaxMsgDecoded)
		}
		if len(got) == 0 {
			t.Fatal("SanitizeMsg dropped everything")
		}
		if !utf8.ValidString(got) {
			t.Fatalf("SanitizeMsg returned invalid UTF-8: %q", got)
		}
		for _, r := range got {
			if r != 'é' {
				t.Fatalf("SanitizeMsg changed a rune to %q", r)
			}
		}
	})
}

func TestEncode(t *testing.T) {
	tests := []struct {
		name string
		in   Status
		want string
	}{
		{
			name: "working with message",
			in:   Status{State: StateWorking, Progress: -1, Msg: "Grouping with haiku"},
			want: "\x1b]7501;state=working:app=clump:msg=R3JvdXBpbmcgd2l0aCBoYWlrdQ==\x1b\\",
		},
		{
			name: "blocked question",
			in:   Status{State: StateBlocked, Kind: KindQuestion, Progress: -1, Msg: "Approve grouping 12 hunks?"},
			want: "\x1b]7501;state=blocked:kind=question:app=clump:msg=QXBwcm92ZSBncm91cGluZyAxMiBodW5rcz8=\x1b\\",
		},
		{
			name: "done without message",
			in:   Status{State: StateDone, Progress: -1},
			want: "\x1b]7501;state=done:app=clump\x1b\\",
		},
		{
			name: "error",
			in:   Status{State: StateError, Progress: -1, Msg: "provider failed: exit 1"},
			want: "\x1b]7501;state=error:app=clump:msg=cHJvdmlkZXIgZmFpbGVkOiBleGl0IDE=\x1b\\",
		},
		{
			name: "progress on working comes before app",
			in:   Status{State: StateWorking, Progress: 40, Msg: "Grouping with haiku"},
			want: "\x1b]7501;state=working:progress=40:app=clump:msg=R3JvdXBpbmcgd2l0aCBoYWlrdQ==\x1b\\",
		},
		{
			name: "progress on done is omitted",
			in:   Status{State: StateDone, Progress: 40},
			want: "\x1b]7501;state=done:app=clump\x1b\\",
		},
		{
			name: "negative progress is omitted",
			in:   Status{State: StateWorking, Progress: -1},
			want: "\x1b]7501;state=working:app=clump\x1b\\",
		},
		{
			name: "progress above 100 is omitted",
			in:   Status{State: StateWorking, Progress: 101},
			want: "\x1b]7501;state=working:app=clump\x1b\\",
		},
		{
			name: "clear",
			in:   Status{State: StateClear},
			want: "\x1b]7501;state=clear\x1b\\",
		},
		{
			name: "kind on a non-blocked state is omitted",
			in:   Status{State: StateWorking, Kind: KindQuestion, Progress: -1},
			want: "\x1b]7501;state=working:app=clump\x1b\\",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Encode(tt.in); got != tt.want {
				t.Errorf("Encode(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func decodeMsg(t *testing.T, seq string) (string, bool) {
	t.Helper()
	body, ok := strings.CutPrefix(seq, "\x1b]7501;")
	if !ok {
		t.Fatalf("sequence %q has no OSC 7501 prefix", seq)
	}
	body, ok = strings.CutSuffix(body, "\x1b\\")
	if !ok {
		t.Fatalf("sequence %q has no ST", seq)
	}
	for _, pair := range strings.Split(body, ":") {
		key, val, ok := strings.Cut(pair, "=")
		if !ok {
			t.Fatalf("pair %q has no =", pair)
		}
		if key != "msg" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			t.Fatalf("msg %q does not decode: %v", val, err)
		}
		return string(raw), true
	}
	return "", false
}

func TestEncodeLimits(t *testing.T) {
	in := strings.Repeat("bad\x1b[0m line\n\twith: separators ", 4000)
	if len(in) < 100*1024 {
		t.Fatalf("test input is %d bytes, want >= 100 KB", len(in))
	}
	seq := Encode(Status{State: StateError, Progress: -1, Msg: in})
	if len(seq) > MaxSequence {
		t.Errorf("len(Encode(100 KB msg)) = %d, want <= %d", len(seq), MaxSequence)
	}
	msg, ok := decodeMsg(t, seq)
	if !ok {
		t.Fatal("the report dropped msg entirely")
	}
	if len(msg) > MaxMsgDecoded {
		t.Errorf("decoded msg is %d bytes, want <= %d", len(msg), MaxMsgDecoded)
	}
	for _, r := range msg {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Errorf("decoded msg contains control rune %U", r)
		}
	}
	if !utf8.ValidString(msg) {
		t.Errorf("decoded msg is not valid UTF-8: %q", msg)
	}
}

func TestEncodeValueCharset(t *testing.T) {
	valueRE := regexp.MustCompile(`^[A-Za-z0-9_.,+/=-]*$`)
	states := []State{StateIdle, StateWorking, StateDone, StateBlocked, StateError, StateClear}
	rng := rand.New(rand.NewPCG(1, 2))
	const pool = "abc XYZ 0123:_=;.,/+-*\n\t\r\x00\x1b\x7f\u0085\u009f\u202e\u200f\u2066é🎉"

	for i := range 1000 {
		var b strings.Builder
		for range rng.IntN(40) {
			b.WriteByte(pool[rng.IntN(len(pool))])
		}
		s := Status{
			State:    states[rng.IntN(len(states))],
			Kind:     Kind([]string{"", string(KindQuestion), "permission"}[rng.IntN(3)]),
			Msg:      b.String(),
			Progress: rng.IntN(140) - 20,
		}
		seq := Encode(s)

		if len(seq) > MaxSequence {
			t.Fatalf("iteration %d: len(Encode(%+v)) = %d, want <= %d", i, s, len(seq), MaxSequence)
		}
		body, ok := strings.CutPrefix(seq, "\x1b]7501;")
		if !ok {
			t.Fatalf("iteration %d: sequence %q has no OSC 7501 prefix", i, seq)
		}
		body, ok = strings.CutSuffix(body, "\x1b\\")
		if !ok {
			t.Fatalf("iteration %d: sequence %q has no ST", i, seq)
		}
		for _, pair := range strings.Split(body, ":") {
			key, val, ok := strings.Cut(pair, "=")
			if !ok {
				t.Fatalf("iteration %d: pair %q has no = (from %q)", i, pair, seq)
			}
			if key == "" {
				t.Fatalf("iteration %d: empty key in %q", i, seq)
			}
			if !valueRE.MatchString(val) {
				t.Fatalf("iteration %d: value %q of key %q is outside the protocol charset (msg %q)", i, val, key, s.Msg)
			}
		}
	}
}

func TestReporter(t *testing.T) {
	t.Run("outside Rex nothing is written", func(t *testing.T) {
		t.Setenv("REX_SESSION", "")
		t.Setenv("REX_BLOCK", "")
		var writes []string
		r := NewReporter(os.Getenv, func(s string) { writes = append(writes, s) })
		if r.Enabled() {
			t.Error("Enabled() = true outside Rex, want false")
		}
		r.Report(Status{State: StateWorking, Progress: -1, Msg: "Grouping"})
		r.Report(Status{State: StateDone, Progress: -1})
		if len(writes) != 0 {
			t.Errorf("writes = %q outside Rex, want none", writes)
		}
	})

	t.Run("inside Rex identical reports are written once", func(t *testing.T) {
		t.Setenv("REX_SESSION", "session:1")
		t.Setenv("REX_BLOCK", "block:2")
		var writes []string
		r := NewReporter(os.Getenv, func(s string) { writes = append(writes, s) })
		if !r.Enabled() {
			t.Fatal("Enabled() = false inside Rex, want true")
		}
		working := Status{State: StateWorking, Progress: -1, Msg: "Grouping with haiku"}
		r.Report(working)
		r.Report(working)
		if len(writes) != 1 {
			t.Fatalf("writes = %q for two identical reports, want 1", writes)
		}
		if want := Encode(working); writes[0] != want {
			t.Errorf("wrote %q, want %q", writes[0], want)
		}
		done := Status{State: StateDone, Progress: -1, Msg: "Grouped 12 hunks into 3 changes"}
		r.Report(done)
		if len(writes) != 2 {
			t.Fatalf("writes = %q after a different report, want 2", writes)
		}
		if want := Encode(done); writes[1] != want {
			t.Errorf("wrote %q, want %q", writes[1], want)
		}
		r.Report(working)
		if len(writes) != 3 {
			t.Fatalf("writes = %q after returning to the first report, want 3", writes)
		}
	})

	t.Run("nil Reporter", func(t *testing.T) {
		var r *Reporter
		if r.Enabled() {
			t.Error("nil Reporter.Enabled() = true, want false")
		}
		r.Report(Status{State: StateDone})
	})

	t.Run("concurrent reports", func(t *testing.T) {
		t.Setenv("REX_SESSION", "session:1")
		t.Setenv("REX_BLOCK", "block:2")
		var mu sync.Mutex
		writes := 0
		r := NewReporter(os.Getenv, func(s string) {
			mu.Lock()
			writes++
			mu.Unlock()
			if !strings.HasPrefix(s, "\x1b]7501;") || !strings.HasSuffix(s, "\x1b\\") {
				t.Errorf("concurrent write %q is not a report", s)
			}
		})

		const goroutines, rounds = 8, 25
		var wg sync.WaitGroup
		for g := range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range rounds {
					r.Report(Status{State: StateWorking, Progress: g % 2, Msg: "Grouping"})
					r.Report(Status{State: StateDone, Progress: -1, Msg: "Grouped"})
				}
			}()
		}
		wg.Wait()
		mu.Lock()
		defer mu.Unlock()
		if writes < 1 || writes > goroutines*rounds*2 {
			t.Errorf("writes = %d, want between 1 and %d", writes, goroutines*rounds*2)
		}
	})
}
