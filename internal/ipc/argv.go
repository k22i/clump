package ipc

// DecodeProcargs2 decodes a darwin kern.procargs2 buffer: a little-endian
// int32 argc, the exec path, NUL padding, then argc NUL-terminated args. It
// returns nil for a malformed buffer. Pure, so it is tested on every OS.
func DecodeProcargs2(b []byte) []string {
	panic("ipc: DecodeProcargs2 not implemented")
}

// SplitCmdline splits a linux /proc/<pid>/cmdline buffer (NUL-separated,
// usually NUL-terminated) into args; nil for an empty buffer.
func SplitCmdline(b []byte) []string {
	panic("ipc: SplitCmdline not implemented")
}
