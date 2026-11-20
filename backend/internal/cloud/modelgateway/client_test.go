package modelgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientUsesFixedChatEndpointAndServerBearerKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer server-key" {
			t.Fatalf("unexpected gateway request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var request Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "upstream-model" {
			t.Fatalf("unexpected server-controlled request: %+v err=%v", request, err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": "ok"}}},
			"usage":   map[string]int64{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer server.Close()
	client, err := New(server.URL, "server-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Call(context.Background(), "upstream-model", Request{Model: "", Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil || result.Text != "ok" || result.Usage.TotalTokens != 5 {
		t.Fatalf("gateway call result: %+v err=%v", result, err)
	}
}

func TestClientRejectsUntrustedModelAndInvalidUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": "bad"}}},
			"usage":   map[string]int64{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 99},
		})
	}))
	defer server.Close()
	client, err := New(server.URL, "server-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(context.Background(), "upstream-model", Request{Model: "client-model"}); err == nil {
		t.Fatal("client-controlled model was accepted")
	}
	if _, err := client.Call(context.Background(), "upstream-model", Request{}); err == nil {
		t.Fatal("invalid usage response was accepted")
	}
}
