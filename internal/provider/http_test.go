package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// capturedRequest is one request a test server received.
type capturedRequest struct {
	method string
	path   string
	header http.Header
	raw    []byte
}

// jsonBody decodes the captured request body as a JSON object.
func (c capturedRequest) jsonBody(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.raw, &m); err != nil {
		t.Fatalf("decode request body %q: %v", c.raw, err)
	}
	return m
}

// recorder records the requests a test server received. Handlers run in their
// own goroutines, so the mutex is not optional.
type recorder struct {
	mu   sync.Mutex
	reqs []capturedRequest
}

// newRecorder returns an empty recorder.
func newRecorder() *recorder { return &recorder{} }

// add records r. A body read failure fails the test.
func (rec *recorder) add(t *testing.T, r *http.Request) {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read request body: %v", err)
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.reqs = append(rec.reqs, capturedRequest{
		method: r.Method,
		path:   r.URL.Path,
		header: r.Header.Clone(),
		raw:    raw,
	})
}

// all returns the recorded requests in arrival order.
func (rec *recorder) all() []capturedRequest {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]capturedRequest(nil), rec.reqs...)
}

// count returns the number of recorded requests.
func (rec *recorder) count() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return len(rec.reqs)
}

// only returns the single recorded request, failing when there is not exactly
// one.
func (rec *recorder) only(t *testing.T) capturedRequest {
	t.Helper()
	reqs := rec.all()
	if len(reqs) != 1 {
		t.Fatalf("server received %d requests, want 1", len(reqs))
	}
	return reqs[0]
}

// writeJSON writes body as a JSON response with status.
func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A write error means the client went away; there is nobody left to report
	// it to, and the test asserts on what the handler recorded.
	_, _ = io.WriteString(w, body)
}

// objectList returns body[key] as a list of JSON objects.
func objectList(t *testing.T, body map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := body[key].([]any)
	if !ok {
		t.Fatalf("body[%q] = %#v, want a list of objects", key, body[key])
	}
	out := make([]map[string]any, 0, len(raw))
	for i, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("body[%q][%d] = %#v, want an object", key, i, v)
		}
		out = append(out, m)
	}
	return out
}

// numberField returns body[key] as a float64.
func numberField(t *testing.T, body map[string]any, key string) float64 {
	t.Helper()
	f, ok := body[key].(float64)
	if !ok {
		t.Fatalf("body[%q] = %#v, want a number", key, body[key])
	}
	return f
}

// stringField returns body[key] as a string.
func stringField(t *testing.T, body map[string]any, key string) string {
	t.Helper()
	s, ok := body[key].(string)
	if !ok {
		t.Fatalf("body[%q] = %#v, want a string", key, body[key])
	}
	return s
}

