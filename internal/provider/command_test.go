package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writeScript writes an executable /bin/sh script to a temp dir and returns its
// path. Tests never call a real agent CLI.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

// mustNew builds a provider from cfg or fails the test.
func mustNew(t *testing.T, cfg Config) Provider {
	t.Helper()
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New(%+v): %v", cfg, err)
	}
	return p
}

// evalSymlinks resolves path for comparison; a path that does not exist is
// returned unchanged.
func evalSymlinks(t *testing.T, path string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return got
}

func TestCommandRawStdinAndDir(t *testing.T) {
	cfg := Config{Name: "pwd-cat", Type: TypeCommand, Argv: []string{"sh", "-c", "pwd; cat"}}
	p := mustNew(t, cfg)
	prompt := "hello\n"

	text, u, err := p.Complete(context.Background(), prompt)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	dir, rest, ok := strings.Cut(text, "\n")
	if !ok {
		t.Fatalf("output %q has no newline", text)
	}
	if got, want := evalSymlinks(t, dir), evalSymlinks(t, os.TempDir()); got != want {
		t.Fatalf("working directory = %q (%q), want %q (%q)", dir, got, os.TempDir(), want)
	}
	if rest != prompt {
		t.Fatalf("stdout after pwd = %q, want %q", rest, prompt)
	}
	if u.CostSource != CostEstimated {
		t.Fatalf("CostSource = %q, want %q", u.CostSource, CostEstimated)
	}
	if want := cfg.EstimateTokens(len(prompt), 0); u.InputTokens != want {
		t.Fatalf("InputTokens = %d, want %d", u.InputTokens, want)
	}
	if want := cfg.ceilTokens(len(text), 0); u.OutputTokens != want {
		t.Fatalf("OutputTokens = %d, want %d", u.OutputTokens, want)
	}
	if want := cfg.Cost(u.InputTokens, u.OutputTokens); u.CostUSD != want {
		t.Fatalf("CostUSD = %v, want %v", u.CostUSD, want)
	}
}

func TestCommandDir(t *testing.T) {
	dir := t.TempDir()
	p := mustNew(t, Config{Name: "pwd", Type: TypeCommand, Argv: []string{"sh", "-c", "pwd"}, Dir: dir})

	text, _, err := p.Complete(context.Background(), "")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got, want := evalSymlinks(t, strings.TrimSpace(text)), evalSymlinks(t, dir); got != want {
		t.Fatalf("pwd = %q (%q), want %q (%q)", text, got, dir, want)
	}
}

func TestCommandClaudeJSON(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
cat <<'EOF'
{"type":"result","subtype":"success","is_error":false,"result":"{\"groups\":[]}","total_cost_usd":0.0009,
 "usage":{"input_tokens":3,"cache_creation_input_tokens":3262,"cache_read_input_tokens":0,"output_tokens":543}}
EOF
`)
	p := mustNew(t, Config{Name: "claude", Type: TypeCommand, Argv: []string{script}, Format: FormatClaudeJSON})

	text, u, err := p.Complete(context.Background(), "x")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != `{"groups":[]}` {
		t.Fatalf("text = %q", text)
	}
	if u.InputTokens != 3265 || u.CacheTokens != 3262 || u.OutputTokens != 543 {
		t.Fatalf("usage = %+v, want input 3265 cache 3262 output 543", u)
	}
	if u.CostUSD != 0.0009 {
		t.Fatalf("CostUSD = %v, want 0.0009", u.CostUSD)
	}
	if u.CostSource != CostReported {
		t.Fatalf("CostSource = %q, want %q", u.CostSource, CostReported)
	}
}

func TestCommandClaudeJSONIsError(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
printf '%s' '{"is_error":true,"result":"rate limited"}'
`)
	p := mustNew(t, Config{Name: "claude", Type: TypeCommand, Argv: []string{script}, Format: FormatClaudeJSON})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("error = %v, want it to mention \"rate limited\"", err)
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Fatalf("error = %v, want it to name the provider", err)
	}
}

