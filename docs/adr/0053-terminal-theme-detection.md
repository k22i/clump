# The terminal's colour scheme is asked for, once, before the TUI starts

`clump` needs to know whether the terminal is light or dark before it draws anything: the theme picks
every colour the TUI emits, and asking after the first frame means a visible flash of the wrong one.

Precedence: `CLUMP_THEME=light|dark` (explicit, and what tests and scripts set) → a query on the tty →
dark. The query is the colour-scheme report `CSI ? 996 n`, and it is followed by DA1 (`CSI c`) as a
sentinel every terminal answers, so a terminal that does not implement 996 still unblocks the read
instead of hanging until the deadline.

- The read happens in raw mode with `VMIN=0`/`VTIME=1`, so each read returns within 100 ms and the
  loop can watch the clock; the whole query is bounded well before Bubble Tea takes the terminal.
- `ParseThemeReply` looks for `CSI ? 997 ; 1 n` (dark) and `CSI ? 997 ; 2 n` (light) and is re-run
  over the growing buffer, so a reply split across reads still parses (ADR-0004 closes the remaining
  partial-sequence gap).
- Rex answers in 0.2 ms (`C-rex.md`), which is why the query is viable in a startup path that must
  stay fast (ADR-0011).
- Darwin and Linux each spell the termios requests differently (`TIOCGETA`/`TIOCSETA` vs
  `TCGETS`/`TCSETS`); other platforms have no termios at all, so they report the env override or
  `ThemeUnknown` and never query.

Why dark as the default rather than light: every terminal that answers is handled explicitly, and a
terminal that stays silent is far more often dark than light, so the wrong-guess cost lands where it
is least visible.
