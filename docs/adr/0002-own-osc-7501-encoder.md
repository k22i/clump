# Own OSC 7501 encoder, not Bubble Tea's `View.ProgramStatus`

Bubble Tea v2 can emit program status itself. clump keeps its own encoder in
`internal/rex` (gated on `REX_SESSION`/`REX_BLOCK`, emitted with `tea.Raw`) instead.

Two reasons: watch mode and the non-TUI paths (`clump push`, one-shot) never build a Bubble Tea
view but must report the same states, and a hand-rolled encoder lets the protocol limits
(control-character stripping, base64 `msg`, 64 KiB truncation, dedupe of identical reports) be
tested directly in `internal/rex/status_test.go`.

Rejected: `View.ProgramStatus` alone, which would have left the non-TUI paths without reports or
forced a second implementation.
