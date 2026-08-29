package acp

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/halqme/kei/internal/agent"
	agentcontext "github.com/halqme/kei/internal/context"
	"github.com/halqme/kei/internal/provider"
	"github.com/halqme/kei/internal/session"
	"github.com/halqme/kei/internal/tool"
)

type streamingTestProvider struct{}

func (streamingTestProvider) Generate(_ context.Context, _ agentcontext.Request, callback provider.StreamCallback) (provider.Result, error) {
	if callback != nil {
		callback(provider.StreamEvent{Type: provider.StreamEventTextDelta, Text: "hello "})
		callback(provider.StreamEvent{Type: provider.StreamEventTextDelta, Text: "world"})
	}
	return provider.Result{
		Message:      provider.Message{Role: "assistant", Content: "hello world"},
		FinishReason: "completed",
	}, nil
}

func TestSessionPromptForwardsStreamChunksToACP(t *testing.T) {
	tools, err := tool.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	s := NewServer(strings.NewReader(""), &out, func(id, cwd string) (*agent.Runtime, error) {
		return &agent.Runtime{
			State:    &session.State{ID: id, Workspace: cwd},
			Provider: streamingTestProvider{},
			Tools:    tools,
		}, nil
	})

	s.handle(t.Context(), request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "session/new",
		Params:  jsontext.Value(`{"cwd":"/tmp"}`),
	})
	s.handle(t.Context(), request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "session/prompt",
		Params:  jsontext.Value(`{"sessionId":"kei-1","prompt":[{"type":"text","text":"Say hello"}]}`),
	})

	var chunks []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var message struct {
			Params struct {
				Update struct {
					SessionUpdate string `json:"sessionUpdate"`
					Content       struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"update"`
			} `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("invalid JSON-RPC output %q: %v", line, err)
		}
		if message.Params.Update.SessionUpdate == "agent_message_chunk" {
			chunks = append(chunks, message.Params.Update.Content.Text)
		}
	}

	if got := strings.Join(chunks, ""); got != "hello world" {
		t.Fatalf("unexpected streamed chunks: %q", got)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected two streamed chunks, got %d", len(chunks))
	}
	if strings.Contains(out.String(), `"text":"hello world"`) {
		t.Fatal("final response was duplicated after streaming chunks")
	}
}
