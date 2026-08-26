package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-anthropic-key" {
			t.Errorf("unexpected api key: %s", r.Header.Get("x-api-key"))
		}

		resp := map[string]any{
			"id":          "msg_123",
			"stop_reason": "tool_use",
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Searching for pattern...",
				},
				{
					"type":  "tool_use",
					"id":    "tool_use_1",
					"name":  "unix_search_text",
					"input": map[string]any{"pattern": "foo"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	anthropic := &Anthropic{
		BaseURL: server.URL,
		APIKey:  "test-anthropic-key",
		Model:   "claude-3-7-sonnet-20250219",
		Client:  server.Client(),
	}

	messages := []Message{
		{Role: "system", Content: "You are Claude."},
		{Role: "user", Content: "Search for foo."},
	}
	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "unix_search_text",
				"description": "search text",
				"parameters":  map[string]any{"type": "object"},
			},
		},
	}

	var streamed string
	res, err := anthropic.Stream(t.Context(), messages, tools, func(event StreamEvent) {
		streamed += event.Text
	})
	if err != nil {
		t.Fatalf("Anthropic.Stream failed: %v", err)
	}
	if streamed != "Searching for pattern..." {
		t.Errorf("unexpected streamed text: %q", streamed)
	}

	if res.FinishReason != "tool_use" {
		t.Errorf("expected finish reason tool_use, got %q", res.FinishReason)
	}
	if res.Message.Content != "Searching for pattern..." {
		t.Errorf("unexpected content: %q", res.Message.Content)
	}
	if len(res.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(res.Message.ToolCalls))
	}
	tc := res.Message.ToolCalls[0]
	if tc.ID != "tool_use_1" || tc.Function.Name != "unix_search_text" || tc.Function.Arguments != `{"pattern":"foo"}` {
		t.Errorf("unexpected tool call: %+v", tc)
	}
}