// sortedKeys returns the keys of body in sorted order.
func sortedKeys(body map[string]any) []string {
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// openAISuccessBody is a minimal successful /chat/completions response.
const openAISuccessBody = `{"choices":[{"message":{"content":"{\"groups\":[]}"},"finish_reason":"stop"}],` +
	`"usage":{"prompt_tokens":120,"completion_tokens":20}}`

// anthropicSuccessBody answers with a thinking block and two text blocks.
const anthropicSuccessBody = `{"content":[{"type":"thinking","thinking":"let me think","signature":"sig"},` +
	`{"type":"text","text":"{\"gro"},{"type":"text","text":"ups\":[]}"}],` +
	`"stop_reason":"end_turn","stop_details":null,` +
	`"usage":{"input_tokens":10,"output_tokens":30,"cache_creation_input_tokens":5,"cache_read_input_tokens":7}}`

func TestOpenAIRequestAndResponse(t *testing.T) {
	const key = "sk-test-1234567890abcdef"
	t.Setenv("CLUMP_TEST_OPENAI_KEY", key)

	rec := newRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(t, r)
		writeJSON(w, http.StatusOK, openAISuccessBody)
	}))
	defer srv.Close()

	cfg := Config{
		Name:      "fake",
		Type:      TypeOpenAI,
		BaseURL:   srv.URL + "/v1/", // trailing slash must not double up
		Model:     "deepseek-chat",
		APIKeyEnv: "CLUMP_TEST_OPENAI_KEY",
	}
	p := mustNew(t, cfg)

	text, u, err := p.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := `{"groups":[]}`; text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}

	req := rec.only(t)
	if req.method != http.MethodPost {
		t.Fatalf("method = %q, want POST", req.method)
	}
	if want := "/v1/chat/completions"; req.path != want {
		t.Fatalf("path = %q, want %q", req.path, want)
	}
	if got, want := req.header.Get("Authorization"), "Bearer "+key; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	if got := req.header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	body := req.jsonBody(t)
	if got, want := stringField(t, body, "model"), "deepseek-chat"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got := numberField(t, body, "temperature"); got != 0 {
		t.Fatalf("temperature = %v, want 0", got)
	}
	if got := numberField(t, body, "max_tokens"); got != openAIDefaultMaxOutputTokens {
		t.Fatalf("max_tokens = %v, want %d", got, openAIDefaultMaxOutputTokens)
	}
	msgs := objectList(t, body, "messages")
	if len(msgs) != 2 {
		t.Fatalf("messages = %#v, want 2 entries", msgs)
	}
	if got, want := stringField(t, msgs[0], "role"), "system"; got != want {
		t.Fatalf("messages[0].role = %q, want %q", got, want)
	}
	if got, want := stringField(t, msgs[0], "content"), jsonSystemPrompt; got != want {
		t.Fatalf("messages[0].content = %q, want %q", got, want)
	}
	if got, want := stringField(t, msgs[1], "role"), "user"; got != want {
		t.Fatalf("messages[1].role = %q, want %q", got, want)
	}
	if got, want := stringField(t, msgs[1], "content"), "hello"; got != want {
		t.Fatalf("messages[1].content = %q, want %q", got, want)
	}
	rf, ok := body["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("response_format = %#v, want an object", body["response_format"])
	}
	if got, want := stringField(t, rf, "type"), "json_object"; got != want {
		t.Fatalf("response_format.type = %q, want %q", got, want)
	}

	if u.InputTokens != 120 || u.OutputTokens != 20 {
		t.Fatalf("usage = %+v, want input 120 output 20", u)
	}
	if want := cfg.Cost(120, 20); u.CostUSD != want {
		t.Fatalf("CostUSD = %v, want %v", u.CostUSD, want)
	}
	if u.CostSource != CostEstimated {
		t.Fatalf("CostSource = %q, want %q", u.CostSource, CostEstimated)
	}
}

func TestOpenAINoKeyEnv(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(t, r)
		writeJSON(w, http.StatusOK, openAISuccessBody)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "ollama", Type: TypeOpenAI, BaseURL: srv.URL, Model: "llama3"})

	if _, _, err := p.Complete(context.Background(), "hello"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	req := rec.only(t)
	if got := req.header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want no header", got)
	}
}

func TestOpenAIMissingKey(t *testing.T) {
	t.Setenv("CLUMP_TEST_UNSET_KEY", "")

	rec := newRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(t, r)
		writeJSON(w, http.StatusOK, openAISuccessBody)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m",
		APIKeyEnv: "CLUMP_TEST_UNSET_KEY"})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want a missing-key error")
	}
	if want := "environment variable CLUMP_TEST_UNSET_KEY is not set"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err, want)
	}
	if got := rec.count(); got != 0 {
		t.Fatalf("server received %d requests, want 0", got)
	}
}

func TestOpenAIRetryWithoutResponseFormat(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(t, r)
		if rec.count() == 1 {
			writeJSON(w, http.StatusBadRequest, `{"error":{"message":"response_format not supported"}}`)
			return
		}
		writeJSON(w, http.StatusOK, openAISuccessBody)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m"})

	text, _, err := p.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := `{"groups":[]}`; text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
	reqs := rec.all()
	if len(reqs) != 2 {
		t.Fatalf("server received %d requests, want 2", len(reqs))
	}
	if _, ok := reqs[0].jsonBody(t)["response_format"]; !ok {
		t.Fatal("first request has no response_format, want the JSON-object hint")
	}
	second := reqs[1].jsonBody(t)
	if _, ok := second["response_format"]; ok {
		t.Fatalf("second request still has response_format: %s", reqs[1].raw)
	}
	if got, want := stringField(t, second, "model"), "m"; got != want {
		t.Fatalf("second request model = %q, want %q", got, want)
	}
	if got := numberField(t, second, "max_tokens"); got != openAIDefaultMaxOutputTokens {
		t.Fatalf("second request max_tokens = %v, want %d", got, openAIDefaultMaxOutputTokens)
	}
}

func TestOpenAIHTTPErrorParsesMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusTooManyRequests, `{"error":{"message":"slow down","type":"rate_limit","code":429}}`)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m"})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want an HTTP error")
	}
	if want := "fake: HTTP 429: slow down"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestOpenAIHTTPErrorRawBody(t *testing.T) {
	// A 1000-byte body that is not JSON: the error reports its last 300 bytes.
	raw := strings.Repeat("a", 700) + "  " + strings.Repeat("T", 298) + "  "
	if len(raw) != 1002 {
		t.Fatalf("test body is %d bytes, want 1002", len(raw))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, raw)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m"})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want an HTTP error")
	}
	if want := "fake: HTTP 500: " + strings.Repeat("T", 298); err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestOpenAIRedactsKey(t *testing.T) {
	const key = "sk-live-abcdefghijklmnop"
	t.Setenv("CLUMP_TEST_LEAK_KEY", key)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A badly behaved server echoing the credential back.
		writeJSON(w, http.StatusUnauthorized,
			`{"error":{"message":"invalid key `+r.Header.Get("Authorization")+`"}}`)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m",
		APIKeyEnv: "CLUMP_TEST_LEAK_KEY"})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want an HTTP error")
	}
	if strings.Contains(err.Error(), "abcdefghijklmnop") {
		t.Fatalf("error leaks the key: %q", err)
	}
	if !strings.Contains(err.Error(), redactedMarker) {
		t.Fatalf("error = %q, want it to contain %q", err, redactedMarker)
	}
}

func TestOpenAINoChoicesAndBadJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "no choices", body: `{"choices":[]}`, want: "fake: response had no choices"},
		{name: "bad json", body: `not json`, want: "fake: bad JSON response: "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, tc.body)
			}))
			defer srv.Close()

			p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m"})
			text, u, err := p.Complete(context.Background(), "x")
			if err == nil {
				t.Fatalf("Complete = %q, %+v, want an error", text, u)
			}
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to start with %q", err, tc.want)
			}
		})
	}
}

func TestOpenAICancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain the body so the server starts its background read, then block
		// until the client goes away.
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := p.Complete(ctx, "x")
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancelled := time.Now()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
		if waited := time.Since(cancelled); waited > 2*time.Second {
			t.Fatalf("Complete took %v after cancellation, want < 2s", waited)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Complete did not return within 2s of cancellation")
	}
}

func TestOpenAIBodyTooLarge(t *testing.T) {
	huge := strings.Repeat("x", 9<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, huge)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m"})

	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want a size error")
	}
	if want := "fake: response larger than 8 MiB"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestOpenAIConfigErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "missing base_url",
			cfg:  Config{Name: "fake", Type: TypeOpenAI, Model: "m"},
			want: "base_url is required",
		},
		{
			name: "missing model",
			cfg:  Config{Name: "fake", Type: TypeOpenAI, BaseURL: "https://example.invalid/v1"},
			want: "model is required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.cfg)
			if err == nil {
				t.Fatalf("New = %v, want an error", p)
			}
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("error = %v, want it to wrap ErrConfig", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), `provider "fake"`) {
				t.Fatalf("error = %q, want it to name the provider", err)
			}
		})
	}
}

