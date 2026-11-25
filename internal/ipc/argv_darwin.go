//go:build darwin

package ipc

// parentArgv reads kern.procargs2 for os.Getppid() and decodes it with
// DecodeProcargs2.
func parentArgv() []string {
	panic("ipc: parentArgv (darwin) not implemented")
}
