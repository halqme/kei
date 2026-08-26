package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	loadToolName         = "load_skill"
	readResourceToolName = "read_skill_resource"
)

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Root        string `json:"root"`
}

type Registry struct {
	byName map[string]Skill
	list   []Skill
}

// SearchRoots follows the cross-client .agents/skills convention recommended
// by the Agent Skills client implementation guide: https://agentskills.io/client-implementation/adding-skills-support
func SearchRoots(workdir string) []string {
	roots := make([]string, 0, 2)
	if workdir != "" {
		roots = append(roots, filepath.Join(workdir, ".agents", "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, filepath.Join(home, ".agents", "skills"))
	}
	return uniqueDirs(roots)
}

func Discover(roots []string) (*Registry, error) {
	r := &Registry{byName: map[string]Skill{}}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			skillPath := filepath.Join(root, entry.Name(), "SKILL.md")
			b, err := os.ReadFile(skillPath)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			name, description, err := parseMetadata(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", skillPath, err)
			}
			if name != entry.Name() {
				return nil, fmt.Errorf("%s: skill name %q must match parent directory %q", skillPath, name, entry.Name())
			}
			if _, seen := r.byName[name]; seen {
				continue
			}
			s := Skill{Name: name, Description: description, Root: filepath.Join(root, entry.Name())}
			r.byName[name] = s
			r.list = append(r.list, s)
		}
	}
	return r, nil
}

func (r *Registry) List() []Skill {
	if r == nil {
		return nil
	}
	out := make([]Skill, len(r.list))
	copy(out, r.list)
	return out
}

func (r *Registry) CatalogPrompt() string {
	if r == nil || len(r.list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Agent Skills are available. Match the user's task against the skill descriptions below. Before following a skill, call load_skill to read its complete SKILL.md. Load referenced files only when needed with read_skill_resource.\n\nAvailable skills:\n")
	for _, s := range r.list {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	return strings.TrimSpace(b.String())
}

func (r *Registry) OpenAITools() []map[string]any {
	if r == nil || len(r.list) == 0 {
		return nil
	}
	return []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        loadToolName,
				"description": "Load the complete SKILL.md for an available Agent Skill before following its instructions.",
				"parameters": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"name": map[string]any{"type": "string", "description": "Skill name from the available skills catalog."},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        readResourceToolName,
				"description": "Read a file referenced by a loaded Agent Skill. The path is relative to that skill's directory.",
				"parameters": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"name": map[string]any{"type": "string", "description": "Skill name."},
						"path": map[string]any{"type": "string", "description": "File path relative to the skill directory."},
					},
					"required": []string{"name", "path"},
				},
			},
		},
	}
}

func (r *Registry) HandlesTool(name string) bool {
	return r != nil && len(r.list) > 0 && (name == loadToolName || name == readResourceToolName)
}

func (r *Registry) Execute(name string, input map[string]any) (string, error) {
	if r == nil {
		return "", fmt.Errorf("skills are not available")
	}
	skillName, ok := input["name"].(string)
	if !ok || skillName == "" {
		return "", fmt.Errorf("skill name is required")
	}
	s, ok := r.byName[skillName]
	if !ok {
		return "", fmt.Errorf("unknown skill %q", skillName)
	}

	switch name {
	case loadToolName:
		b, err := os.ReadFile(filepath.Join(s.Root, "SKILL.md"))
		return string(b), err
	case readResourceToolName:
		path, ok := input["path"].(string)
		if !ok || path == "" {
			return "", fmt.Errorf("resource path is required")
		}
		return readResource(s.Root, path)
	default:
		return "", fmt.Errorf("unknown skill tool %q", name)
	}
}

func readResource(root, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("resource path must be relative")
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resource path escapes skill root")
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, clean))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resource path escapes skill root")
	}

	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	resolvedTarget, err := filepath.EvalSymlinks(targetAbs)
	if err != nil {
		return "", err
	}
	resolvedRel, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resource path escapes skill root")
	}

	b, err := os.ReadFile(resolvedTarget)
	return string(b), err
}

func parseMetadata(data []byte) (string, string, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return "", "", fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return "", "", fmt.Errorf("SKILL.md frontmatter is not closed")
	}

	var metadata struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &metadata); err != nil {
		return "", "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	if err := validateName(metadata.Name); err != nil {
		return "", "", err
	}
	if n := utf8.RuneCountInString(metadata.Description); n == 0 || n > 1024 {
		return "", "", fmt.Errorf("description must be 1-1024 characters")
	}
	return metadata.Name, metadata.Description, nil
}

func validateName(name string) error {
	if len(name) == 0 || len(name) > 64 {
		return fmt.Errorf("name must be 1-64 characters")
	}
	if name[0] == '-' || name[len(name)-1] == '-' || strings.Contains(name, "--") {
		return fmt.Errorf("name must not start or end with a hyphen or contain consecutive hyphens")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("name may contain only lowercase letters, numbers, and hyphens")
	}
	return nil
}

func uniqueDirs(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, dir := range in {
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return out
}
