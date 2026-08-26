package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseInvocation(t *testing.T) {
	name, args, ok := ParseInvocation("/review:security src/auth.go")
	if !ok || name != "review:security" || args != "src/auth.go" {
		t.Fatalf("got %q %q %v", name, args, ok)
	}
}

func TestExecuteResolvesExtensionCommand(t *testing.T) {
	root := t.TempDir()
	extRoot := filepath.Join(root, "ext")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(extRoot, "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(extRoot, "commands", "echo")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s:%s' \"$(pwd)\" \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := Descriptor{Name: "echo", QualifiedName: "demo:echo", Command: "./commands/echo", Args: []string{"{arguments?}"}, BaseDir: extRoot}
	out, err := Execute(context.Background(), workspace, d, "hello world")
	if err != nil {
		t.Fatal(err)
	}
	want := workspace + ":hello world"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
