package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	maxResponseBytes                = 8 << 20
	defaultHTTPTimeoutSec           = 180
	openAIDefaultMaxOutputTokens    = 4096
	anthropicDefaultMaxOutputTokens = 16000
	errorBodyBytes                  = 300
	jsonSystemPrompt                = "You are a JSON API. Follow the user message exactly and output only JSON."
	anthropicVersion                = "2026-10-21"
	defaultAnthropicBaseURL         = "https://api.anthropic.com"
	defaultAnthropicKeyEnv          = "ANTHROPIC_API_KEY"
)

func newOpenAI(c Config) (Provider, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("%w: provider %q: base_url is required", ErrConfig, c.Name)
	}
	if strings.TrimSpace(c.Model) == "" {
		return nil, fmt.Errorf("%w: provider %q: model is required", ErrConfig, c.Name)
	}
	return &openAIProvider{
		cfg:      c,
		endpoint: strings.TrimRight(c.BaseURL, "/") + "/chat/completions",
		client:   newHTTPClient(c),
	}, nil
}

func newAnthropic(c Config) (Provider, error) {
	if strings.TrimSpace(c.Model) == "" {
		return nil, fmt.Errorf("%w: provider %q: model is required", ErrConfig, c.Name)
	}
	baseURL := c.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultAnthropicBaseURL
	}
	keyEnv := c.APIKeyEnv
	if keyEnv == "" {
		keyEnv = defaultAnthropicKeyEnv
	}
	RegisterSecretEnv(keyEnv)
	return &anthropicProvider{
		cfg:      c,
		keyEnv:   keyEnv,
		endpoint: strings.TrimRight(baseURL, "/") + "/v1/messages",
		client:   newHTTPClient(c),
	}, nil
}

func newHTTPClient(c Config) *http.Client {
	sec := c.TimeoutSec
	if sec <= 0 {
		sec = defaultHTTPTimeoutSec
	}
	return &http.Client{Timeout: time.Duration(sec) * time.Second}
}

func maxTokens(n, def int) int {
	if n <= 0 {
		return def
	}
	return n
}

func postJSON(ctx context.Context, name string, client *http.Client, endpoint string, header map[string]string, payload []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %s", name, Redact(err.Error()))
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, httpCallError(ctx, name, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return 0, nil, httpCallError(ctx, name, err)
	}
	if len(body) > maxResponseBytes {
		return 0, nil, fmt.Errorf("%s: response larger than %d MiB", name, maxResponseBytes>>20)
	}
	return resp.StatusCode, body, nil
}

func httpCallError(ctx context.Context, name string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", name, ctxErr)
	}
	return fmt.Errorf("%s: %s", name, Redact(err.Error()))
}

func errorBody(body []byte) string {
	return strings.TrimSpace(tail(string(body), errorBodyBytes))
}

type openAIProvider struct {
	cfg      Config
	endpoint string
	client   *http.Client
}

