package ipc

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// maxArgc is the largest argv size DecodeProcargs2 accepts; a bigger count
// means the buffer is not a process-argument block.
const maxArgc = 4096

// DecodeProcargs2 decodes a darwin kern.procargs2 buffer: a little-endian
// int32 argc, the exec path, NUL padding, then argc NUL-terminated args. It
// returns nil for a malformed buffer. Pure, so it is tested on every OS.
func DecodeProcargs2(b []byte) []string {
	if len(b) < 4 {
		return nil
	}
	argc := int(int32(binary.LittleEndian.Uint32(b[:4])))
	if argc <= 0 || argc > maxArgc {
		return nil
	}
	rest := b[4:]
	// The exec path, then zero or more padding NULs before the args.
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil
	}
	rest = rest[i+1:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	var out []string
	for len(rest) > 0 && len(out) < argc {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			// The last argument may lack its NUL at the end of the buffer.
			out = append(out, string(rest))
			break
		}
		out = append(out, string(rest[:j]))
		rest = rest[j+1:]
	}
	return out
}

// SplitCmdline splits a linux /proc/<pid>/cmdline buffer (NUL-separated,
// usually NUL-terminated) into args; nil for an empty buffer.
func SplitCmdline(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	b = bytes.TrimSuffix(b, []byte{0})
	if len(b) == 0 {
		return nil
	}
	return strings.Split(string(b), "\x00")
}
