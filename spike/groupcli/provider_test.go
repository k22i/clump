package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIProviderAgainstFake(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "nope", 404)
			return
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": `{"groups":[{"title":"x","hunks":["h1"]}]}`}}},
			"usage":   map[string]any{"prompt_tokens": 120, "completion_tokens": 20},
		})
	}))
	defer srv.Close()
	t.Setenv("FAKE_KEY", "sk-test-1234567890abcdef")
	p := &openAIProvider{name: "fake", baseURL: srv.URL + "/v1", model: "m", keyEnv: "FAKE_KEY", inPer: 0.27, outPer: 1.1}
	text, u, err := p.Complete(context.Background(), "hello")
	if err != nil || !strings.Contains(text, "groups") || u.InputTokens != 120 || gotAuth != "Bearer sk-test-1234567890abcdef" {
		t.Fatalf("text=%q u=%+v err=%v auth=%q", text, u, err, gotAuth)
	}
}

func TestOpenAIProviderRedactsKeyInErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid key "+r.Header.Get("Authorization"), 401)
	}))
	defer srv.Close()
	t.Setenv("OPENAI_API_KEY", "sk-live-abcdefghijklmnop")
	p := &openAIProvider{name: "fake", baseURL: srv.URL, model: "m", keyEnv: "OPENAI_API_KEY"}
	_, _, err := p.Complete(context.Background(), "x")
	if err == nil || strings.Contains(err.Error(), "abcdefghijklmnop") || !strings.Contains(err.Error(), "REDACTED") {
		t.Fatalf("err=%v", err)
	}
}

func TestCommandProviderRaw(t *testing.T) {
	p := &commandProvider{name: "cat", argv: []string{"cat"}, format: "raw"}
	text, _, err := p.Complete(context.Background(), `{"groups":[]}`)
	if err != nil || text != `{"groups":[]}` {
		t.Fatal(text, err)
	}
}
