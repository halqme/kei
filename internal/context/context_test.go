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

func TestMaterializePreservesRequestRegionsWithoutMutatingTail(t *testing.T) {
	tail := []transcript.Entry{{
		Role: transcript.RoleAssistant,
		ToolCalls: []transcript.ToolCall{{
			ID:        "call-1",
			Name:      "search",
			Arguments: `{"query":"hello"}`,
		}},
	}}
	request := New("base").Materialize(tail, []map[string]any{{"type": "function"}}, "")

	if request.Instructions != "base" {
		t.Fatalf("unexpected instructions: %q", request.Instructions)
	}
	if len(request.Tail) != 1 || request.Tail[0].Role != transcript.RoleAssistant || len(request.Tail[0].ToolCalls) != 1 {
		t.Fatalf("unexpected request tail: %+v", request.Tail)
	}
	if len(request.Tools) != 1 {
		t.Fatalf("unexpected request tools: %+v", request.Tools)
	}

	request.Tail[0].Role = transcript.RoleUser
	request.Tail[0].ToolCalls[0].Name = "changed"
	if tail[0].Role != transcript.RoleAssistant || tail[0].ToolCalls[0].Name != "search" {
		t.Fatalf("materialization exposed transcript slices: %+v", tail)
	}
}

func TestMaterializeInstructionReplacementIsRequestScoped(t *testing.T) {
	b := New("base")
	tail := []transcript.Entry{{Role: transcript.RoleUser, Content: "hello"}}

	replaced := b.Materialize(tail, nil, "temporary")
	if replaced.Instructions != "temporary" {
		t.Fatalf("replacement was not applied: %+v", replaced)
	}
	fresh := b.Materialize(tail, nil, "")
	if fresh.Instructions != "base" {
		t.Fatalf("replacement leaked into later materialization: %+v", fresh)
	}
}