func TestAnthropicRequestAndResponse(t *testing.T) {
	const key = "sk-ant-test-abcdefghijklmnop"
	t.Setenv("ANTHROPIC_API_KEY", key)

	rec := newRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(t, r)
		writeJSON(w, http.StatusOK, anthropicSuccessBody)
	}))
	defer srv.Close()

	cfg := Config{
		Name:        "fake",
		Type:        TypeAnthropic,
		BaseURL:     srv.URL,
		Model:       "claude-sonnet-4-6",
		InputPrice:  2,
		OutputPrice: 10,
	}
	p := mustNew(t, cfg)

	text, u, err := p.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := `{"groups":[]}`; text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}

	req := rec.only(t)
	if req.method != http.MethodPost {
		t.Fatalf("method = %q, want POST", req.method)
	}
	if want := "/v1/messages"; req.path != want {
		t.Fatalf("path = %q, want %q", req.path, want)
	}
	if got, want := req.header.Get("x-api-key"), key; got != want {
		t.Fatalf("x-api-key = %q, want %q", got, want)
	}
	if got, want := req.header.Get("anthropic-version"), "2026-10-21"; got != want {
		t.Fatalf("anthropic-version = %q, want %q", got, want)
	}
	if got := req.header.Get("content-type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content-type = %q, want application/json", got)
	}
	if got, want := req.header.Get("Authorization"), ""; got != want {
		t.Fatalf("Authorization = %q, want no header (the key goes in x-api-key)", got)
	}

	body := req.jsonBody(t)
	if got, want := sortedKeys(body), []string{"max_tokens", "messages", "model", "system"}; !slices.Equal(got, want) {
		t.Fatalf("body keys = %v, want exactly %v", got, want)
	}
	for _, banned := range []string{"temperature", "top_p", "top_k", "thinking"} {
		if _, ok := body[banned]; ok {
			t.Fatalf("body has %q, want it left out", banned)
		}
	}
	if got, want := stringField(t, body, "model"), "claude-sonnet-4-6"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got := numberField(t, body, "max_tokens"); got != anthropicDefaultMaxOutputTokens {
		t.Fatalf("max_tokens = %v, want %d", got, anthropicDefaultMaxOutputTokens)
	}
	if got, want := stringField(t, body, "system"), jsonSystemPrompt; got != want {
		t.Fatalf("system = %q, want %q", got, want)
	}
	msgs := objectList(t, body, "messages")
	if len(msgs) != 1 {
		t.Fatalf("messages = %#v, want 1 entry", msgs)
	}
	if got, want := stringField(t, msgs[0], "role"), "user"; got != want {
		t.Fatalf("messages[0].role = %q, want %q", got, want)
	}
	if got, want := stringField(t, msgs[0], "content"), "hello"; got != want {
		t.Fatalf("messages[0].content = %q, want %q", got, want)
	}

	if u.InputTokens != 22 || u.CacheTokens != 12 || u.OutputTokens != 30 {
		t.Fatalf("usage = %+v, want input 22 cache 12 output 30", u)
	}
	if want := cfg.Cost(22, 30); u.CostUSD != want {
		t.Fatalf("CostUSD = %v, want %v", u.CostUSD, want)
	}
	if u.CostSource != CostEstimated {
		t.Fatalf("CostSource = %q, want %q", u.CostSource, CostEstimated)
	}
}

func TestAnthropicDefaultBaseURL(t *testing.T) {
	p := mustNew(t, Config{Name: "fake", Type: TypeAnthropic, Model: "m"})
	ap, ok := p.(*anthropicProvider)
	if !ok {
		t.Fatalf("New returned %T, want *anthropicProvider", p)
	}
	if want := "https://api.anthropic.com/v1/messages"; ap.endpoint != want {
		t.Fatalf("endpoint = %q, want %q", ap.endpoint, want)
	}
	if want := defaultAnthropicKeyEnv; ap.keyEnv != want {
		t.Fatalf("keyEnv = %q, want %q", ap.keyEnv, want)
	}

	// A configured base URL wins, and a trailing slash does not double up.
	p = mustNew(t, Config{Name: "fake2", Type: TypeAnthropic, BaseURL: "https://proxy.example/anthropic/", Model: "m"})
	ap, ok = p.(*anthropicProvider)
	if !ok {
		t.Fatalf("New returned %T, want *anthropicProvider", p)
	}
	if want := "https://proxy.example/anthropic/v1/messages"; ap.endpoint != want {
		t.Fatalf("endpoint = %q, want %q", ap.endpoint, want)
	}
}

func TestAnthropicRefusal(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "with category",
			body: `{"content":[],"stop_reason":"refusal","stop_details":{"category":"cyber"},` +
				`"usage":{"input_tokens":5,"output_tokens":1}}`,
			want: "fake: the model declined the request (cyber)",
		},
		{
			name: "without category",
			body: `{"content":[],"stop_reason":"refusal","stop_details":{"category":""},` +
				`"usage":{"input_tokens":5,"output_tokens":1}}`,
			want: "fake: the model declined the request",
		},
		{
			name: "without stop details",
			body: `{"content":[],"stop_reason":"refusal","stop_details":null,` +
				`"usage":{"input_tokens":5,"output_tokens":1}}`,
			want: "fake: the model declined the request",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-abcdefghijklmnop")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, tc.body)
			}))
			defer srv.Close()

			p := mustNew(t, Config{Name: "fake", Type: TypeAnthropic, BaseURL: srv.URL, Model: "m"})
			text, u, err := p.Complete(context.Background(), "x")
			if err == nil {
				t.Fatalf("Complete = %q, %+v, want a refusal error", text, u)
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
		})
	}
}

