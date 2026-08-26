package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	GeminiDefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	GeminiDefaultModel   = "gemini-2.5-flash"
)

type Gemini struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

func (p *Gemini) Stream(ctx context.Context, messages []Message, tools []map[string]any, callback StreamCallback) (Result, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}

	apiKey := p.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("GOOGLE_API_KEY")
		}
	}
	if apiKey == "" {
		return Result{}, fmt.Errorf("gemini provider: GEMINI_API_KEY (or GOOGLE_API_KEY) is not set")
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = GeminiDefaultBaseURL
	}
	model := p.Model
	if model == "" {
		model = GeminiDefaultModel
	}

	systemInstruction, contents := formatGeminiContents(messages)

	body := map[string]any{
		"contents": contents,
	}
	if systemInstruction != nil {
		body["systemInstruction"] = systemInstruction
	}
	if len(tools) > 0 {
		body["tools"] = []map[string]any{
			{"functionDeclarations": formatGeminiTools(tools)},
		}
	}

	reqBytes, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", strings.TrimRight(baseURL, "/"), model, url.QueryEscape(apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(resp.Body)
		var errObj any
		_ = json.Unmarshal(respBody, &errObj)
		return Result{}, fmt.Errorf("gemini provider returned %s: %v", resp.Status, errObj)
	}

	var raw struct {
		Candidates []struct {
			Content struct {
				Role  string `json:"role"`
				Parts []struct {
					Text         string `json:"text,omitempty"`
					FunctionCall *struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args"`
					} `json:"functionCall,omitempty"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Result{}, err
	}

	if len(raw.Candidates) == 0 {
		return Result{}, fmt.Errorf("gemini provider returned no candidates")
	}

	cand := raw.Candidates[0]
	var textParts []string
	var toolCalls []ToolCall

	for i, part := range cand.Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			argsBytes, _ := json.Marshal(part.FunctionCall.Args)
			toolCalls = append(toolCalls, ToolCall{
				ID:   fmt.Sprintf("call_%d", i+1),
				Type: "function",
				Function: FunctionCall{
					Name:      part.FunctionCall.Name,
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
		FinishReason: cand.FinishReason,
	}
	if callback != nil && text != "" {
		callback(StreamEvent{Type: StreamEventTextDelta, Text: text})
	}
	return result, nil
}

func formatGeminiContents(messages []Message) (map[string]any, []map[string]any) {
	var systemInstruction map[string]any
	var contents []map[string]any

	for i, m := range messages {
		if m.Role == "system" {
			if i == 0 && systemInstruction == nil {
				systemInstruction = map[string]any{
					"parts": []map[string]any{
						{"text": contentToString(m.Content)},
					},
				}
				continue
			}
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{"text": contentToString(m.Content)},
				},
			})
			continue
		}

		if m.Role == "user" {
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{"text": contentToString(m.Content)},
				},
			})
			continue
		}

		if m.Role == "assistant" {
			var parts []map[string]any
			if text := contentToString(m.Content); text != "" {
				parts = append(parts, map[string]any{"text": text})
			}
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if strings.TrimSpace(tc.Function.Arguments) != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				}
				if args == nil {
					args = map[string]any{}
				}
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": tc.Function.Name,
						"args": args,
					},
				})
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]any{
					"role":  "model",
					"parts": parts,
				})
			}
			continue
		}

		if m.Role == "tool" {
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{
						"functionResponse": map[string]any{
							"name":     m.ToolCallID,
							"response": map[string]any{"content": contentToString(m.Content)},
						},
					},
				},
			})
			continue
		}
	}

	return systemInstruction, contents
}

func formatGeminiTools(tools []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		if fn, ok := t["function"].(map[string]any); ok {
			out = append(out, map[string]any{
				"name":        fn["name"],
				"description": fn["description"],
				"parameters":  fn["parameters"],
			})
		} else {
			out = append(out, t)
		}
	}
	return out
}
