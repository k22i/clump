//go:build linux

package ipc

// parentArgv reads /proc/<ppid>/cmdline and splits it with SplitCmdline.
func parentArgv() []string {
	panic("ipc: parentArgv (linux) not implemented")
}
