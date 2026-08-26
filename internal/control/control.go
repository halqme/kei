package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/halqme/kei/internal/config"
)

type Event struct {
	Kind         string         `json:"kind"`
	SessionID    string         `json:"session_id,omitempty"`
	Tool         string         `json:"tool,omitempty"`
	Effects      []string       `json:"effects,omitempty"`
	Input        map[string]any `json:"input,omitempty"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Workdir      string         `json:"workdir,omitempty"`
}

type Decision struct {
	Action       string   `json:"action,omitempty"` // allow, deny, ask; empty = no opinion
	Reason       string   `json:"reason,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	HiddenTools  []string `json:"hidden_tools,omitempty"`
}

type Chain struct{ controls []config.Control }

func New(cs []config.Control) *Chain { return &Chain{controls: cs} }

func (c *Chain) Apply(ctx context.Context, ev Event) (Decision, error) {
	result := Decision{}
	for _, ctl := range c.controls {
		b, _ := json.Marshal(ev)
		cmd := exec.CommandContext(ctx, ctl.Command, ctl.Args...)
		if ev.Workdir != "" {
			cmd.Dir = ev.Workdir
		}
		cmd.Stdin = bytes.NewReader(b)
		var out, er bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &er
		if err := cmd.Run(); err != nil {
			return Decision{}, fmt.Errorf("control %s: %w: %s", ctl.Command, err, er.String())
		}
		if out.Len() == 0 {
			continue
		}
		var d Decision
		if err := json.Unmarshal(out.Bytes(), &d); err != nil {
			return Decision{}, fmt.Errorf("control %s: invalid JSON: %w", ctl.Command, err)
		}
		if d.SystemPrompt != "" {
			result.SystemPrompt = d.SystemPrompt
			ev.SystemPrompt = d.SystemPrompt
		}
		result.HiddenTools = append(result.HiddenTools, d.HiddenTools...)
		if d.Action != "" {
			result.Action, result.Reason = d.Action, d.Reason
		}
		if d.Action == "deny" || d.Action == "ask" {
			return result, nil
		}
	}
	return result, nil
}