func TestCommandClaudeJSONBad(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
echo 'not json'
`)
	p := mustNew(t, Config{Name: "claude", Type: TypeCommand, Argv: []string{script}, Format: FormatClaudeJSON})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "bad JSON from command") {
		t.Fatalf("error = %v, want \"bad JSON from command\"", err)
	}
}

func TestCommandOpencodeJSON(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
cat <<'EOF'
not json at all
{"type":"text","part":{"text":"{\"gro"}}
{"type":"text","part":{"text":"ups\":[]}"}}
{"type":"step_finish","part":{"cost":0.1,"tokens":{"input":10,"output":5,"cache":{"read":1,"write":2}}}}
EOF
`)
	p := mustNew(t, Config{Name: "oc", Type: TypeCommand, Argv: []string{script}, Format: FormatOpencodeJSON})

	text, u, err := p.Complete(context.Background(), "x")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != `{"groups":[]}` {
		t.Fatalf("text = %q, want %q", text, `{"groups":[]}`)
	}
	want := Usage{InputTokens: 13, OutputTokens: 5, CacheTokens: 3, CostUSD: 0.1, CostSource: CostReported}
	if u != want {
		t.Fatalf("usage = %+v, want %+v", u, want)
	}
}

func TestCommandOpencodeError(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
cat <<'EOF'
{"type":"text","part":{"text":"partial"}}
{"type":"error","error":{"message":"model overloaded"}}
EOF
`)
	p := mustNew(t, Config{Name: "oc", Type: TypeCommand, Argv: []string{script}, Format: FormatOpencodeJSON})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "model overloaded") {
		t.Fatalf("error = %v, want it to mention \"model overloaded\"", err)
	}
}

func TestCommandExitError(t *testing.T) {
	p := mustNew(t, Config{Name: "failing", Type: TypeCommand, Argv: []string{"sh", "-c", "echo boom >&2; exit 3"}})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want an exit error")
	}
	// Provider name, exit status and the stderr tail, in that order.
	for _, want := range []string{"failing: exit status 3: boom"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestCommandRedactsStderr(t *testing.T) {
	const key = "sk-live-abcdefghijklmnop"
	t.Setenv("OPENAI_API_KEY", key)

	p := mustNew(t, Config{Name: "leak", Type: TypeCommand,
		Argv: []string{"sh", "-c", `echo "$OPENAI_API_KEY" >&2; exit 1`}})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want an error")
	}
	if strings.Contains(err.Error(), "abcdefghijklmnop") {
		t.Fatalf("error leaks the key: %q", err)
	}
	if !strings.Contains(err.Error(), redactedMarker) {
		t.Fatalf("error = %q, want it to contain %q", err, redactedMarker)
	}
}

func TestCommandCancel(t *testing.T) {
	p := mustNew(t, Config{Name: "sleepy", Type: TypeCommand, Argv: []string{"sh", "-c", "sleep 30"}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := p.Complete(ctx, "x")
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancelled := time.Now()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
		if waited := time.Since(cancelled); waited > 3*time.Second {
			t.Fatalf("Complete took %v after cancellation, want < 3s", waited)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Complete did not return within 3s of cancellation")
	}
}

func TestCommandKillsGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := writeScript(t, "#!/bin/sh\nsleep 30 &\necho $! > "+pidFile+"\nwait\n")
	p := mustNew(t, Config{Name: "tree", Type: TypeCommand, Argv: []string{script}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := p.Complete(ctx, "x")
		done <- err
	}()

	// Wait until the shell has forked the child, so we have a pid to check.
	var child int
	for deadline := time.Now().Add(10 * time.Second); child == 0; {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("the script never wrote its child pid")
		}
		if b, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				child = pid
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Complete did not return after cancellation")
	}

	// The grandchild must be dead too: signal 0 fails once it is gone.
	for deadline := time.Now().Add(2 * time.Second); ; {
		err := syscall.Kill(child, 0)
		if err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %d is still alive 2s after cancellation", child)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCommandTimeout(t *testing.T) {
	p := mustNew(t, Config{Name: "slow", Type: TypeCommand, TimeoutSec: 1, Argv: []string{"sh", "-c", "sleep 30"}})

	start := time.Now()
	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want a timeout")
	}
	if !strings.Contains(err.Error(), "timed out after 1s") {
		t.Fatalf("error = %q, want \"timed out after 1s\"", err)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("Complete took %v, want < 4s", elapsed)
	}
}

func TestCommandMissingBinary(t *testing.T) {
	p := mustNew(t, Config{Name: "missing", Type: TypeCommand, Argv: []string{"clump-no-such-binary-xyz"}})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want a start error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %q, want it to name the provider", err)
	}
}
