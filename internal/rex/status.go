// Package rex holds clump's Rex specialisations: the Program Status Protocol
// (OSC 7501) encoder gated on running inside Rex, the light/dark theme query
// (CSI ? 996 n), and `clump rex install`.
package rex

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
	panic("rex: InRex not implemented")
}

// SanitizeMsg makes s a legal msg: every control character (U+0000-U+001F,
// U+007F, U+0080-U+009F, which includes \n and \t) becomes a space, runs of
// spaces collapse to one, the result is trimmed, invalid UTF-8 is replaced
// with U+FFFD, and it is cut to at most MaxMsgDecoded bytes on a rune
// boundary.
func SanitizeMsg(s string) string {
	panic("rex: SanitizeMsg not implemented")
}

// Encode returns the OSC 7501 report for s, terminated with ESC \:
//
//	ESC ] 7501 ; state=<state>[:kind=<kind>][:progress=<n>]:app=clump[:msg=<base64>] ESC \
//
// msg is standard base64 (with padding) of SanitizeMsg(s.Msg), omitted
// when that is empty. StateClear is written as just "state=clear".
func Encode(s Status) string {
	panic("rex: Encode not implemented")
}

// Reporter writes status reports, but only inside Rex. Its zero value is a
// no-op.
type Reporter struct {
	enabled bool
	write   func(string)
	last    string
}

// NewReporter returns a Reporter that calls write(Encode(s)) for each
// report when InRex(getenv), and does nothing otherwise.
func NewReporter(getenv func(string) string, write func(string)) *Reporter {
	panic("rex: NewReporter not implemented")
}

// Enabled reports whether reports are written.
func (r *Reporter) Enabled() bool { return r != nil && r.enabled }

// Report writes s unless it encodes identically to the previous report.
func (r *Reporter) Report(s Status) {
	panic("rex: Reporter.Report not implemented")
}
