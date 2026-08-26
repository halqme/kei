package provider

import "context"

type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Result struct {
	Message      Message
	FinishReason string
}

type Provider interface {
	Stream(ctx context.Context, messages []Message, tools []map[string]any, callback StreamCallback) (Result, error)
}

type StreamEvent struct {
	Type string
	Text string
}

const StreamEventTextDelta = "text_delta"

type StreamCallback func(StreamEvent)
