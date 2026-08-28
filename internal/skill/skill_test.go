package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverLoadsProjectSkillBeforeUserSkill(t *testing.T) {
	home := t.TempDir()
	workdir := t.TempDir()
	t.Setenv("HOME", home)

	writeSkill := func(root, description string) {
		t.Helper()
		dir := filepath.Join(root, ".agents", "skills", "review")
		if err := os.MkdirAll(filepath.Join(dir, "references"), 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
		content := "---\nname: review\ndescription: " + description + "\n---\n\n# Review\n\nFollow the review workflow.\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile SKILL.md failed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "references", "guide.md"), []byte("project guide"), 0644); err != nil {
			t.Fatalf("WriteFile guide failed: %v", err)
		}
	}
	writeSkill(home, "User review workflow.")
	writeSkill(workdir, "Project review workflow.")

	r, err := Discover(SearchRoots(workdir))
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	got := r.List()
	if len(got) != 1 {
		t.Fatalf("got %d skills, want 1", len(got))
	}
	if got[0].Description != "Project review workflow." || !strings.HasPrefix(got[0].Root, workdir) {
		t.Fatalf("project skill did not shadow user skill: %+v", got[0])
	}
	if catalog := r.CatalogPrompt(); !strings.Contains(catalog, "review: Project review workflow.") {
		t.Fatalf("catalog does not advertise project skill: %s", catalog)
	}

	loaded, err := r.Execute("load_skill", map[string]any{"name": "review"})
	if err != nil {
		t.Fatalf("load_skill failed: %v", err)
	}
	if !strings.Contains(loaded, "# Review") {
		t.Fatalf("load_skill did not return SKILL.md: %s", loaded)
	}
	resource, err := r.Execute("read_skill_resource", map[string]any{"name": "review", "path": "references/guide.md"})
	if err != nil {
		t.Fatalf("read_skill_resource failed: %v", err)
	}
	if resource != "project guide" {
		t.Fatalf("unexpected resource: %q", resource)
	}
}

func TestDiscoverRejectsSkillNameThatDoesNotMatchDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agents", "skills", "review")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: other\ndescription: Review code.\n---\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := Discover([]string{filepath.Dir(root)})
	if err == nil || !strings.Contains(err.Error(), "must match parent directory") {
		t.Fatalf("expected name mismatch error, got %v", err)
	}
}

func TestReadSkillResourceCannotEscapeSkillRoot(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "review")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if _, err := readResource(skillRoot, "../secret.txt"); err == nil {
		t.Fatal("expected traversal outside skill root to fail")
	}
}
