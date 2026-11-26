package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Usage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CacheTokens  int     `json:"cache_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd"`
	CostSource   string  `json:"cost_source"`
}

type Provider interface {
	Name() string
	Complete(ctx context.Context, prompt string) (text string, u Usage, err error)
}

type commandProvider struct {
	name   string
	argv   []string
	format string
	inPer, outPer float64
}

func (c *commandProvider) Name() string { return c.name }

func (c *commandProvider) Complete(ctx context.Context, prompt string) (string, Usage, error) {
	cmd := exec.CommandContext(ctx, c.argv[0], c.argv[1:]...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = os.TempDir()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", Usage{}, fmt.Errorf("%s: %v: %s", c.name, err, redact(tail(errb.String(), 400)))
	}
	switch c.format {
	case "claude-json":
		var r struct {
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
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			return "", Usage{}, fmt.Errorf("%s: bad JSON from CLI: %v", c.name, err)
		}
		if r.IsError {
			return "", Usage{}, fmt.Errorf("%s: %s", c.name, redact(r.Result))
		}
		return r.Result, Usage{InputTokens: r.Usage.In + r.Usage.CW + r.Usage.CR, OutputTokens: r.Usage.Out,
			CacheTokens: r.Usage.CW + r.Usage.CR, CostUSD: r.Cost, CostSource: "reported"}, nil
	case "opencode-json":
		var text strings.Builder
		var u Usage
		for _, line := range strings.Split(out.String(), "\n") {
			var ev struct {
				Type string `json:"type"`
				Part struct {
					Text   string  `json:"text"`
					Cost   float64 `json:"cost"`
					Tokens struct {
						In    int `json:"input"`
						Out   int `json:"output"`
						Cache struct {
							R int `json:"read"`
							W int `json:"write"`
						} `json:"cache"`
					} `json:"tokens"`
				} `json:"part"`
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(line), &ev) != nil {
				continue
			}
			switch ev.Type {
			case "text":
				text.WriteString(ev.Part.Text)
			case "step_finish":
				u.InputTokens += ev.Part.Tokens.In + ev.Part.Tokens.Cache.R + ev.Part.Tokens.Cache.W
				u.CacheTokens += ev.Part.Tokens.Cache.R + ev.Part.Tokens.Cache.W
				u.OutputTokens += ev.Part.Tokens.Out
				u.CostUSD += ev.Part.Cost
			case "error":
				return "", Usage{}, fmt.Errorf("%s: %s", c.name, redact(ev.Error.Message))
			}
		}
		u.CostSource = "reported"
		return text.String(), u, nil
	default:
		u := Usage{InputTokens: EstimateTokens(prompt), OutputTokens: EstimateTokens(out.String()), CostSource: "estimated"}
		u.CostUSD = (float64(u.InputTokens)*c.inPer + float64(u.OutputTokens)*c.outPer) / 1e6
		return out.String(), u, nil
	}
}

type openAIProvider struct {
	name, baseURL, model, keyEnv string
	inPer, outPer                float64
	client                       *http.Client
}

func (o *openAIProvider) Name() string { return o.name }

func (o *openAIProvider) Complete(ctx context.Context, prompt string) (string, Usage, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       o.model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": "You output only JSON."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(o.baseURL, "/")+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if o.keyEnv != "" {
		k := os.Getenv(o.keyEnv)
		if k == "" {
			return "", Usage{}, fmt.Errorf("%s: env var %s is not set", o.name, o.keyEnv)
		}
		req.Header.Set("Authorization", "Bearer "+k)
	}
	cl := o.client
	if cl == nil {
		cl = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("%s: %s", o.name, redact(err.Error()))
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", Usage{}, fmt.Errorf("%s: HTTP %d: %s", o.name, resp.StatusCode, redact(tail(string(rb), 300)))
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			In  int `json:"prompt_tokens"`
			Out int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rb, &r); err != nil || len(r.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("%s: unexpected response", o.name)
	}
	u := Usage{InputTokens: r.Usage.In, OutputTokens: r.Usage.Out, CostSource: "estimated"}
	u.CostUSD = (float64(u.InputTokens)*o.inPer + float64(u.OutputTokens)*o.outPer) / 1e6
	return r.Choices[0].Message.Content, u, nil
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func redact(s string) string {
	for _, k := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "DEEPSEEK_API_KEY", "OPENROUTER_API_KEY"} {
		if v := os.Getenv(k); len(v) > 8 {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	return keyRe.ReplaceAllString(s, "[REDACTED]")
}
