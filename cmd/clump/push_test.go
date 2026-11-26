package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/k22i/clump/internal/gitdiff"
	"github.com/k22i/clump/internal/ipc"
)

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func clumpBin(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "clumpbin")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "clump")
		cmd := exec.Command("go", "build", "-o", binPath, ".")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			binErr = err
			binPath = string(out)
		}
	})
	if binErr != nil {
		t.Fatalf("build: %v\n%s", binErr, binPath)
	}
	return binPath
}

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../internal/gitdiff/testdata/lazygit/commit-6bdf738-colorArg-always.txt")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func shortRepo(t *testing.T) (repo string, env []string) {
	t.Helper()
	base, err := os.MkdirTemp("", "cp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	repo = filepath.Join(base, "r")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if repo, err = filepath.EvalSymlinks(repo); err != nil {
		t.Fatal(err)
	}
	return repo, append(os.Environ(), "XDG_RUNTIME_DIR="+base)
}

func runPushBin(t *testing.T, dir string, env []string, stdin []byte, args ...string) (stdout, stderr []byte, err error) {
	t.Helper()
	cmd := exec.Command(clumpBin(t), append([]string{"push"}, args...)...)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdin = bytes.NewReader(stdin)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	return o.Bytes(), e.Bytes(), err
}

func TestPushPassesThroughWithoutListener(t *testing.T) {
	repo, env := shortRepo(t)
	in := fixture(t)
	out, errOut, err := runPushBin(t, repo, env, in)
	if err != nil || !bytes.Equal(out, in) || len(errOut) != 0 {
		t.Fatalf("err=%v stderr=%q identical=%v", err, errOut, bytes.Equal(out, in))
	}
	out, errOut, err = runPushBin(t, os.TempDir(), env, in)
	if err != nil || !bytes.Equal(out, in) || len(errOut) != 0 {
		t.Fatalf("outside a repo: err=%v stderr=%q", err, errOut)
	}
}

func TestPushThen(t *testing.T) {
	repo, env := shortRepo(t)
	out, _, err := runPushBin(t, repo, env, []byte("a\nb\n"), "--then", "tr a-z A-Z")
	if err != nil || string(out) != "A\nB\n" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	out, _, err = runPushBin(t, repo, env, []byte("raw\n"), "--then", "exit 3")
	if err != nil || string(out) != "raw\n" {
		t.Fatalf("failing renderer: out=%q err=%v", out, err)
	}
}

func TestPushTTYProbeExitsAtOnce(t *testing.T) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		t.Logf("no controlling terminal (%v); checked the non-TTY paths only", err)
		return
	}
	defer tty.Close()
	cmd := exec.Command(clumpBin(t), "push")
	cmd.Stdin = tty
	start := time.Now()
	out, err := cmd.Output()
	if err != nil || len(out) != 0 || time.Since(start) > 2*time.Second {
		t.Fatalf("probe: err=%v out=%q took %s", err, out, time.Since(start))
	}
}

func TestPushReachesWatch(t *testing.T) {
	repo, env := shortRepo(t)
	t.Setenv("XDG_RUNTIME_DIR", strings.TrimPrefix(env[len(env)-1], "XDG_RUNTIME_DIR="))
	l, err := ipc.Listen(ipc.SocketPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan ipc.Msg, 1)
	go l.Serve(ctx, func(m ipc.Msg) { got <- m })
	in := fixture(t)
	out, _, err := runPushBin(t, repo, append(env, "REX_BLOCK=block:test"), in)
	if err != nil || !bytes.Equal(out, in) {
		t.Fatalf("pass-through broke with a listener: %v", err)
	}
	select {
	case m := <-got:
		if m.V != ipc.Version || m.RepoRoot != repo || m.RexBlock != "block:test" || m.Diff != gitdiff.StripSGR(string(in)) {
			t.Fatalf("msg = %+v", m)
		}
		if strings.Contains(m.Diff, "\x1b[") {
			t.Fatal("SGR not stripped")
		}
		if len(m.GitArgv) == 0 || filepath.Base(m.GitArgv[0]) != filepath.Base(os.Args[0]) {
			t.Fatalf("parent argv = %q", m.GitArgv)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message")
	}
}

func medianRun(t *testing.T, dir string, env []string, in []byte, n int, argv ...string) float64 {
	t.Helper()
	var ts []float64
	for i := 0; i < n; i++ {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir, cmd.Env, cmd.Stdin = dir, env, bytes.NewReader(in)
		start := time.Now()
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		ts = append(ts, float64(time.Since(start).Microseconds())/1000)
	}
	sort.Float64s(ts)
	return ts[len(ts)/2]
}

func TestPushLatency(t *testing.T) {
	repo, env := shortRepo(t)
	in := fixture(t)
	bin := clumpBin(t)
	medianRun(t, repo, env, in, 5, bin, "push")
	cat := medianRun(t, repo, env, in, 40, "cat")
	push := medianRun(t, repo, env, in, 40, bin, "push")
	t.Logf("p50: cat %.2f ms, push %.2f ms, overhead %.2f ms", cat, push, push-cat)
	if os.Getenv("CLUMP_PUSH_LATENCY") == "1" && push-cat > 5 {
		t.Fatalf("push adds %.2f ms over cat (budget 5 ms)", push-cat)
	}
}

func BenchmarkPush(b *testing.B) {
	in, err := os.ReadFile("../../internal/gitdiff/testdata/lazygit/commit-6bdf738-colorArg-always.txt")
	if err != nil {
		b.Fatal(err)
	}
	bin := filepath.Join(b.TempDir(), "clump")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		b.Fatalf("%v\n%s", err, out)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(bin, "push")
		cmd.Stdin = bytes.NewReader(in)
		if err := cmd.Run(); err != nil {
			b.Fatal(err)
		}
	}
}

