package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Limits and defaults of the command provider.
const (
	// defaultTimeoutSec bounds one call when Config.TimeoutSec is unset.
	defaultTimeoutSec = 180
	// maxOutputBytes caps the stdout capture; a CLI that prints more is broken.
	maxOutputBytes = 16 << 20
	// stderrKeepBytes is how much of the tail of stderr is kept for errors.
	stderrKeepBytes = 4 << 10
	// stderrReportBytes is how much of that tail an error message shows.
	stderrReportBytes = 400
	// killWaitDelay bounds how long exec waits for the output pipes after the
	// process group was killed, so a stuck grandchild cannot hang the call.
	killWaitDelay = 2 * time.Second
)

// commandProvider runs an argv with the prompt on stdin and reads stdout in the
// configured format.
type commandProvider struct {
	cfg Config
}

// newCommand builds the command provider for c. New validates c first.
func newCommand(c Config) *commandProvider {
	return &commandProvider{cfg: c}
}

// Name implements Provider.
func (p *commandProvider) Name() string { return p.cfg.Name }

// Complete implements Provider: it runs the argv with prompt on stdin, in its
// own process group so cancellation also kills the CLI's children, and parses
// stdout according to the configured format.
func (p *commandProvider) Complete(ctx context.Context, prompt string) (string, Usage, error) {
	name := p.cfg.Name
	timeoutSec := p.cfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeoutSec
	}

	ctx2, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	stdout := &cappedBuffer{max: maxOutputBytes}
	stderr := &tailBuffer{max: stderrKeepBytes}

	cmd := exec.CommandContext(ctx2, p.cfg.Argv[0], p.cfg.Argv[1:]...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = p.cfg.Dir
	if cmd.Dir == "" {
		// Never let an agent CLI pick up repo context (CLAUDE.md, AGENTS.md).
		cmd.Dir = os.TempDir()
	}
	// cmd.Env stays nil: the CLI inherits our environment (PATH, API keys).
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// Run the CLI in its own process group, so cancelling kills the whole tree
	// (a shell wrapper's children too) instead of only the direct child.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// A negative pid signals the whole process group.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			// The group is already gone, or we may not signal it: fall back to
			// the direct child so the call still ends.
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = killWaitDelay

	// Stop reading (and kill the CLI) as soon as it floods stdout.
	stdout.onOverflow = cancel

	runErr := cmd.Run()
	stderrTail := stderr.String()

	if stdout.overflow {
		return "", Usage{}, errors.New(Redact(fmt.Sprintf("%s: output larger than %d MiB", name, maxOutputBytes>>20)))
	}
	switch {
	case ctx.Err() != nil:
		// The caller's deadline or cancellation ended the call. ctx.Err() is a
		// fixed message ("context canceled", "context deadline exceeded") and
		// carries no secret, so it is safe to wrap without Redact.
		return "", Usage{}, fmt.Errorf("%s: %w", name, ctx.Err())
	case ctx2.Err() != nil:
		return "", Usage{}, errors.New(Redact(fmt.Sprintf("%s: timed out after %ds", name, timeoutSec)))
	case runErr != nil:
		// A non-zero exit: report the exit error and the tail of stderr, which
		// is where a CLI says why it failed.
		if msg := tail(strings.TrimSpace(stderrTail), stderrReportBytes); msg != "" {
			return "", Usage{}, errors.New(Redact(fmt.Sprintf("%s: %v: %s", name, runErr, msg)))
		}
		return "", Usage{}, errors.New(Redact(fmt.Sprintf("%s: %v", name, runErr)))
	}

	switch p.cfg.Format {
	case FormatClaudeJSON:
		return p.parseClaudeJSON(stdout.Bytes())
	case FormatOpencodeJSON:
		return p.parseOpencodeJSON(stdout.Bytes())
	default: // "", FormatRaw
		text := stdout.String()
		return text, p.rawUsage(prompt, text), nil
	}
}

// rawUsage estimates the usage of a raw stdout: input from the prompt, output
// from the bytes the CLI returned, both at the calibrated rate.
func (p *commandProvider) rawUsage(prompt, stdout string) Usage {
	in := p.cfg.EstimateTokens(len(prompt), 0)
	out := p.cfg.ceilTokens(len(stdout), 0)
	return Usage{
		InputTokens:  in,
		OutputTokens: out,
		CostUSD:      p.cfg.Cost(in, out),
		CostSource:   CostEstimated,
	}
}

