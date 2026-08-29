package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/halqme/kei/internal/transcript"
)

func TestFileStoreRoundTripsLogicalTranscript(t *testing.T) {
	store := NewFileStore(t.TempDir())
	state := &State{ID: "review-1", Workspace: "/workspace/project"}
	if err := store.Create(state); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	entries := []transcript.Entry{
		{Role: transcript.RoleUser, Content: "review this"},
		{
			Role:    transcript.RoleAssistant,
			Content: "checking",
			ToolCalls: []transcript.ToolCall{{
				ID:        "call-1",
				Name:      "search",
				Arguments: `{"query":"hello"}`,
			}},
		},
		{Role: transcript.RoleTool, ToolCallID: "call-1", Content: "result"},
	}
	for _, entry := range entries {
		if err := store.Append(state.ID, entry); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	loaded, err := store.Load(state.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.ID != state.ID || loaded.Workspace != state.Workspace {
		t.Fatalf("unexpected loaded metadata: %+v", loaded)
	}
	got := loaded.Transcript.Entries()
	if len(got) != len(entries) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(entries), got)
	}
	if got[0].Role != transcript.RoleUser || got[0].Content != "review this" {
		t.Fatalf("unexpected user entry: %+v", got[0])
	}
	if got[1].Role != transcript.RoleAssistant || len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].Name != "search" {
		t.Fatalf("unexpected assistant entry: %+v", got[1])
	}
	if got[2].Role != transcript.RoleTool || got[2].ToolCallID != "call-1" || got[2].Content != "result" {
		t.Fatalf("unexpected tool entry: %+v", got[2])
	}
}

func TestFileStoreRejectsContentOutsideDurableTextContract(t *testing.T) {
	store := NewFileStore(t.TempDir())
	state := &State{ID: "text-only", Workspace: "/workspace"}
	if err := store.Create(state); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err := store.Append(state.ID, transcript.Entry{
		Role:    transcript.RoleUser,
		Content: map[string]any{"type": "image"},
	})
	if err == nil {
		t.Fatal("Append accepted non-text durable content")
	}

	loaded, loadErr := store.Load(state.ID)
	if loadErr != nil {
		t.Fatalf("Load failed after rejected append: %v", loadErr)
	}
	if got := loaded.Transcript.Entries(); len(got) != 0 {
		t.Fatalf("rejected entry reached durable transcript: %+v", got)
	}
}

func TestFileStoreUsesSafeSessionIDs(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if _, err := store.Load("../escape"); err == nil {
		t.Fatal("Load accepted a path-like session id")
	}
	if err := store.Create(&State{ID: "../escape"}); err == nil {
		t.Fatal("Create accepted a path-like session id")
	}
}

func TestDefaultStoreDirUsesXDGStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	got, err := DefaultStoreDir()
	if err != nil {
		t.Fatalf("DefaultStoreDir failed: %v", err)
	}
	want := filepath.Join(stateHome, "kei", "sessions")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFileStoreReportsMissingAndExistingSessions(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	if _, err := store.Load("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load error = %v, want ErrNotFound", err)
	}
	state := &State{ID: "existing", Workspace: "/workspace"}
	if err := store.Create(state); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := store.Create(state); !errors.Is(err, ErrExists) {
		t.Fatalf("second Create error = %v, want ErrExists", err)
	}
	info, err := os.Stat(filepath.Join(root, "existing.jsonl"))
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("session file mode = %o, want 600", info.Mode().Perm())
	}
}