const (
	initClockBudgetMS  = 12.0
	initBytesBudget    = 2 << 20
	initSingleBudgetMS = 5.0
)

type initTrace struct {
	pkg   string
	clock float64
	bytes int
}

func parseInitTrace(line string) (initTrace, bool) {
	rest, ok := strings.CutPrefix(line, "init ")
	if !ok {
		return initTrace{}, false
	}
	f := strings.Fields(rest)
	if len(f) < 8 || !strings.HasPrefix(f[1], "@") || f[2] != "ms," || f[4] != "ms" || f[5] != "clock," || f[7] != "bytes," {
		return initTrace{}, false
	}
	clock, err := strconv.ParseFloat(f[3], 64)
	if err != nil {
		return initTrace{}, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(f[6], ","))
	if err != nil {
		return initTrace{}, false
	}
	return initTrace{pkg: f[0], clock: clock, bytes: n}, true
}

func TestPushInitBudget(t *testing.T) {
	repo, env := shortRepo(t)
	cmd := exec.Command(clumpBin(t), "push")
	cmd.Dir, cmd.Env = repo, append(env, "GODEBUG=inittrace=1")
	cmd.Stdin = bytes.NewReader(fixture(t))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("push: %v\n%s", err, stderr.String())
	}
	var traces []initTrace
	clock, bytesOut := 0.0, 0
	for _, line := range strings.Split(stderr.String(), "\n") {
		tr, ok := parseInitTrace(line)
		if !ok {
			continue
		}
		traces = append(traces, tr)
		clock += tr.clock
		bytesOut += tr.bytes
	}
	if len(traces) == 0 {
		t.Fatalf("no init trace in stderr:\n%s", stderr.String())
	}
	sort.Slice(traces, func(i, j int) bool { return traces[i].bytes > traces[j].bytes })
	worst := func(by func(a, b initTrace) bool) initTrace {
		out := traces[0]
		for _, tr := range traces[1:] {
			if by(tr, out) {
				out = tr
			}
		}
		return out
	}
	heaviestClock := worst(func(a, b initTrace) bool { return a.clock > b.clock })
	t.Logf("init budget: %.2f ms clock, %d bytes over %d packages; worst %.2f ms and %d bytes in %s",
		clock, bytesOut, len(traces), heaviestClock.clock, traces[0].bytes, traces[0].pkg)
	if clock > initClockBudgetMS {
		t.Fatalf("package init takes %.2f ms before main (budget %.1f ms); heaviest: %s %.2f ms, %s %d bytes",
			clock, initClockBudgetMS, heaviestClock.pkg, heaviestClock.clock, traces[0].pkg, traces[0].bytes)
	}
	if heaviestClock.clock > initSingleBudgetMS {
		t.Fatalf("%s init takes %.2f ms (budget %.1f ms); it belongs behind a lazy registry (ADR-0011)",
			heaviestClock.pkg, heaviestClock.clock, initSingleBudgetMS)
	}
	if bytesOut > initBytesBudget {
		t.Fatalf("package init allocates %d bytes before main (budget %d); heaviest: %s %d bytes, %s %d bytes",
			bytesOut, initBytesBudget, traces[0].pkg, traces[0].bytes, traces[1].pkg, traces[1].bytes)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("\n")) {
		t.Fatalf("push did not pass stdin through: %q", stdout.String())
	}
}
