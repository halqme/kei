package provider

import (
	"testing"

	agentcontext "github.com/halqme/kei/internal/context"
	"github.com/halqme/kei/internal/transcript"
)

func TestRequestMessagesRendersContextForMessageTransports(t *testing.T) {
	request := agentcontext.Request{
		Instructions: "base",
		Tail: []transcript.Entry{
			{Role: transcript.RoleUser, Content: "hello"},
			{
				Role: transcript.RoleAssistant,
				ToolCalls: []transcript.ToolCall{{
					ID:        "call-1",
					Name:      "search",
					Arguments: `{"query":"hello"}`,
				}},
			},
			{Role: transcript.RoleTool, ToolCallID: "call-1", Content: "result"},
		},
	}

	messages := requestMessages(request)
	if len(messages) != 4 {
		t.Fatalf("got %d messages, want 4: %+v", len(messages), messages)
	}
	if messages[0].Role != "system" || messages[0].Content != "base" {
		t.Fatalf("unexpected system message: %+v", messages[0])
	}
	if messages[1].Role != "user" || messages[1].Content != "hello" {
		t.Fatalf("unexpected user message: %+v", messages[1])
	}
	if len(messages[2].ToolCalls) != 1 {
		t.Fatalf("unexpected assistant tool calls: %+v", messages[2])
	}
	call := messages[2].ToolCalls[0]
	if call.ID != "call-1" || call.Type != "function" || call.Function.Name != "search" || call.Function.Arguments != `{"query":"hello"}` {
		t.Fatalf("unexpected provider tool call: %+v", call)
	}
	if messages[3].Role != "tool" || messages[3].ToolCallID != "call-1" || messages[3].Content != "result" {
		t.Fatalf("unexpected tool message: %+v", messages[3])
	}
}
