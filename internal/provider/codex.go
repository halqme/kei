package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/halqme/kei/internal/auth"
)

const (
	CodexDefaultBaseURL = "https://chatgpt.com/backend-api/codex"
	CodexDefaultModel   = "gpt-5.5"
)

type Codex struct {
	BaseURL     string
	Model       string
	StaticToken string
	AccountID   string
	Client      *http.Client
}

func (p *Codex) Stream(ctx context.Context, messages []Message, tools []map[string]any, callback StreamCallback) (Result, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}

	token := p.StaticToken
	accountID := p.AccountID
	if token == "" {
		creds, err := auth.GetDefaultStore().GetCredential(ctx, "codex")
		if err != nil {
			return Result{}, fmt.Errorf("codex provider: %w (run 'kei login codex' or set CODEX_ACCESS_TOKEN)", err)
		}
		token = creds.AccessToken
		if accountID == "" {
			accountID = creds.AccountID
		}
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = CodexDefaultBaseURL
	}
	model := p.Model
	if model == "" {
		model = CodexDefaultModel
	}

	instructions, input := formatResponsesInput(messages)

	body := map[string]any{
		"model":               model,
		"input":               input,
		"store":               false,
		"stream":              true,
		"parallel_tool_calls": false,
	}
	if instructions != "" {
		body["instructions"] = instructions
	}
	if len(tools) > 0 {
		body["tools"] = formatResponsesTools(tools)
		body["tool_choice"] = "auto"
	}

	reqBytes, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if accountID != "" {
		req.Header.Set("chatgpt-account-id", accountID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(resp.Body)
		var errObj any
		_ = json.Unmarshal(respBody, &errObj)
		return Result{}, fmt.Errorf("codex provider returned %s: %v", resp.Status, errObj)
	}

	return readCodexStream(resp.Body, callback)
}

func formatResponsesInput(messages []Message) (string, []map[string]any) {
	var instructions string
	var input []map[string]any

	for i, m := range messages {
		if m.Role == "system" {
			if i == 0 && instructions == "" {
				instructions = contentToString(m.Content)
				continue
			}
			input = append(input, map[string]any{
				"type": "message",
				"role": "developer",
				"content": []map[string]any{
					{"type": "input_text", "text": contentToString(m.Content)},
				},
			})
			continue
		}

		if m.Role == "user" {
			input = append(input, map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": contentToString(m.Content)},
				},
			})
			continue
		}

		if m.Role == "assistant" {
			// Add any tool calls as function_call items
			for _, tc := range m.ToolCalls {
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}
			// Add text content if present
			if text := contentToString(m.Content); text != "" {
				input = append(input, map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []map[string]any{
						{"type": "output_text", "text": text},
					},
				})
			}
			continue
		}

		if m.Role == "tool" {
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  contentToString(m.Content),
			})
			continue
		}
	}

	return instructions, input
}

