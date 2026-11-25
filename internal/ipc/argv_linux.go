//go:build linux

package ipc

import (
	"fmt"
	"os"
)

// parentArgv reads /proc/<ppid>/cmdline and splits it with SplitCmdline.
func parentArgv() []string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", os.Getppid()))
	if err != nil {
		return nil
	}
	return SplitCmdline(b)
}
