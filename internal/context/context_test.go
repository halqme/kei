package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halqme/kei/internal/transcript"
)

func TestNewForWorkspaceComposesStableBase(t *testing.T) {
	workdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workdir, "AGENTS.md"), []byte("# Project instructions\n\nUse small changes.\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	b, err := NewForWorkspace(workdir, "Agent Skills are available.\n- review: Review code.")
	if err != nil {
		t.Fatalf("NewForWorkspace failed: %v", err)
	}
	got := b.BaseInstructions()
	baseAt := strings.Index(got, basePrompt)
	agentsAt := strings.Index(got, "# Project instructions")
	skillsAt := strings.Index(got, "Agent Skills are available.")
	if baseAt < 0 || agentsAt < 0 || skillsAt < 0 || !(baseAt < agentsAt && agentsAt < skillsAt) {
		t.Fatalf("unexpected base instruction composition:\n%s", got)
	}
}

func TestNewForWorkspaceAllowsMissingAgentsFile(t *testing.T) {
	b, err := NewForWorkspace(t.TempDir(), "")
	if err != nil {
		t.Fatalf("NewForWorkspace failed: %v", err)
	}
	if got := b.BaseInstructions(); got != basePrompt {
		t.Fatalf("unexpected base instructions: %q", got)
	}
}

func TestMaterializeRendersTranscriptWithoutMutatingIt(t *testing.T) {
	tail := []transcript.Entry{{Role: transcript.RoleUser, Content: "hello"}}
	m := New("base").Materialize(tail, []map[string]any{{"type": "function"}}, "")

	if len(m.Messages) != 2 || m.Messages[0].Role != "system" || m.Messages[0].Content != "base" || m.Messages[1].Role != "user" {
		t.Fatalf("unexpected materialized messages: %+v", m.Messages)
	}
	if len(tail) != 1 || tail[0].Role != transcript.RoleUser {
		t.Fatalf("materialization mutated transcript tail: %+v", tail)
	}
	if len(m.Tools) != 1 {
		t.Fatalf("unexpected materialized tools: %+v", m.Tools)
	}
}

func TestMaterializeRendersLogicalToolCallsForProvider(t *testing.T) {
	tail := []transcript.Entry{{
		Role:    transcript.RoleAssistant,
		Content: "checking",
		ToolCalls: []transcript.ToolCall{{
			ID:        "call-1",
			Name:      "search",
			Arguments: `{"query":"hello"}`,
		}},
	}}

	m := New("").Materialize(tail, nil, "")
	if len(m.Messages) != 1 || len(m.Messages[0].ToolCalls) != 1 {
		t.Fatalf("unexpected materialized tool call: %+v", m.Messages)
	}
	call := m.Messages[0].ToolCalls[0]
	if call.ID != "call-1" || call.Type != "function" || call.Function.Name != "search" || call.Function.Arguments != `{"query":"hello"}` {
		t.Fatalf("unexpected provider tool call: %+v", call)
	}
}

func TestMaterializeInstructionReplacementIsRequestScoped(t *testing.T) {
	b := New("base")
	tail := []transcript.Entry{{Role: transcript.RoleUser, Content: "hello"}}

	replaced := b.Materialize(tail, nil, "temporary")
	if replaced.Messages[0].Content != "temporary" {
		t.Fatalf("replacement was not applied: %+v", replaced.Messages)
	}
	fresh := b.Materialize(tail, nil, "")
	if fresh.Messages[0].Content != "base" {
		t.Fatalf("replacement leaked into later materialization: %+v", fresh.Messages)
	}
}
