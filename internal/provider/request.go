package provider

import (
	"context"

	agentcontext "github.com/halqme/kei/internal/context"
)

func (p *Anthropic) Generate(ctx context.Context, request agentcontext.Request, callback StreamCallback) (Result, error) {
	return p.Stream(ctx, requestMessages(request), request.Tools, callback)
}

func (p *Codex) Generate(ctx context.Context, request agentcontext.Request, callback StreamCallback) (Result, error) {
	return p.Stream(ctx, requestMessages(request), request.Tools, callback)
}

func (p *Gemini) Generate(ctx context.Context, request agentcontext.Request, callback StreamCallback) (Result, error) {
	return p.Stream(ctx, requestMessages(request), request.Tools, callback)
}

func (p *OpenAICompatible) Generate(ctx context.Context, request agentcontext.Request, callback StreamCallback) (Result, error) {
	return p.Stream(ctx, requestMessages(request), request.Tools, callback)
}

func requestMessages(request agentcontext.Request) []Message {
	messages := make([]Message, 0, len(request.Tail)+1)
	if request.Instructions != "" {
		messages = append(messages, Message{Role: "system", Content: request.Instructions})
	}
	for _, entry := range request.Tail {
		message := Message{
			Role:       string(entry.Role),
			Content:    entry.Content,
			ToolCallID: entry.ToolCallID,
		}
		for _, call := range entry.ToolCalls {
			message.ToolCalls = append(message.ToolCalls, ToolCall{
				ID:   call.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      call.Name,
					Arguments: call.Arguments,
				},
			})
		}
		messages = append(messages, message)
	}
	return messages
}
