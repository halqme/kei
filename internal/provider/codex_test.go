package provider

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatResponsesInput(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are a coding assistant."},
		{Role: "user", Content: "Find files in the directory."},
		{
			Role:    "assistant",
			Content: "Let me search for files.",
			ToolCalls: []ToolCall{
				{
					ID:   "call_abc123",
					Type: "function",
					Function: FunctionCall{
						Name:      "unix_search_text",
						Arguments: `{"pattern":"test"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "call_abc123",
			Content:    "file1.go\nfile2.go",
		},
	}

	instructions, input := formatResponsesInput(messages)
	if instructions != "You are a coding assistant." {
		t.Errorf("expected instructions %q, got %q", "You are a coding assistant.", instructions)
	}

	if len(input) != 4 {
		t.Fatalf("expected 4 input items, got %d: %+v", len(input), input)
	}

	// 1. User message
	if input[0]["type"] != "message" || input[0]["role"] != "user" {
		t.Errorf("unexpected input[0]: %+v", input[0])
	}

	// 2. Assistant function_call
	if input[1]["type"] != "function_call" || input[1]["call_id"] != "call_abc123" {
		t.Errorf("unexpected input[1]: %+v", input[1])
	}

	// 3. Assistant text message
	if input[2]["type"] != "message" || input[2]["role"] != "assistant" {
		t.Errorf("unexpected input[2]: %+v", input[2])
	}

	// 4. Function call output
	if input[3]["type"] != "function_call_output" || input[3]["call_id"] != "call_abc123" {
		t.Errorf("unexpected input[3]: %+v", input[3])
	}
}

func TestFormatResponsesTools(t *testing.T) {
	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "search_tool",
				"description": "Searches for items",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}

	formatted := formatResponsesTools(tools)
	if len(formatted) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(formatted))
	}
	if formatted[0]["name"] != "search_tool" || formatted[0]["description"] != "Searches for items" {
		t.Errorf("unexpected formatted tool: %+v", formatted[0])
	}
}

func TestCodexStream(t *testing.T) {
	var receivedAuth string
	var receivedAccountID string
	var receivedPath string
	var receivedStream bool
	var receivedAccept string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")
		receivedAccountID = r.Header.Get("chatgpt-account-id")
		receivedAccept = r.Header.Get("Accept")

		var requestBody map[string]any
		if err := json.UnmarshalRead(r.Body, &requestBody); err == nil {
			receivedStream, _ = requestBody["stream"].(bool)
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(`event: response.created
data: {"type":"response.created","response":{"id":"resp_123"}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_123","output_index":0,"delta":"I will execute the "}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_123","output_index":0,"delta":"tool now."}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"msg_123","type":"message","role":"assistant","content":[{"type":"output_text","text":"I will execute the tool now."}]}}

event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"fc_999","type":"function_call","call_id":"call_999","name":"unix_search_text","arguments":""}}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"fc_999","call_id":"call_999","delta":"{\"pattern\":\"hello\"}"}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"fc_999","type":"function_call","call_id":"call_999","name":"unix_search_text","arguments":"{\"pattern\":\"hello\"}"}}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_123","status":"completed","output":null}}

