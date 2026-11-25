//go:build darwin

package ipc

import (
	"os"

	"golang.org/x/sys/unix"
)

// parentArgv reads kern.procargs2 for os.Getppid() and decodes it with
// DecodeProcargs2.
func parentArgv() []string {
	b, err := unix.SysctlRaw("kern.procargs2", os.Getppid())
	if err != nil {
		return nil
	}
	return DecodeProcargs2(b)
}