func (p *openAIProvider) Name() string { return p.cfg.Name }

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatRequest struct {
	Model          string                `json:"model"`
	Temperature    float64               `json:"temperature"`
	MaxTokens      int                   `json:"max_tokens"`
	Messages       []openAIMessage       `json:"messages"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type openAIErrorResponse struct {
	Error *struct {
		Message string          `json:"message"`
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
	} `json:"error"`
}

func (p *openAIProvider) Complete(ctx context.Context, prompt string) (string, Usage, error) {
	name := p.cfg.Name
	var key string
	if p.cfg.APIKeyEnv != "" {
		key = os.Getenv(p.cfg.APIKeyEnv)
		if key == "" {
			return "", Usage{}, fmt.Errorf("%s: environment variable %s is not set", name, p.cfg.APIKeyEnv)
		}
	}
	header := map[string]string{}
	if key != "" {
		header["Authorization"] = "Bearer " + key
	}

	payload, err := p.payload(prompt, true)
	if err != nil {
		return "", Usage{}, err
	}
	status, body, err := postJSON(ctx, name, p.client, p.endpoint, header, payload)
	if err != nil {
		return "", Usage{}, err
	}
	if status == http.StatusBadRequest && bytes.Contains(body, []byte("response_format")) {
		payload, err = p.payload(prompt, false)
		if err != nil {
			return "", Usage{}, err
		}
		status, body, err = postJSON(ctx, name, p.client, p.endpoint, header, payload)
		if err != nil {
			return "", Usage{}, err
		}
	}
	if status < 200 || status > 299 {
		return "", Usage{}, fmt.Errorf("%s: HTTP %d: %s", name, status, Redact(openAIErrorMessage(body)))
	}

	var r openAIChatResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", Usage{}, fmt.Errorf("%s: bad JSON response: %s", name, Redact(err.Error()))
	}
	if len(r.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("%s: response had no choices", name)
	}
	u := Usage{
		InputTokens:  r.Usage.PromptTokens,
		OutputTokens: r.Usage.CompletionTokens,
		CostSource:   CostEstimated,
	}
	u.CostUSD = p.cfg.Cost(u.InputTokens, u.OutputTokens)
	return r.Choices[0].Message.Content, u, nil
}

func (p *openAIProvider) payload(prompt string, withResponseFormat bool) ([]byte, error) {
	req := openAIChatRequest{
		Model:       p.cfg.Model,
		Temperature: 0,
		MaxTokens:   maxTokens(p.cfg.MaxOutputTokens, openAIDefaultMaxOutputTokens),
		Messages: []openAIMessage{
			{Role: "system", Content: jsonSystemPrompt},
			{Role: "user", Content: prompt},
		},
	}
	if withResponseFormat {
		req.ResponseFormat = &openAIResponseFormat{Type: "json_object"}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %s", p.cfg.Name, Redact(err.Error()))
	}
	return b, nil
}

func openAIErrorMessage(body []byte) string {
	var r openAIErrorResponse
	if err := json.Unmarshal(body, &r); err == nil && r.Error != nil {
		if msg := strings.TrimSpace(r.Error.Message); msg != "" {
			return msg
		}
	}
	return errorBody(body)
}

type anthropicProvider struct {
	cfg      Config
	keyEnv   string
	endpoint string
	client   *http.Client
}

func (p *anthropicProvider) Name() string { return p.cfg.Name }

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason  string `json:"stop_reason"`
	StopDetails *struct {
		Category string `json:"category"`
	} `json:"stop_details"`
	Usage struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

type anthropicErrorResponse struct {
	Type  string `json:"type"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *anthropicProvider) Complete(ctx context.Context, prompt string) (string, Usage, error) {
	name := p.cfg.Name
	key := os.Getenv(p.keyEnv)
	if key == "" {
		return "", Usage{}, fmt.Errorf("%s: environment variable %s is not set", name, p.keyEnv)
	}
	payload, err := json.Marshal(anthropicRequest{
		Model:     p.cfg.Model,
		MaxTokens: maxTokens(p.cfg.MaxOutputTokens, anthropicDefaultMaxOutputTokens),
		System:    jsonSystemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", Usage{}, fmt.Errorf("%s: marshal request: %s", name, Redact(err.Error()))
	}
	header := map[string]string{
		"x-api-key":         key,
		"anthropic-version": anthropicVersion,
	}
	status, body, err := postJSON(ctx, name, p.client, p.endpoint, header, payload)
	if err != nil {
		return "", Usage{}, err
	}
	if status < 200 || status > 299 {
		return "", Usage{}, fmt.Errorf("%s: HTTP %d: %s", name, status, Redact(anthropicErrorMessage(body)))
	}

	var r anthropicResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", Usage{}, fmt.Errorf("%s: bad JSON response: %s", name, Redact(err.Error()))
	}
	if r.StopReason == "refusal" {
		category := ""
		if r.StopDetails != nil {
			category = strings.TrimSpace(r.StopDetails.Category)
		}
		if category != "" {
			return "", Usage{}, fmt.Errorf("%s: the model declined the request (%s)", name, category)
		}
		return "", Usage{}, fmt.Errorf("%s: the model declined the request", name)
	}
	var text strings.Builder
	sawText := false
	for _, block := range r.Content {
		if block.Type != "text" {
			continue
		}
		sawText = true
		text.WriteString(block.Text)
	}
	if !sawText {
		return "", Usage{}, fmt.Errorf("%s: response had no text", name)
	}

	in := r.Usage.InputTokens + r.Usage.CacheCreationInputTokens + r.Usage.CacheReadInputTokens
	out := r.Usage.OutputTokens
	u := Usage{
		InputTokens:  in,
		OutputTokens: out,
		CacheTokens:  r.Usage.CacheCreationInputTokens + r.Usage.CacheReadInputTokens,
		CostSource:   CostEstimated,
	}
	u.CostUSD = p.cfg.Cost(in, out)
	return text.String(), u, nil
}

func anthropicErrorMessage(body []byte) string {
	var r anthropicErrorResponse
	if err := json.Unmarshal(body, &r); err == nil && r.Error != nil {
		typ := strings.TrimSpace(r.Error.Type)
		msg := strings.TrimSpace(r.Error.Message)
		switch {
		case typ != "" && msg != "":
			return typ + ": " + msg
		case msg != "":
			return msg
		case typ != "":
			return typ
		}
	}
	return errorBody(body)
}
