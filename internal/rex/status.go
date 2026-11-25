// Package rex holds clump's Rex specialisations: the Program Status Protocol
// (OSC 7501) encoder gated on running inside Rex, the light/dark theme query
// (CSI ? 996 n), and `clump rex install`.
package rex

import (
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// App is the app= value on every status report.
const App = "clump"

// State is an OSC 7501 state.
type State string

// States clump reports.
const (
	StateIdle    State = "idle"
	StateWorking State = "working"
	StateDone    State = "done"
	StateBlocked State = "blocked"
	StateError   State = "error"
	StateClear   State = "clear"
)

// Kind is the kind= of a blocked report.
type Kind string

// Kinds.
const (
	KindNone     Kind = ""
	KindQuestion Kind = "question"
)

// Status is one report.
type Status struct {
	State State
	Kind  Kind // only written when State is StateBlocked and Kind != KindNone
	Msg   string
	// Progress 0..100, written only for working/blocked; < 0 means absent.
	Progress int
}

// Limits from the protocol spec.
const (
	MaxMsgDecoded = 2048 // bytes of UTF-8 before base64
	MaxSequence   = 4096 // bytes, OSC through ST
)

// InRex reports whether clump runs inside Rex: both REX_SESSION and
// REX_BLOCK are non-empty in env (a getenv function, e.g. os.Getenv).
func InRex(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	return getenv("REX_SESSION") != "" && getenv("REX_BLOCK") != ""
}

// SanitizeMsg makes s a legal msg: every control character (U+0000-U+001F,
// U+007F, U+0080-U+009F, which includes \n and \t) becomes a space, runs of
// spaces collapse to one, the result is trimmed, invalid UTF-8 is replaced
// with U+FFFD, and it is cut to at most MaxMsgDecoded bytes on a rune
// boundary.
func SanitizeMsg(s string) string {
	s = strings.ToValidUTF8(s, string(utf8.RuneError))

	var b strings.Builder
	b.Grow(len(s))
	spaced := true // the start of the string trims leading spaces
	for _, r := range s {
		if r == ' ' || isInvisible(r) {
			if !spaced {
				b.WriteByte(' ')
				spaced = true
			}
			continue
		}
		b.WriteRune(r)
		spaced = false
	}
	out := strings.TrimRight(b.String(), " ")

	if len(out) > MaxMsgDecoded {
		cut := MaxMsgDecoded
		for cut > 0 && !utf8.RuneStart(out[cut]) {
			cut--
		}
		out = strings.TrimRight(out[:cut], " ")
	}
	return out
}

// isInvisible reports whether r must not reach a terminal from a msg: the C0
// and C1 controls, and the bidi controls that would reorder the line.
func isInvisible(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r == 0x200e, r == 0x200f:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// Encode returns the OSC 7501 report for s, terminated with ESC \:
//
//	ESC ] 7501 ; state=<state>[:kind=<kind>][:progress=<n>]:app=clump[:msg=<base64>] ESC \
//
// msg is standard base64 (with padding) of SanitizeMsg(s.Msg), omitted
// when that is empty. StateClear is written as just "state=clear".
func Encode(s Status) string {
	var b strings.Builder
	b.Grow(64 + len(s.Msg)*4/3)
	b.WriteString("\x1b]7501;")
	if s.State == StateClear {
		// clear removes a record; it carries nothing else.
		b.WriteString("state=clear")
	} else {
		pairs := make([]string, 0, 5)
		pairs = append(pairs, "state="+string(s.State))
		if s.State == StateBlocked && s.Kind != KindNone {
			pairs = append(pairs, "kind="+string(s.Kind))
		}
		if (s.State == StateWorking || s.State == StateBlocked) && s.Progress >= 0 && s.Progress <= 100 {
			pairs = append(pairs, "progress="+strconv.Itoa(s.Progress))
		}
		pairs = append(pairs, "app="+App)
		if msg := SanitizeMsg(s.Msg); msg != "" {
			pairs = append(pairs, "msg="+base64.StdEncoding.EncodeToString([]byte(msg)))
		}
		b.WriteString(strings.Join(pairs, ":"))
	}
	b.WriteString("\x1b\\")
	return b.String()
}

// Reporter writes status reports, but only inside Rex. Its zero value is a
// no-op.
type Reporter struct {
	mu      sync.Mutex
	enabled bool
	write   func(string)
	last    string // last sequence written, to drop identical reports
}

// NewReporter returns a Reporter that calls write(Encode(s)) for each
// report when InRex(getenv), and does nothing otherwise.
func NewReporter(getenv func(string) string, write func(string)) *Reporter {
	return &Reporter{enabled: InRex(getenv) && write != nil, write: write}
}

// Enabled reports whether reports are written.
func (r *Reporter) Enabled() bool { return r != nil && r.enabled }

// Report writes s unless it encodes identically to the previous report.
// It is safe for concurrent use; write must not call Report.
func (r *Reporter) Report(s Status) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled || r.write == nil {
		return
	}
	seq := Encode(s)
	if seq == r.last {
		return
	}
	r.last = seq
	r.write(seq)
}