// claudeJSON is the object `claude -p --output-format json` prints.
type claudeJSON struct {
	Result  string  `json:"result"`
	IsError bool    `json:"is_error"`
	Cost    float64 `json:"total_cost_usd"`
	Usage   struct {
		In  int `json:"input_tokens"`
		Out int `json:"output_tokens"`
		CW  int `json:"cache_creation_input_tokens"`
		CR  int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// parseClaudeJSON reads the single JSON object FormatClaudeJSON prints.
func (p *commandProvider) parseClaudeJSON(stdout []byte) (string, Usage, error) {
	var r claudeJSON
	if err := json.Unmarshal(stdout, &r); err != nil {
		return "", Usage{}, errors.New(Redact(fmt.Sprintf("%s: bad JSON from command: %v", p.cfg.Name, err)))
	}
	if r.IsError {
		return "", Usage{}, errors.New(Redact(fmt.Sprintf("%s: %s", p.cfg.Name, r.Result)))
	}
	return r.Result, Usage{
		InputTokens:  r.Usage.In + r.Usage.CW + r.Usage.CR,
		OutputTokens: r.Usage.Out,
		CacheTokens:  r.Usage.CW + r.Usage.CR,
		CostUSD:      r.Cost,
		CostSource:   CostReported,
	}, nil
}

// opencodeEvent is one JSONL event `opencode run --format json` emits.
type opencodeEvent struct {
	Type string `json:"type"`
	Part struct {
		Text   string  `json:"text"`
		Cost   float64 `json:"cost"`
		Tokens struct {
			In    int `json:"input"`
			Out   int `json:"output"`
			Cache struct {
				Read  int `json:"read"`
				Write int `json:"write"`
			} `json:"cache"`
		} `json:"tokens"`
	} `json:"part"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// parseOpencodeJSON reads the JSONL event stream FormatOpencodeJSON prints.
func (p *commandProvider) parseOpencodeJSON(stdout []byte) (string, Usage, error) {
	var text strings.Builder
	var u Usage
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev opencodeEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue // progress noise, not an event
		}
		switch ev.Type {
		case "text":
			text.WriteString(ev.Part.Text)
		case "step_finish":
			read, write := ev.Part.Tokens.Cache.Read, ev.Part.Tokens.Cache.Write
			u.InputTokens += ev.Part.Tokens.In + read + write
			u.CacheTokens += read + write
			u.OutputTokens += ev.Part.Tokens.Out
			u.CostUSD += ev.Part.Cost
		case "error":
			return "", Usage{}, errors.New(Redact(fmt.Sprintf("%s: %s", p.cfg.Name, ev.Error.Message)))
		}
	}
	u.CostSource = CostReported
	return text.String(), u, nil
}

// cappedBuffer keeps the first max bytes written to it and remembers that more
// arrived, so callers can reject oversized output without holding it.
type cappedBuffer struct {
	max        int
	buf        bytes.Buffer
	overflow   bool
	onOverflow func()
}

// Write implements io.Writer. It never fails: a full buffer drops the rest of
// the output, which the caller reports as an error after the process ends.
func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room < len(p) {
		if room > 0 {
			b.buf.Write(p[:room])
		}
		if !b.overflow {
			b.overflow = true
			if b.onOverflow != nil {
				b.onOverflow()
			}
		}
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

// Bytes returns the captured output.
func (b *cappedBuffer) Bytes() []byte { return b.buf.Bytes() }

// String returns the captured output.
func (b *cappedBuffer) String() string { return b.buf.String() }

// tailBuffer keeps only the last max bytes written to it.
type tailBuffer struct {
	max int
	buf []byte
}

// Write implements io.Writer. It never fails: the head of the output is
// dropped, which is what an error message wants.
func (b *tailBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append(b.buf[:0], b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

// String returns the kept tail.
func (b *tailBuffer) String() string { return string(b.buf) }

// tail returns the last n bytes of s.
func tail(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
