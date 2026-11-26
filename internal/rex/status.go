package rex

import (
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

const App = "clump"

type State string

const (
	StateIdle    State = "idle"
	StateWorking State = "working"
	StateDone    State = "done"
	StateBlocked State = "blocked"
	StateError   State = "error"
	StateClear   State = "clear"
)

type Kind string

const (
	KindNone     Kind = ""
	KindQuestion Kind = "question"
)

type Status struct {
	State    State
	Kind     Kind
	Msg      string
	Progress int
}

const (
	MaxMsgDecoded = 2048
	MaxSequence   = 4096
)

func InRex(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	return getenv("REX_SESSION") != "" && getenv("REX_BLOCK") != ""
}

func SanitizeMsg(s string) string {
	s = strings.ToValidUTF8(s, string(utf8.RuneError))

	var b strings.Builder
	b.Grow(len(s))
	spaced := true
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

func Encode(s Status) string {
	var b strings.Builder
	b.Grow(64 + len(s.Msg)*4/3)
	b.WriteString("\x1b]7501;")
	if s.State == StateClear {
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

type Reporter struct {
	mu      sync.Mutex
	enabled bool
	write   func(string)
	last    string
}

func NewReporter(getenv func(string) string, write func(string)) *Reporter {
	return &Reporter{enabled: InRex(getenv) && write != nil, write: write}
}

func (r *Reporter) Enabled() bool { return r != nil && r.enabled }

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
