//go:build linux

package rex

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Termios requests: linux spells them TCGETS/TCSETS.
const (
	termiosGet = unix.TCGETS
	termiosSet = unix.TCSETS
)

// enterRawMode puts fd in the mode DetectTheme needs — echo and canonical
// input off, VMIN = 0, VTIME = 1 so a read returns within 100 ms — and
// returns a function that restores the saved modes. ok is false when fd is
// not a terminal, in which case nothing is changed.
func enterRawMode(fd int) (restore func() error, ok bool) {
	saved, err := unix.IoctlGetTermios(fd, termiosGet)
	if err != nil {
		return nil, false
	}
	raw := *saved
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1
	if err := unix.IoctlSetTermios(fd, termiosSet, &raw); err != nil {
		return nil, false
	}
	return func() error { return unix.IoctlSetTermios(fd, termiosSet, saved) }, true
}

// termiosSnapshot renders fd's terminal modes as a comparable string, or
// ok = false when fd is not a terminal. Tests use it to check that
// DetectTheme restored what it found. PENDIN is masked out: the kernel sets
// it while input is queued for re-echo, so it describes the input queue, not
// a mode, and changes without anyone touching the terminal.
func termiosSnapshot(fd int) (snap string, ok bool) {
	t, err := unix.IoctlGetTermios(fd, termiosGet)
	if err != nil {
		return "", false
	}
	t.Lflag &^= unix.PENDIN
	return fmt.Sprintf("%+v", *t), true
}
