//go:build darwin

package rex

import (
	"fmt"

	"golang.org/x/sys/unix"
)

const (
	termiosGet = unix.TIOCGETA
	termiosSet = unix.TIOCSETA
)

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

func termiosSnapshot(fd int) (snap string, ok bool) {
	t, err := unix.IoctlGetTermios(fd, termiosGet)
	if err != nil {
		return "", false
	}
	t.Lflag &^= unix.PENDIN
	return fmt.Sprintf("%+v", *t), true
}
