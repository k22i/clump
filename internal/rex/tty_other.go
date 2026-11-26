//go:build !darwin && !linux

package rex

func enterRawMode(fd int) (restore func() error, ok bool) {
	return nil, false
}

func termiosSnapshot(fd int) (snap string, ok bool) {
	return "", false
}
