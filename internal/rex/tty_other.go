//go:build !darwin && !linux

package rex

func enterRawMode(fd int) (func() error, error) {
	return nil, errNoTermios
}

func termiosSnapshot(fd int) (snap string, ok bool) {
	return "", false
}
