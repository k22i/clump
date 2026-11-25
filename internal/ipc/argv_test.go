package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// procargs2 builds a kern.procargs2 buffer: argc, then the given raw parts
// (which carry their own NULs).
func procargs2(argc int32, parts ...string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, uint32(argc))
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

func TestDecodeProcargs2(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []string
	}{
		{
			name: "exec path, padding, three args",
			in:   procargs2(3, "/usr/bin/git\x00\x00\x00", "git\x00", "show\x00", "abc\x00"),
			want: []string{"git", "show", "abc"},
		},
		{
			name: "no padding after the exec path",
			in:   procargs2(3, "/usr/bin/git\x00", "git\x00", "diff\x00", "--\x00"),
			want: []string{"git", "diff", "--"},
		},
		{
			name: "last arg has no trailing NUL",
			in:   procargs2(3, "/usr/bin/git\x00\x00", "git\x00", "show\x00", "abc"),
			want: []string{"git", "show", "abc"},
		},
		{
			name: "fewer args than argc",
			in:   procargs2(3, "/usr/bin/git\x00\x00", "git\x00"),
			want: []string{"git"},
		},
		{
			name: "more args than argc",
			in:   procargs2(1, "/usr/bin/git\x00\x00", "git\x00", "show\x00", "abc\x00"),
			want: []string{"git"},
		},
		{
			name: "too short for argc",
			in:   []byte{3, 0, 0},
			want: nil,
		},
		{
			name: "empty buffer",
			in:   []byte{},
			want: nil,
		},
		{
			name: "argc zero",
			in:   procargs2(0, "/usr/bin/git\x00\x00", "git\x00"),
			want: nil,
		},
		{
			name: "argc negative",
			in:   procargs2(-1, "/usr/bin/git\x00\x00", "git\x00"),
			want: nil,
		},
		{
			name: "argc absurdly large",
			in:   procargs2(5000, "/usr/bin/git\x00\x00", "git\x00"),
			want: nil,
		},
		{
			name: "no NUL after the exec path",
			in:   procargs2(1, "/usr/bin/git"),
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DecodeProcargs2(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("DecodeProcargs2(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitCmdline(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []string
	}{
		{
			name: "trailing NUL",
			in:   []byte("git\x00diff\x00--\x00"),
			want: []string{"git", "diff", "--"},
		},
		{
			name: "no trailing NUL",
			in:   []byte("git\x00diff\x00--"),
			want: []string{"git", "diff", "--"},
		},
		{
			name: "single arg",
			in:   []byte("git\x00"),
			want: []string{"git"},
		},
		{
			name: "empty buffer",
			in:   nil,
			want: nil,
		},
		{
			name: "only a NUL",
			in:   []byte{0},
			want: nil,
		},
		{
			name: "arg with spaces and flags",
			in:   []byte("git\x00show\x00--stat\x00-p\x00abc123\x00--\x00"),
			want: []string{"git", "show", "--stat", "-p", "abc123", "--"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SplitCmdline(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SplitCmdline(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestHelperParentArgv is not a test: it is the child process body used by
// TestParentArgvOfChild. It prints its own parent's argv as JSON and exits.
func TestHelperParentArgv(t *testing.T) {
	if os.Getenv("IPC_HELPER") != "1" {
		t.Log("helper process only: run through TestParentArgvOfChild")
		return
	}
	b, err := json.Marshal(ParentArgv())
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal parent argv: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ARGV:%s\n", b)
	os.Exit(0)
}

// TestParentArgvOfChild re-execs the test binary and checks that the child
// reads this process's argv: the real end-to-end check of ParentArgv.
func TestParentArgvOfChild(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperParentArgv")
	cmd.Env = append(os.Environ(), "IPC_HELPER=1")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("helper failed: %v\nstderr:\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("run helper: %v", err)
	}

	payload, ok := "", false
	for _, line := range strings.Split(string(out), "\n") {
		if s, found := strings.CutPrefix(line, "ARGV:"); found {
			payload, ok = s, true
		}
	}
	if !ok {
		t.Fatalf("helper printed no ARGV line; output:\n%s", out)
	}
	var argv []string
	if err := json.Unmarshal([]byte(payload), &argv); err != nil {
		t.Fatalf("decode helper argv %q: %v", payload, err)
	}

	switch runtime.GOOS {
	case "darwin", "linux":
		if len(argv) == 0 {
			t.Fatal("ParentArgv returned nothing for the test binary")
		}
		if got, want := filepath.Base(argv[0]), filepath.Base(os.Args[0]); got != want {
			t.Fatalf("argv[0] = %q, want %q (full argv: %q)", got, want, argv)
		}
		var flagged bool
		for _, a := range argv {
			if strings.HasPrefix(a, "-test.") {
				flagged = true
				break
			}
		}
		if !flagged {
			t.Fatalf("argv has no -test.* argument: %q", argv)
		}
	default:
		if len(argv) != 0 {
			t.Fatalf("ParentArgv = %q on %s, want nil", argv, runtime.GOOS)
		}
	}
}
