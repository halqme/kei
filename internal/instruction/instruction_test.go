package instruction

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadComposesBaseWorkspaceAndSkillInstructions(t *testing.T) {
	workdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workdir, "AGENTS.md"), []byte("# Project instructions\n\nUse small changes.\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got, err := Load(workdir, "Agent Skills are available.\n- review: Review code.")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	for _, want := range []string{
		basePrompt,
		"# Project instructions",
		"Agent Skills are available.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("system prompt does not contain %q:\n%s", want, got)
		}
	}
}

func TestLoadAllowsWorkspaceWithoutAgentsFile(t *testing.T) {
	got, err := Load(t.TempDir(), "")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got != basePrompt {
		t.Fatalf("unexpected prompt: %q", got)
	}
}
