package main

import (
	"path/filepath"
	"testing"

	"github.com/halqme/kei/internal/transcript"
)

func TestOpenSessionCreatesAndColdResumesNamedState(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	firstWorkspace := filepath.Join(t.TempDir(), "first")
	secondWorkspace := filepath.Join(t.TempDir(), "second")

	state, store, err := openSession("review-task", firstWorkspace)
	if err != nil {
		t.Fatalf("openSession create failed: %v", err)
	}
	entry := transcript.Entry{Role: transcript.RoleUser, Content: "remember this"}
	if err := store.Append(state.ID, entry); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	state.Transcript.Append(entry)

	resumed, resumedStore, err := openSession("review-task", secondWorkspace)
	if err != nil {
		t.Fatalf("openSession resume failed: %v", err)
	}
	if resumedStore == nil {
		t.Fatal("resumed named session has no store")
	}
	if resumed.Workspace != firstWorkspace {
		t.Fatalf("resumed workspace = %q, want persisted %q", resumed.Workspace, firstWorkspace)
	}
	entries := resumed.Transcript.Entries()
	if len(entries) != 1 || entries[0].Content != "remember this" {
		t.Fatalf("resumed transcript = %+v", entries)
	}
}

func TestOpenSessionWithoutIDRemainsEphemeral(t *testing.T) {
	state, store, err := openSession("", "/workspace")
	if err != nil {
		t.Fatalf("openSession failed: %v", err)
	}
	if store != nil {
		t.Fatal("ephemeral session unexpectedly has a store")
	}
	if state.ID != "cli" || state.Workspace != "/workspace" {
		t.Fatalf("unexpected ephemeral state: %+v", state)
	}
}
