package agent

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strings"

	keicommand "github.com/halqme/kei/internal/command"
	agentcontext "github.com/halqme/kei/internal/context"
	"github.com/halqme/kei/internal/control"
	"github.com/halqme/kei/internal/provider"
	"github.com/halqme/kei/internal/skill"
	"github.com/halqme/kei/internal/tool"
)

type ApprovalFunc func(ctx context.Context, toolName, reason string, input map[string]any) (bool, error)
type EventFunc func(kind string, payload any)

type Session struct {
	ID             string
	Provider       provider.Provider
	Tools          *tool.Registry
	Commands       *keicommand.Registry
	Skills         *skill.Registry
	Controls       *control.Chain
	ContextBuilder *agentcontext.Builder
	Workdir        string
	Messages       []provider.Message
	Approve        ApprovalFunc
	OnEvent        EventFunc
}

func (s *Session) Prompt(ctx context.Context, text string) (string, error) {
	if name, arguments, ok := keicommand.ParseInvocation(text); ok && s.Commands != nil {
		if d, found := s.Commands.Get(name); found {
			if s.OnEvent != nil {
				s.OnEvent("command_start", map[string]any{"name": d.QualifiedName, "arguments": arguments})
			}
			out, err := keicommand.Execute(ctx, s.Workdir, d, arguments)
			if s.OnEvent != nil {
				s.OnEvent("command_end", map[string]any{"name": d.QualifiedName, "output": out, "error": errString(err)})
			}
			return out, err
		}
	}

	s.Messages = append(s.Messages, provider.Message{Role: "user", Content: text})

	for turn := 0; turn < 32; turn++ {
		var tools []map[string]any
		if s.Tools != nil {
			tools = s.Tools.OpenAITools()
		}
		if s.Skills != nil {
			tools = append(tools, s.Skills.OpenAITools()...)
		}

		instructions := s.ContextBuilder.BaseInstructions()
		if s.Controls != nil {
			d, err := s.Controls.Apply(ctx, control.Event{Kind: "before_model", SessionID: s.ID, SystemPrompt: instructions, Workdir: s.Workdir})
			if err != nil {
				return "", err
			}
			if d.SystemPrompt != "" {
				instructions = d.SystemPrompt
			}
			if len(d.HiddenTools) > 0 {
				tools = filterTools(tools, d.HiddenTools)
			}
		}

		materialized := s.ContextBuilder.Materialize(s.Messages, tools, instructions)
		var callback provider.StreamCallback
		if s.OnEvent != nil {
			callback = func(event provider.StreamEvent) {
				if event.Type == provider.StreamEventTextDelta && event.Text != "" {
					s.OnEvent("assistant_message_chunk", map[string]any{"text": event.Text})
				}
			}
		}
		res, err := s.Provider.Stream(ctx, materialized.Messages, materialized.Tools, callback)
		if err != nil {
			return "", err
		}
		s.Messages = append(s.Messages, res.Message)
		if len(res.Message.ToolCalls) == 0 {
			return contentString(res.Message.Content), nil
		}

		for _, call := range res.Message.ToolCalls {
			input := map[string]any{}
			if strings.TrimSpace(call.Function.Arguments) != "" {
				if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
					return "", fmt.Errorf("tool %s arguments: %w", call.Function.Name, err)
				}
			}

			toolName := call.Function.Name
			var effects []string
			var execute func() (string, error)
			if s.Skills != nil && s.Skills.HandlesTool(call.Function.Name) {
				execute = func() (string, error) {
					return s.Skills.Execute(call.Function.Name, input)
				}
			} else {
				if s.Tools == nil {
					return "", fmt.Errorf("model requested unknown tool %q", call.Function.Name)
				}
				d, ok := s.Tools.Get(call.Function.Name)
				if !ok {
					return "", fmt.Errorf("model requested unknown tool %q", call.Function.Name)
				}
				toolName = d.QualifiedName
				effects = d.Effects
				execute = func() (string, error) {
					return tool.Execute(ctx, s.Workdir, d, input)
				}
			}

			if s.OnEvent != nil {
				s.OnEvent("tool_start", map[string]any{"name": toolName, "input": input})
			}
			if s.Controls != nil {
				dec, err := s.Controls.Apply(ctx, control.Event{Kind: "before_tool", SessionID: s.ID, Tool: toolName, Effects: effects, Input: input, Workdir: s.Workdir})
				if err != nil {
					return "", err
				}
				switch dec.Action {
				case "deny":
					s.Messages = append(s.Messages, provider.Message{Role: "tool", ToolCallID: call.ID, Content: "Denied: " + dec.Reason})
					continue
				case "ask":
					if s.Approve == nil {
						return "", fmt.Errorf("tool %s requires approval but no approval frontend is available", toolName)
					}
					yes, err := s.Approve(ctx, toolName, dec.Reason, input)
					if err != nil {
						return "", err
					}
					if !yes {
						s.Messages = append(s.Messages, provider.Message{Role: "tool", ToolCallID: call.ID, Content: "Denied by user"})
						continue
					}
				}
			}
			out, err := execute()
			if s.OnEvent != nil {
				s.OnEvent("tool_end", map[string]any{"name": toolName, "output": out, "error": errString(err)})
			}
			if err != nil {
				out = "ERROR: " + err.Error() + "\n" + out
			}
			s.Messages = append(s.Messages, provider.Message{Role: "tool", ToolCallID: call.ID, Content: out})
			if s.Controls != nil {
				_, _ = s.Controls.Apply(ctx, control.Event{Kind: "after_tool", SessionID: s.ID, Tool: toolName, Effects: effects, Input: input, Workdir: s.Workdir})
			}
		}
	}
	return "", fmt.Errorf("agent exceeded maximum turns")
}

func filterTools(in []map[string]any, hidden []string) []map[string]any {
	h := map[string]bool{}
	for _, n := range hidden {
		h[n] = true
		h[strings.ReplaceAll(n, ".", "_")] = true
	}
	out := make([]map[string]any, 0, len(in))
	for _, t := range in {
		fn, _ := t["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if !h[name] {
			out = append(out, t)
		}
	}
	return out
}
func contentString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
