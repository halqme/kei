package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halqme/kei/internal/provider"
	"github.com/halqme/kei/internal/skill"
)

type skillProvider struct {
	t     *testing.T
	calls int
}

func (p *skillProvider) Stream(_ context.Context, messages []provider.Message, tools []map[string]any, _ provider.StreamCallback) (provider.Result, error) {
	p.t.Helper()
	p.calls++
	switch p.calls {
	case 1:
		found := false
		for _, tool := range tools {
			fn, _ := tool["function"].(map[string]any)
			if fn["name"] == "load_skill" {
				found = true
				break
			}
		}
		if !found {
			p.t.Fatal("load_skill was not exposed to the provider")
		}
		return provider.Result{Message: provider.Message{
			Role: "assistant",
			ToolCalls: []provider.ToolCall{{
				ID:   "skill-call",
				Type: "function",
				Function: provider.FunctionCall{
					Name:      "load_skill",
					Arguments: `{"name":"review"}`,
				},
			}},
		}}, nil
	case 2:
		last := messages[len(messages)-1]
		if last.Role != "tool" || last.ToolCallID != "skill-call" || !strings.Contains(last.Content.(string), "# Review") {
			p.t.Fatalf("unexpected skill result: %+v", last)
		}
		return provider.Result{Message: provider.Message{Role: "assistant", Content: "done"}}, nil
	default:
		p.t.Fatalf("unexpected provider call %d", p.calls)
		return provider.Result{}, nil
	}
}

func TestSessionLoadsDiscoveredSkillThroughToolLoop(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	dir := filepath.Join(root, "review")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\ndescription: Review code.\n---\n\n# Review\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	skills, err := skill.Discover([]string{root})
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	p := &skillProvider{t: t}
	s := &Session{Provider: p, Skills: skills, SystemPrompt: "base"}
	got, err := s.Prompt(context.Background(), "review this")
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}
	if got != "done" {
		t.Fatalf("unexpected response: %q", got)
	}
}
