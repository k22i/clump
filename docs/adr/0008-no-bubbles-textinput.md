# No `bubbles/textinput`

`internal/tui/input.go` is a hand-written single-line editor rather than `bubbles/textinput`.

`bubbles/textinput` pulls in `github.com/atotto/clipboard`, which is outside the agreed dependency
set (the whole list is in ADR-0046). Bubbles is used for the spinner only. The line editor
needs enter/esc/backspace/cursor-left/right and nothing else, so the dependency was not worth it.
