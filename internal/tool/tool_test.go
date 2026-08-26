package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExpandArgs(t *testing.T) {
	got, err := expandArgs([]string{"--json", "{pattern}", "{path?}"}, map[string]any{"pattern": "foo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "--json" || got[1] != "foo" {
		t.Fatalf("got %#v", got)
	}
}

func TestMissingRequired(t *testing.T) {
	if _, err := expandArgs([]string{"{x}"}, map[string]any{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestExecuteResolvesExtensionCommandButKeepsWorkspaceCWD(t *testing.T) {
	root := t.TempDir()
	extRoot := filepath.Join(root, "extension")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(extRoot, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(extRoot, "tools", "where")
	if err := os.WriteFile(path, []byte("#!/bin/sh\npwd\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := Execute(context.Background(), workspace, Descriptor{Name: "where", Command: "./tools/where", BaseDir: extRoot}, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Clean(stringTrimSpace(out)); got != filepath.Clean(workspace) {
		t.Fatalf("cwd = %q, want %q", got, workspace)
	}
}

func TestExecuteAppliesSchemaDefaults(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "echo-arg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := Descriptor{
		Name:    "echo",
		Command: "./echo-arg",
		BaseDir: root,
		Args:    []string{"{path}"},
		InputSchema: map[string]any{
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "default": "."},
			},
		},
	}
	out, err := Execute(context.Background(), root, d, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "." {
		t.Fatalf("got %q", out)
	}
}

func stringTrimSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
