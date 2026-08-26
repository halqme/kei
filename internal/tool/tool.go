package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Descriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	Command     string         `json:"command"`
	Args        []string       `json:"args,omitempty"`
	Stdin       string         `json:"stdin,omitempty"` // "json" or empty
	TimeoutMS   int            `json:"timeout_ms,omitempty"`
	Effects     []string       `json:"effects,omitempty"`

	ExtensionID   string `json:"extension_id,omitempty"`
	QualifiedName string `json:"qualified_name,omitempty"`
	ModelName     string `json:"model_name,omitempty"`
	BaseDir       string `json:"-"`
}

type Registry struct {
	byQualified map[string]Descriptor
	byModel     map[string]Descriptor
}

func NewRegistry(ds []Descriptor) (*Registry, error) {
	r := &Registry{byQualified: map[string]Descriptor{}, byModel: map[string]Descriptor{}}
	for _, d := range ds {
		if d.Name == "" || d.Command == "" {
			return nil, fmt.Errorf("tool name and command are required")
		}
		if d.QualifiedName == "" {
			d.QualifiedName = d.Name
		}
		if d.ModelName == "" {
			d.ModelName = sanitizeName(d.QualifiedName)
		}
		if _, ok := r.byQualified[d.QualifiedName]; ok {
			return nil, fmt.Errorf("duplicate tool %q", d.QualifiedName)
		}
		if prev, ok := r.byModel[d.ModelName]; ok {
			return nil, fmt.Errorf("tool model name collision %q between %q and %q", d.ModelName, prev.QualifiedName, d.QualifiedName)
		}
		r.byQualified[d.QualifiedName] = d
		r.byModel[d.ModelName] = d
	}
	return r, nil
}

func (r *Registry) List() []Descriptor {
	out := make([]Descriptor, 0, len(r.byQualified))
	for _, d := range r.byQualified {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QualifiedName < out[j].QualifiedName })
	return out
}

// Get accepts either the stable qualified name used by kei (extension.tool)
// or the provider-facing model name (extension_tool).
func (r *Registry) Get(name string) (Descriptor, bool) {
	if d, ok := r.byQualified[name]; ok {
		return d, true
	}
	d, ok := r.byModel[name]
	return d, ok
}

func (r *Registry) OpenAITools() []map[string]any {
	ds := r.List()
	out := make([]map[string]any, 0, len(ds))
	for _, d := range ds {
		out = append(out, map[string]any{"type": "function", "function": map[string]any{
			"name": d.ModelName, "description": d.Description, "parameters": d.InputSchema,
		}})
	}
	return out
}

func Execute(ctx context.Context, cwd string, d Descriptor, input map[string]any) (string, error) {
	timeout := 60 * time.Second
	if d.TimeoutMS > 0 {
		timeout = time.Duration(d.TimeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	input = applyDefaults(d.InputSchema, input)
	args, err := expandArgs(d.Args, input)
	if err != nil {
		return "", err
	}
	command := resolveCommand(d.BaseDir, d.Command)
	cmd := exec.CommandContext(ctx, command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if d.Stdin == "json" {
		b, err := json.Marshal(input)
		if err != nil {
			return "", err
		}
		cmd.Stdin = bytes.NewReader(b)
	}
	err = cmd.Run()
	if ctx.Err() != nil {
		return stdout.String(), ctx.Err()
	}
	if err != nil {
		if stderr.Len() > 0 {
			return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func applyDefaults(schema map[string]any, input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	props, _ := schema["properties"].(map[string]any)
	for name, raw := range props {
		if _, ok := out[name]; ok {
			continue
		}
		prop, _ := raw.(map[string]any)
		if v, ok := prop["default"]; ok {
			out[name] = v
		}
	}
	return out
}

var placeholder = regexp.MustCompile(`^\{([A-Za-z_][A-Za-z0-9_]*)(\?)?\}$`)

func expandArgs(template []string, input map[string]any) ([]string, error) {
	var out []string
	for _, a := range template {
		m := placeholder.FindStringSubmatch(a)
		if m == nil {
			out = append(out, a)
			continue
		}
		v, ok := input[m[1]]
		if !ok || v == nil || fmt.Sprint(v) == "" {
			if m[2] == "?" {
				continue
			}
			return nil, fmt.Errorf("missing required argument %q", m[1])
		}
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				out = append(out, fmt.Sprint(item))
			}
		default:
			out = append(out, fmt.Sprint(v))
		}
	}
	return out, nil
}

func resolveCommand(baseDir, command string) string {
	if command == "" || filepath.IsAbs(command) || !strings.Contains(command, string(filepath.Separator)) {
		return command
	}
	if baseDir == "" {
		return command
	}
	return filepath.Clean(filepath.Join(baseDir, command))
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