func formatResponsesTools(tools []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		if fn, ok := t["function"].(map[string]any); ok {
			out = append(out, map[string]any{
				"type":        "function",
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

func contentToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

type codexStreamItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Role      string `json:"role"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Text      string `json:"text"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type codexStreamError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type codexStreamResponse struct {
	ID                string            `json:"id"`
	Status            string            `json:"status"`
	Output            []codexStreamItem `json:"output"`
	Error             *codexStreamError `json:"error"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type codexStreamEvent struct {
	Type        string               `json:"type"`
	Delta       string               `json:"delta"`
	Arguments   string               `json:"arguments"`
	ItemID      string               `json:"item_id"`
	CallID      string               `json:"call_id"`
	Name        string               `json:"name"`
	OutputIndex *int                 `json:"output_index"`
	Item        *codexStreamItem     `json:"item"`
	Response    *codexStreamResponse `json:"response"`
	Error       *codexStreamError    `json:"error"`
	Code        string               `json:"code"`
	Message     string               `json:"message"`
}

type codexTextState struct {
	deltas   []string
	fallback []string
	sawDelta bool
}

type codexCallState struct {
	id            string
	name          string
	argumentParts []string
}

type codexStreamAccumulator struct {
	textItems   []*codexTextState
	textIndexes map[string]int
	calls       []*codexCallState
	callIndexes map[string]int
}

func newCodexStreamAccumulator() *codexStreamAccumulator {
	return &codexStreamAccumulator{
		textIndexes: map[string]int{},
		callIndexes: map[string]int{},
	}
}

func codexStreamKeys(ids []string, outputIndex *int) []string {
	keys := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		if id != "" {
			keys = append(keys, id)
		}
	}
	if outputIndex != nil {
		keys = append(keys, fmt.Sprintf("output-%d", *outputIndex))
	}
	if len(keys) == 0 {
		keys = append(keys, "default")
	}
	return keys
}

func findCodexStreamIndex(indexes map[string]int, keys []string) int {
	for _, key := range keys {
		if index, ok := indexes[key]; ok {
			return index
		}
	}
	return -1
}

func registerCodexStreamKeys(indexes map[string]int, index int, keys []string) {
	for _, key := range keys {
		indexes[key] = index
	}
}

func (a *codexStreamAccumulator) textState(keys []string) *codexTextState {
	index := findCodexStreamIndex(a.textIndexes, keys)
	if index < 0 {
		index = len(a.textItems)
		a.textItems = append(a.textItems, &codexTextState{})
	}
	registerCodexStreamKeys(a.textIndexes, index, keys)
	return a.textItems[index]
}

func (a *codexStreamAccumulator) addTextDelta(event codexStreamEvent) {
	state := a.textState(codexStreamKeys([]string{event.ItemID}, event.OutputIndex))
	state.sawDelta = true
	if event.Delta != "" {
		state.deltas = append(state.deltas, event.Delta)
	}
}

func (a *codexStreamAccumulator) addItem(item *codexStreamItem, itemID string, outputIndex *int) {
	if item == nil {
		return
	}

	keys := codexStreamKeys([]string{item.ID, itemID}, outputIndex)
	switch item.Type {
	case "message":
		state := a.textState(keys)
		if state.sawDelta || len(state.fallback) > 0 {
			return
		}
		if item.Text != "" {
			state.fallback = append(state.fallback, item.Text)
		}
		for _, content := range item.Content {
			if content.Text != "" {
				state.fallback = append(state.fallback, content.Text)
			}
		}
	case "function_call", "tool_call":
		a.addCall(*item, "", outputIndex)
	}
}

func (a *codexStreamAccumulator) addCall(item codexStreamItem, argument string, outputIndex *int) {
	keys := codexStreamKeys([]string{item.ID, item.CallID}, outputIndex)
	index := findCodexStreamIndex(a.callIndexes, keys)
	if index < 0 {
		index = len(a.calls)
		a.calls = append(a.calls, &codexCallState{})
	}
	registerCodexStreamKeys(a.callIndexes, index, keys)

	call := a.calls[index]
	if call.id == "" {
		call.id = item.CallID
		if call.id == "" {
			call.id = item.ID
		}
		if call.id == "" && outputIndex != nil {
			call.id = fmt.Sprintf("output-%d", *outputIndex)
		}
	}
	if item.Name != "" {
		call.name = item.Name
	}
	if argument != "" {
		call.argumentParts = append(call.argumentParts, argument)
	} else if item.Arguments != "" && len(call.argumentParts) == 0 {
		call.argumentParts = append(call.argumentParts, item.Arguments)
	}
}

func (a *codexStreamAccumulator) result(status string) Result {
	var textParts []string
	for _, item := range a.textItems {
		parts := item.deltas
		if !item.sawDelta {
			parts = item.fallback
		}
		if text := strings.Join(parts, ""); text != "" {
			textParts = append(textParts, text)
		}
	}

	toolCalls := make([]ToolCall, 0, len(a.calls))
	for _, call := range a.calls {
		toolCalls = append(toolCalls, ToolCall{
			ID:   call.id,
			Type: "function",
			Function: FunctionCall{
				Name:      call.name,
				Arguments: strings.Join(call.argumentParts, ""),
			},
		})
	}

	return Result{
		Message: Message{
			Role:      "assistant",
			Content:   strings.Join(textParts, "\n"),
			ToolCalls: toolCalls,
		},
		FinishReason: status,
	}
}

func codexStreamErrorMessage(event codexStreamEvent) string {
	if event.Error != nil && event.Error.Message != "" {
		return event.Error.Message
	}
	if event.Response != nil && event.Response.Error != nil && event.Response.Error.Message != "" {
		return event.Response.Error.Message
	}
	if event.Message != "" {
		return event.Message
	}
	if event.Code != "" {
		return event.Code
	}
	return "unknown stream error"
}

func readCodexStream(body io.Reader, callback StreamCallback) (Result, error) {
	accumulator := newCodexStreamAccumulator()
	finishReason := ""
	terminal := false

	process := func(payload string) error {
		payload = strings.TrimSpace(payload)
		if payload == "" {
			return nil
		}
		if payload == "[DONE]" {
			finishReason = "completed"
			terminal = true
			return nil
		}

		var event codexStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return fmt.Errorf("codex provider stream: invalid event: %w", err)
		}
		if event.Type == "" {
			return fmt.Errorf("codex provider stream: event is missing type")
		}

		switch event.Type {
		case "response.output_text.delta":
			accumulator.addTextDelta(event)
			if event.Delta != "" && callback != nil {
				callback(StreamEvent{Type: StreamEventTextDelta, Text: event.Delta})
			}
		case "response.output_item.added", "response.output_item.done":
			accumulator.addItem(event.Item, event.ItemID, event.OutputIndex)
		case "response.function_call_arguments.delta", "response.tool_call_arguments.delta":
			accumulator.addCall(codexStreamItem{ID: event.ItemID, CallID: event.CallID}, event.Delta, event.OutputIndex)
		case "response.function_call_arguments.done", "response.tool_call_arguments.done":
			accumulator.addCall(codexStreamItem{
				ID:        event.ItemID,
				CallID:    event.CallID,
				Name:      event.Name,
				Arguments: event.Arguments,
			}, "", event.OutputIndex)
		case "response.completed", "response.done":
			if event.Response != nil {
				for i := range event.Response.Output {
					accumulator.addItem(&event.Response.Output[i], "", nil)
				}
				finishReason = event.Response.Status
			}
			if finishReason == "" {
				finishReason = "completed"
			}
			terminal = true
		case "response.failed", "error", "response.error":
			return fmt.Errorf("codex provider stream failed: %s", codexStreamErrorMessage(event))
		case "response.incomplete":
			reason := "unknown reason"
			if event.Response != nil && event.Response.IncompleteDetails != nil && event.Response.IncompleteDetails.Reason != "" {
				reason = event.Response.IncompleteDetails.Reason
			}
			return fmt.Errorf("codex provider response incomplete: %s", reason)
		}
		return nil
	}

	reader := bufio.NewReader(body)
	var dataLines []string
	dispatch := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = nil
		return process(payload)
	}

	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			switch {
			case line == "":
				if err := dispatch(); err != nil {
					return Result{}, err
				}
			case strings.HasPrefix(line, ":"):
			case strings.HasPrefix(line, "data:"):
				data := strings.TrimPrefix(line, "data:")
				dataLines = append(dataLines, strings.TrimPrefix(data, " "))
			}
		}

		if terminal {
			break
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return Result{}, err
			}
			if err := dispatch(); err != nil {
				return Result{}, err
			}
			if !terminal {
				return Result{}, fmt.Errorf("codex provider stream ended before completion")
			}
			break
		}
	}

	return accumulator.result(finishReason), nil
}
