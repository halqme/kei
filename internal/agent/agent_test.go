package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcontext "github.com/halqme/kei/internal/context"
	"github.com/halqme/kei/internal/provider"
	"github.com/halqme/kei/internal/session"
	"github.com/halqme/kei/internal/skill"
	"github.com/halqme/kei/internal/transcript"
)

type skillProvider struct {
	t     *testing.T
	calls int
}

func (p *skillProvider) Generate(_ context.Context, request agentcontext.Request, _ provider.StreamCallback) (provider.Result, error) {
	p.t.Helper()
	p.calls++
	if request.Instructions != "base" || len(request.Tail) == 0 || request.Tail[0].Role != transcript.RoleUser {
		p.t.Fatalf("unexpected materialized request: %+v", request)
	}
	switch p.calls {
	case 1:
		found := false
		for _, tool := range request.Tools {
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
		last := request.Tail[len(request.Tail)-1]
		text, _ := last.Content.(string)
		if last.Role != transcript.RoleTool || last.ToolCallID != "skill-call" || !strings.Contains(text, "# Review") {
			p.t.Fatalf("unexpected skill result: %+v", last)
		}
		return provider.Result{Message: provider.Message{Role: "assistant", Content: "done"}}, nil
	default:
		p.t.Fatalf("unexpected provider call %d", p.calls)
		return provider.Result{}, nil
	}
}

func TestRuntimeUsesSessionStateForMaterializedContext(t *testing.T) {
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
	state := &session.State{ID: "test", Workspace: t.TempDir()}
	runtime := &Runtime{State: state, Provider: p, Skills: skills, ContextBuilder: agentcontext.New("base")}
	got, err := runtime.Prompt(context.Background(), "review this")
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}
	if got != "done" {
		t.Fatalf("unexpected response: %q", got)
	}

	entries := state.Transcript.Entries()
	if len(entries) != 4 {
		t.Fatalf("got %d transcript entries, want 4: %+v", len(entries), entries)
	}
	if entries[0].Role != transcript.RoleUser || entries[0].Content != "review this" {
		t.Fatalf("unexpected first transcript entry: %+v", entries[0])
	}
	if entries[1].Role != transcript.RoleAssistant || len(entries[1].ToolCalls) != 1 || entries[1].ToolCalls[0].Name != "load_skill" {
		t.Fatalf("unexpected assistant tool-call entry: %+v", entries[1])
	}
	if entries[2].Role != transcript.RoleTool || entries[2].ToolCallID != "skill-call" {
		t.Fatalf("unexpected tool result entry: %+v", entries[2])
	}
	if entries[3].Role != transcript.RoleAssistant || entries[3].Content != "done" {
		t.Fatalf("unexpected final assistant entry: %+v", entries[3])
	}
}

type recordingStore struct {
	entries []transcript.Entry
}

func (s *recordingStore) Create(*session.State) error { return nil }
func (s *recordingStore) Load(string) (*session.State, error) {
	return nil, session.ErrNotFound
}
func (s *recordingStore) Append(_ string, entry transcript.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

type missingToolProvider struct{}

func (missingToolProvider) Generate(_ context.Context, _ agentcontext.Request, _ provider.StreamCallback) (provider.Result, error) {
	return provider.Result{Message: provider.Message{
		Role: "assistant",
		ToolCalls: []provider.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: provider.FunctionCall{
				Name:      "missing",
				Arguments: `{}`,
			},
		}},
	}}, nil
}

func TestRuntimePersistsAssistantToolCallBeforeExecution(t *testing.T) {
	store := &recordingStore{}
	state := &session.State{ID: "durable", Workspace: t.TempDir()}
	runtime := &Runtime{State: state, Store: store, Provider: missingToolProvider{}}

	_, err := runtime.Prompt(context.Background(), "use a tool")
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("Prompt error = %v, want unknown tool", err)
	}
	if len(store.entries) != 2 {
		t.Fatalf("persisted %d entries, want user + assistant tool call: %+v", len(store.entries), store.entries)
	}
	assistant := store.entries[1]
	if assistant.Role != transcript.RoleAssistant || len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call-1" {
		t.Fatalf("assistant tool call was not persisted before execution: %+v", assistant)
	}
	if got := state.Transcript.Entries(); len(got) != 2 {
		t.Fatalf("in-memory transcript diverged from persisted entries: %+v", got)
	}
}

func TestRuntimeRequiresSessionState(t *testing.T) {
	runtime := &Runtime{}
	if _, err := runtime.Prompt(context.Background(), "hello"); err == nil {
		t.Fatal("Prompt succeeded without session state")
	}
}
