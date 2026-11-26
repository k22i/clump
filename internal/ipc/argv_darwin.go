//go:build darwin

package ipc

import (
	"os"

	"golang.org/x/sys/unix"
)

func parentArgv() []string {
	b, err := unix.SysctlRaw("kern.procargs2", os.Getppid())
	if err != nil {
		return nil
	}
	return DecodeProcargs2(b)
}
