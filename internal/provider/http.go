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

// Limits and defaults shared by the two HTTP providers.
const (
	// maxResponseBytes caps how much of a response body a provider reads.
	maxResponseBytes = 8 << 20
	// defaultHTTPTimeoutSec bounds one call when Config.TimeoutSec is <= 0.
	defaultHTTPTimeoutSec = 180
	// openAIDefaultMaxOutputTokens is max_tokens when Config.MaxOutputTokens
	// is <= 0 for the OpenAI-compatible provider.
	openAIDefaultMaxOutputTokens = 4096
	// anthropicDefaultMaxOutputTokens is max_tokens when Config.MaxOutputTokens
	// is <= 0 for the Anthropic provider. Current models think before they
	// answer and those tokens count against the same budget.
	anthropicDefaultMaxOutputTokens = 16000
	// errorBodyBytes is how many trailing bytes of an error body that does not
	// parse as the API's error shape are reported.
	errorBodyBytes = 300
	// jsonSystemPrompt is the system message both providers send.
	jsonSystemPrompt = "You are a JSON API. Follow the user message exactly and output only JSON."
	// anthropicVersion is the Messages API version the Anthropic provider
	// requests. That version rejects sampling parameters (temperature, top_p,
	// top_k) and leaves thinking at the model's default.
	anthropicVersion = "2026-10-21"
	// defaultAnthropicBaseURL is used when Config.BaseURL is empty.
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	// defaultAnthropicKeyEnv is used when Config.APIKeyEnv is empty.
	defaultAnthropicKeyEnv = "ANTHROPIC_API_KEY"
)

// newOpenAI builds the OpenAI-compatible provider (TypeOpenAI). It posts to
// BaseURL + "/chat/completions" with a Bearer key from Config.APIKeyEnv when
// that is set, so a local server (Ollama) needs no key.
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

// newAnthropic builds the Anthropic Messages provider (TypeAnthropic). It posts
// to BaseURL (default https://api.anthropic.com) + "/v1/messages" with the key
// from Config.APIKeyEnv (default ANTHROPIC_API_KEY).
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

// newHTTPClient is the client both HTTP providers use: one call is bounded by
// Config.TimeoutSec seconds (180 by default). Cancelling ctx aborts the call.
func newHTTPClient(c Config) *http.Client {
	sec := c.TimeoutSec
	if sec <= 0 {
		sec = defaultHTTPTimeoutSec
	}
	return &http.Client{Timeout: time.Duration(sec) * time.Second}
}

// maxTokens returns n, or def when n <= 0.
func maxTokens(n, def int) int {
	if n <= 0 {
		return def
	}
	return n
}

// postJSON sends payload to endpoint as a JSON POST, adds header, and returns
// the status code with at most maxResponseBytes of the body. Errors are
// name-prefixed and pass through Redact; a cancelled ctx is returned wrapped so
// that errors.Is(err, context.Canceled) holds.
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

// httpCallError turns a transport error into a provider error: the context's
// error when the caller cancelled or timed out, otherwise the redacted message.
func httpCallError(ctx context.Context, name string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", name, ctxErr)
	}
	return fmt.Errorf("%s: %s", name, Redact(err.Error()))
}

// errorBody reports an error body that does not parse as the API's error shape:
// its last errorBodyBytes bytes, trimmed.
func errorBody(body []byte) string {
	return strings.TrimSpace(tail(string(body), errorBodyBytes))
}

// openAIProvider speaks the OpenAI-compatible /chat/completions API used by
// DeepSeek, OpenRouter, Ollama and LiteLLM. It is safe for concurrent use.
type openAIProvider struct {
	cfg      Config
	endpoint string // BaseURL + "/chat/completions"
	client   *http.Client
}

// Name returns the configured provider name.
func (p *openAIProvider) Name() string { return p.cfg.Name }

// openAIMessage is one entry of the chat request's messages array.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponseFormat asks the server for a JSON object. It is a pointer so
// the retry for servers that reject the field can leave it out entirely.
type openAIResponseFormat struct {
	Type string `json:"type"`
}

// openAIChatRequest is the /chat/completions request body.
type openAIChatRequest struct {
	Model          string                `json:"model"`
	Temperature    float64               `json:"temperature"`
	MaxTokens      int                   `json:"max_tokens"`
	Messages       []openAIMessage       `json:"messages"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
}

// openAIChatResponse is the part of a /chat/completions response we read.
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

// openAIErrorResponse is the API's error shape.
type openAIErrorResponse struct {
	Error *struct {
		Message string          `json:"message"`
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
	} `json:"error"`
}

// Complete sends prompt and returns the first choice's content.
func (p *openAIProvider) Complete(ctx context.Context, prompt string) (string, Usage, error) {
	name := p.cfg.Name
	// The key is read at call time: a config naming an unset env var still
	// loads, and a key exported later still works.
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
		// Some OpenAI-compatible servers reject response_format; retry the same
		// request once without it and report that attempt.
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

// payload marshals the chat request. withResponseFormat false builds the retry
// for servers that reject the field.
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

// openAIErrorMessage is the message of an OpenAI error body, or the trimmed
// tail of a body that does not parse as one.
func openAIErrorMessage(body []byte) string {
	var r openAIErrorResponse
	if err := json.Unmarshal(body, &r); err == nil && r.Error != nil {
		if msg := strings.TrimSpace(r.Error.Message); msg != "" {
			return msg
		}
	}
	return errorBody(body)
}

// anthropicProvider speaks the Anthropic Messages API. It is safe for
// concurrent use.
type anthropicProvider struct {
	cfg      Config
	keyEnv   string // env var holding the key; never empty
	endpoint string // BaseURL + "/v1/messages"
	client   *http.Client
}

// Name returns the configured provider name.
func (p *anthropicProvider) Name() string { return p.cfg.Name }

// anthropicMessage is one entry of the messages array.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicRequest is the /v1/messages request body. It carries no sampling
// parameters and no thinking field: the API version rejects the former and the
// latter stays at the model's default.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

// anthropicResponse is the part of a /v1/messages response we read.
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

// anthropicErrorResponse is the API's error shape.
type anthropicErrorResponse struct {
	Type  string `json:"type"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends prompt and returns the concatenated text blocks.
func (p *anthropicProvider) Complete(ctx context.Context, prompt string) (string, Usage, error) {
	name := p.cfg.Name
	// The key is read at call time and never put in a URL, a log or an error.
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
			continue // thinking and other block types are not the answer
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

// anthropicErrorMessage is the message of an Anthropic error body
// ("<error.type>: <error.message>"), or the trimmed tail of a body that does not
// parse as one.
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