`))
	}))
	defer server.Close()

	codex := &Codex{
		BaseURL:     server.URL,
		Model:       "gpt-5.5",
		StaticToken: "test-secret-token",
		AccountID:   "org_acc_123",
		Client:      server.Client(),
	}

	messages := []Message{
		{Role: "user", Content: "Search for hello"},
	}
	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "unix_search_text",
				"description": "search",
			},
		},
	}

	var streamed string
	res, err := codex.Stream(t.Context(), messages, tools, func(event StreamEvent) {
		streamed += event.Text
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	if streamed != "I will execute the tool now." {
		t.Errorf("unexpected streamed text: %q", streamed)
	}

	if receivedPath != "/responses" {
		t.Errorf("expected path /responses, got %q", receivedPath)
	}
	if receivedAuth != "Bearer test-secret-token" {
		t.Errorf("expected auth Bearer test-secret-token, got %q", receivedAuth)
	}
	if receivedAccountID != "org_acc_123" {
		t.Errorf("expected account ID org_acc_123, got %q", receivedAccountID)
	}
	if !receivedStream {
		t.Error("expected stream request flag to be true")
	}
	if receivedAccept != "text/event-stream" {
		t.Errorf("expected Accept text/event-stream, got %q", receivedAccept)
	}

	if res.FinishReason != "completed" {
		t.Errorf("expected finish reason completed, got %q", res.FinishReason)
	}
	if res.Message.Content != "I will execute the tool now." {
		t.Errorf("unexpected content: %q", res.Message.Content)
	}
	if len(res.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(res.Message.ToolCalls))
	}
	tc := res.Message.ToolCalls[0]
	if tc.ID != "call_999" || tc.Function.Name != "unix_search_text" || tc.Function.Arguments != `{"pattern":"hello"}` {
		t.Errorf("unexpected tool call: %+v", tc)
	}
}

func codexSSE(events ...map[string]any) string {
	var stream strings.Builder
	for _, event := range events {
		payload, _ := json.Marshal(event)
		kind, _ := event["type"].(string)
		fmt.Fprintf(&stream, "event: %s\r\ndata: %s\r\n\r\n", kind, payload)
	}
	return stream.String()
}

func TestReadCodexStreamAggregatesItemsAndEmitsDeltas(t *testing.T) {
	var chunks []string
	result, err := readCodexStream(strings.NewReader(codexSSE(
		map[string]any{
			"type": "response.output_item.added",
			"item": map[string]any{"id": "msg_1", "type": "message", "role": "assistant"},
		},
		map[string]any{
			"type":         "response.output_text.delta",
			"item_id":      "msg_1",
			"output_index": 0,
			"delta":        "ha",
		},
		map[string]any{
			"type":         "response.output_text.delta",
			"item_id":      "msg_1",
			"output_index": 0,
			"delta":        "ha",
		},
		map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"id":   "msg_1",
				"type": "message",
				"content": []map[string]any{
					{"type": "output_text", "text": "haha"},
				},
			},
		},
		map[string]any{
			"type": "response.output_item.added",
			"item": map[string]any{
				"id": "fc_1", "type": "function_call", "call_id": "call_1", "name": "search",
			},
		},
		map[string]any{
			"type":    "response.function_call_arguments.delta",
			"item_id": "fc_1", "call_id": "call_1", "delta": `{"pattern":`,
		},
		map[string]any{
			"type":    "response.function_call_arguments.delta",
			"item_id": "fc_1", "call_id": "call_1", "delta": `"hello"}`,
		},
		map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"id": "fc_1", "type": "function_call", "call_id": "call_1", "name": "search",
				"arguments": `{"pattern":"hello"}`,
			},
		},
		map[string]any{
			"type":     "response.completed",
			"response": map[string]any{"id": "resp_1", "status": "completed", "output": nil},
		},
	)), func(event StreamEvent) {
		if event.Type == StreamEventTextDelta {
			chunks = append(chunks, event.Text)
		}
	})
	if err != nil {
		t.Fatalf("readCodexStream failed: %v", err)
	}

	if got := strings.Join(chunks, ""); got != "haha" {
		t.Errorf("unexpected streamed text: %q", got)
	}
	if result.Message.Content != "haha" {
		t.Errorf("unexpected result content: %q", result.Message.Content)
	}
	if len(result.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(result.Message.ToolCalls))
	}
	call := result.Message.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "search" || call.Function.Arguments != `{"pattern":"hello"}` {
		t.Errorf("unexpected tool call: %+v", call)
	}
}

func TestReadCodexStreamHandlesMultilineSSE(t *testing.T) {
	stream := ": keepalive\r\n\r\nevent: response.created\r\ndata: {\"type\":\"response.created\",\r\ndata: \"response\":{\"id\":\"resp_1\"}}\r\n\r\nevent: response.completed\r\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\r\n\r\n"
	result, err := readCodexStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("readCodexStream failed: %v", err)
	}
	if result.FinishReason != "completed" {
		t.Errorf("expected completed finish reason, got %q", result.FinishReason)
	}
}

func TestReadCodexStreamRejectsBrokenTerminalPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "failed",
			input: codexSSE(map[string]any{
				"type": "response.failed",
				"response": map[string]any{
					"error": map[string]any{"message": "model failed"},
				},
			}),
			want: "model failed",
		},
		{
			name: "incomplete",
			input: codexSSE(map[string]any{
				"type": "response.incomplete",
				"response": map[string]any{
					"incomplete_details": map[string]any{"reason": "max_output_tokens"},
				},
			}),
			want: "max_output_tokens",
		},
		{
			name:  "premature EOF",
			input: codexSSE(map[string]any{"type": "response.output_text.delta", "delta": "partial"}),
			want:  "ended before completion",
		},
		{
			name:  "malformed event",
			input: "event: response.completed\ndata: not-json\n\n",
			want:  "invalid event",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readCodexStream(strings.NewReader(test.input), nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestReadCodexStreamHandlesLargeArguments(t *testing.T) {
	arguments := strings.Repeat("x", 128<<10)
	result, err := readCodexStream(strings.NewReader(codexSSE(
		map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"id": "fc_large", "type": "function_call", "name": "write", "arguments": arguments,
			},
		},
		map[string]any{
			"type":     "response.completed",
			"response": map[string]any{"status": "completed"},
		},
	)), nil)
	if err != nil {
		t.Fatalf("readCodexStream failed: %v", err)
	}
	if len(result.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(result.Message.ToolCalls))
	}
	if got := result.Message.ToolCalls[0].Function.Arguments; got != arguments {
		t.Errorf("large arguments changed: got %d bytes, want %d", len(got), len(arguments))
	}
}

func TestReadCodexStreamDoesNotDuplicateCompletedOutput(t *testing.T) {
	result, err := readCodexStream(strings.NewReader(codexSSE(
		map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"id": "msg_1", "type": "message",
				"content": []map[string]any{{"type": "output_text", "text": "hello"}},
			},
		},
		map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"status": "completed",
				"output": []map[string]any{{
					"id": "msg_1", "type": "message",
					"content": []map[string]any{{"type": "output_text", "text": "hello"}},
				}},
			},
		},
	)), nil)
	if err != nil {
		t.Fatalf("readCodexStream failed: %v", err)
	}
	if result.Message.Content != "hello" {
		t.Errorf("completed output was duplicated: %q", result.Message.Content)
	}
}
