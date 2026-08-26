package provider

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-gemini-key" {
			t.Errorf("unexpected api key: %s", r.URL.Query().Get("key"))
		}

		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"finishReason": "STOP",
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{
								"text": "Looking up details...",
							},
							{
								"functionCall": map[string]any{
									"name": "unix_search_text",
									"args": map[string]any{"pattern": "gemini"},
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, resp)
	}))
	defer server.Close()

	gemini := &Gemini{
		BaseURL: server.URL,
		APIKey:  "test-gemini-key",
		Model:   "gemini-2.5-flash",
		Client:  server.Client(),
	}

	messages := []Message{
		{Role: "user", Content: "Search for gemini"},
	}
	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "unix_search_text",
				"description": "search",
				"parameters":  map[string]any{"type": "object"},
			},
		},
	}

	var streamed string
	res, err := gemini.Stream(t.Context(), messages, tools, func(event StreamEvent) {
		streamed += event.Text
	})
	if err != nil {
		t.Fatalf("Gemini.Stream failed: %v", err)
	}
	if streamed != "Looking up details..." {
		t.Errorf("unexpected streamed text: %q", streamed)
	}

	if res.FinishReason != "STOP" {
		t.Errorf("expected finish reason STOP, got %q", res.FinishReason)
	}
	if res.Message.Content != "Looking up details..." {
		t.Errorf("unexpected content: %q", res.Message.Content)
	}
	if len(res.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(res.Message.ToolCalls))
	}
	tc := res.Message.ToolCalls[0]
	if tc.Function.Name != "unix_search_text" || tc.Function.Arguments != `{"pattern":"gemini"}` {
		t.Errorf("unexpected tool call: %+v", tc)
	}
}
