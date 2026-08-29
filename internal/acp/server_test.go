package acp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/halqme/kei/internal/agent"
	keicommand "github.com/halqme/kei/internal/command"
	"github.com/halqme/kei/internal/session"
)

func TestInitialize(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	var out bytes.Buffer
	s := NewServer(in, &out, func(id, cwd string) (*agent.Runtime, error) {
		return &agent.Runtime{State: &session.State{ID: id, Workspace: cwd}}, nil
	})
	if err := s.Serve(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"name":"kei"`) || !strings.Contains(got, `"id":1`) {
		t.Fatalf("unexpected response: %s", got)
	}
}

func TestSessionNewAdvertisesCommands(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n")
	var out bytes.Buffer
	commands, err := keicommand.NewRegistry([]keicommand.Descriptor{{Name: "status", QualifiedName: "unix:status", Description: "Show status", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(in, &out, func(id, cwd string) (*agent.Runtime, error) {
		return &agent.Runtime{State: &session.State{ID: id, Workspace: cwd}, Commands: commands}, nil
	})
	if err := s.Serve(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"sessionUpdate":"available_commands_update"`) || !strings.Contains(got, `"name":"unix:status"`) {
		t.Fatalf("unexpected response: %s", got)
	}
}
