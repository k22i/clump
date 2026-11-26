package ipc

import (
	"bytes"
	"encoding/binary"
	"strings"
)

const maxArgc = 4096

func DecodeProcargs2(b []byte) []string {
	if len(b) < 4 {
		return nil
	}
	argc := int(int32(binary.LittleEndian.Uint32(b[:4])))
	if argc <= 0 || argc > maxArgc {
		return nil
	}
	rest := b[4:]
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
			out = append(out, string(rest))
			break
		}
		out = append(out, string(rest[:j]))
		rest = rest[j+1:]
	}
	return out
}

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