func TestAnthropicNoText(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-abcdefghijklmnop")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK,
			`{"content":[{"type":"thinking","thinking":"hmm","signature":"sig"}],"stop_reason":"end_turn",`+
				`"usage":{"input_tokens":5,"output_tokens":1}}`)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeAnthropic, BaseURL: srv.URL, Model: "m"})
	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want a no-text error")
	}
	if want := "fake: response had no text"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestAnthropicErrorShape(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-abcdefghijklmnop")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized,
			`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeAnthropic, BaseURL: srv.URL, Model: "m"})
	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want an HTTP error")
	}
	if want := "fake: HTTP 401: authentication_error: invalid x-api-key"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestAnthropicMissingKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	rec := newRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(t, r)
		writeJSON(w, http.StatusOK, anthropicSuccessBody)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeAnthropic, BaseURL: srv.URL, Model: "m"})
	_, _, err := p.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("Complete succeeded, want a missing-key error")
	}
	if want := "fake: environment variable ANTHROPIC_API_KEY is not set"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
	if got := rec.count(); got != 0 {
		t.Fatalf("server received %d requests, want 0", got)
	}
}

func TestAnthropicMaxTokensOverride(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-abcdefghijklmnop")

	rec := newRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(t, r)
		writeJSON(w, http.StatusOK, anthropicSuccessBody)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeAnthropic, BaseURL: srv.URL, Model: "m",
		MaxOutputTokens: 2000})

	if _, _, err := p.Complete(context.Background(), "x"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	body := rec.only(t).jsonBody(t)
	if got, want := numberField(t, body, "max_tokens"), float64(2000); got != want {
		t.Fatalf("max_tokens = %v, want %v", got, want)
	}
}

func TestNewDispatchesHTTP(t *testing.T) {
	t.Run("openai", func(t *testing.T) {
		base := "http://127.0.0.1:1/v1"
		p := mustNew(t, Config{Name: "o", Type: TypeOpenAI, BaseURL: base, Model: "m"})
		if p == nil {
			t.Fatal("New returned a nil provider")
		}
		if got, want := p.Name(), "o"; got != want {
			t.Fatalf("Name = %q, want %q", got, want)
		}
		op, ok := p.(*openAIProvider)
		if !ok {
			t.Fatalf("New returned %T, want *openAIProvider", p)
		}
		if want := base + "/chat/completions"; op.endpoint != want {
			t.Fatalf("endpoint = %q, want %q", op.endpoint, want)
		}
	})
	t.Run("anthropic", func(t *testing.T) {
		p := mustNew(t, Config{Name: "a", Type: TypeAnthropic, BaseURL: "http://127.0.0.1:1", Model: "m"})
		if p == nil {
			t.Fatal("New returned a nil provider")
		}
		if got, want := p.Name(), "a"; got != want {
			t.Fatalf("Name = %q, want %q", got, want)
		}
		if _, ok := p.(*anthropicProvider); !ok {
			t.Fatalf("New returned %T, want *anthropicProvider", p)
		}
	})
	t.Run("openai without model", func(t *testing.T) {
		p, err := New(Config{Name: "o", Type: TypeOpenAI, BaseURL: "http://127.0.0.1:1/v1"})
		if err == nil {
			t.Fatalf("New = %v, want an error", p)
		}
		if !errors.Is(err, ErrConfig) {
			t.Fatalf("error = %v, want it to wrap ErrConfig", err)
		}
		if want := "model is required"; !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	})
}

func TestHTTPConcurrent(t *testing.T) {
	t.Setenv("CLUMP_TEST_CONCURRENT_KEY", "sk-test-concurrent-abcdefgh")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		writeJSON(w, http.StatusOK, openAISuccessBody)
	}))
	defer srv.Close()

	p := mustNew(t, Config{Name: "fake", Type: TypeOpenAI, BaseURL: srv.URL, Model: "m",
		APIKeyEnv: "CLUMP_TEST_CONCURRENT_KEY"})

	const calls = 20
	var wg sync.WaitGroup
	errs := make([]error, calls)
	texts := make([]string, calls)
	for i := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			texts[i], _, errs[i] = p.Complete(context.Background(), "hello")
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if want := `{"groups":[]}`; texts[i] != want {
			t.Fatalf("call %d text = %q, want %q", i, texts[i], want)
		}
	}
}
