package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatible struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

func (p *OpenAICompatible) Stream(ctx context.Context, messages []Message, tools []map[string]any, callback StreamCallback) (Result, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	body := map[string]any{"model": p.Model, "messages": messages}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var e any
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return Result{}, fmt.Errorf("provider returned %s: %v", resp.Status, e)
	}
	var raw struct {
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Result{}, err
	}
	if len(raw.Choices) == 0 {
		return Result{}, fmt.Errorf("provider returned no choices")
	}
	result := Result{Message: raw.Choices[0].Message, FinishReason: raw.Choices[0].FinishReason}
	if callback != nil {
		if text, ok := result.Message.Content.(string); ok && text != "" {
			callback(StreamEvent{Type: StreamEventTextDelta, Text: text})
		}
	}
	return result, nil
}
