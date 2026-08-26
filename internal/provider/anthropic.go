package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	AnthropicDefaultBaseURL = "https://api.anthropic.com/v1"
	AnthropicDefaultModel   = "claude-3-7-sonnet-20250219"
	AnthropicAPIVersion     = "2023-06-01"
)

type Anthropic struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

func (p *Anthropic) Stream(ctx context.Context, messages []Message, tools []map[string]any, callback StreamCallback) (Result, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}

	apiKey := p.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return Result{}, fmt.Errorf("anthropic provider: ANTHROPIC_API_KEY is not set")
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = AnthropicDefaultBaseURL
	}
	model := p.Model
	if model == "" {
		model = AnthropicDefaultModel
	}

	systemPrompt, formattedMsgs := formatAnthropicMessages(messages)

	body := map[string]any{
		"model":      model,
		"messages":   formattedMsgs,
		"max_tokens": 4096,
	}
	if systemPrompt != "" {
		body["system"] = systemPrompt
	}
	if len(tools) > 0 {
		body["tools"] = formatAnthropicTools(tools)
	}

	reqBytes, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", AnthropicAPIVersion)

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(resp.Body)
		var errObj any
		_ = json.Unmarshal(respBody, &errObj)
		return Result{}, fmt.Errorf("anthropic provider returned %s: %v", resp.Status, errObj)
	}

	var raw struct {
		ID         string `json:"id"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text,omitempty"`
			ID    string         `json:"id,omitempty"`
			Name  string         `json:"name,omitempty"`
			Input map[string]any `json:"input,omitempty"`
		} `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Result{}, err
	}

	var textParts []string
	var toolCalls []ToolCall

	for _, item := range raw.Content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				textParts = append(textParts, item.Text)
			}
		case "tool_use":
			argsBytes, _ := json.Marshal(item.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:   item.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      item.Name,
					Arguments: string(argsBytes),
				},
			})
		}
	}

	text := strings.Join(textParts, "\n")
	result := Result{
		Message: Message{
			Role:      "assistant",
			Content:   text,
			ToolCalls: toolCalls,
		},
		FinishReason: raw.StopReason,
	}
	if callback != nil && text != "" {
		callback(StreamEvent{Type: StreamEventTextDelta, Text: text})
	}
	return result, nil
}

func formatAnthropicMessages(messages []Message) (string, []map[string]any) {
	var system string
	var out []map[string]any

	for i, m := range messages {
		if m.Role == "system" {
			if i == 0 && system == "" {
				system = contentToString(m.Content)
				continue
			}
			// Subsequent system messages can be appended or passed as user context
			if system != "" {
				system += "\n\n" + contentToString(m.Content)
			} else {
				system = contentToString(m.Content)
			}
			continue
		}

		if m.Role == "user" {
			out = append(out, map[string]any{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": contentToString(m.Content)},
				},
			})
			continue
		}

		if m.Role == "assistant" {
			var blocks []map[string]any
			if text := contentToString(m.Content); text != "" {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": text,
				})
			}
			for _, tc := range m.ToolCalls {
				var input map[string]any
				if strings.TrimSpace(tc.Function.Arguments) != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				}
				if input == nil {
					input = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			if len(blocks) > 0 {
				out = append(out, map[string]any{
					"role":    "assistant",
					"content": blocks,
				})
			}
			continue
		}

		if m.Role == "tool" {
			out = append(out, map[string]any{
				"role": "user",
				"content": []map[string]any{
					{
						"type":        "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":     contentToString(m.Content),
					},
				},
			})
			continue
		}
	}

	return system, out
}

func formatAnthropicTools(tools []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		if fn, ok := t["function"].(map[string]any); ok {
			out = append(out, map[string]any{
				"name":         fn["name"],
				"description":  fn["description"],
				"input_schema": fn["parameters"],
			})
		} else {
			out = append(out, map[string]any{
				"name":         t["name"],
				"description":  t["description"],
				"input_schema": t["parameters"],
			})
		}
	}
	return out
}
