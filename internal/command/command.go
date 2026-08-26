package command

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Descriptor struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	InputHint   string   `json:"input_hint,omitempty"`
	Command     string   `json:"command"`
	Args        []string `json:"args,omitempty"`
	Stdin       string   `json:"stdin,omitempty"` // "text" or empty
	TimeoutMS   int      `json:"timeout_ms,omitempty"`

	ExtensionID   string `json:"extension_id,omitempty"`
	QualifiedName string `json:"qualified_name,omitempty"`
	BaseDir       string `json:"-"`
}

type Registry struct{ commands map[string]Descriptor }

func NewRegistry(ds []Descriptor) (*Registry, error) {
	r := &Registry{commands: map[string]Descriptor{}}
	for _, d := range ds {
		if d.Name == "" || d.Command == "" {
			return nil, fmt.Errorf("command name and command are required")
		}
		if d.QualifiedName == "" {
			d.QualifiedName = d.Name
		}
		if _, ok := r.commands[d.QualifiedName]; ok {
			return nil, fmt.Errorf("duplicate command %q", d.QualifiedName)
		}
		r.commands[d.QualifiedName] = d
	}
	return r, nil
}

func (r *Registry) List() []Descriptor {
	out := make([]Descriptor, 0, len(r.commands))
	for _, d := range r.commands {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QualifiedName < out[j].QualifiedName })
	return out
}

func (r *Registry) Get(name string) (Descriptor, bool) {
	d, ok := r.commands[name]
	return d, ok
}

func ParseInvocation(text string) (name, arguments string, ok bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	text = strings.TrimPrefix(text, "/")
	if text == "" {
		return "", "", false
	}
	name, arguments, _ = strings.Cut(text, " ")
	return name, strings.TrimSpace(arguments), true
}

func Execute(ctx context.Context, cwd string, d Descriptor, arguments string) (string, error) {
	timeout := 60 * time.Second
	if d.TimeoutMS > 0 {
		timeout = time.Duration(d.TimeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]string, 0, len(d.Args))
	for _, a := range d.Args {
		switch a {
		case "{arguments}":
			if arguments == "" {
				return "", fmt.Errorf("command /%s requires arguments", d.QualifiedName)
			}
			args = append(args, arguments)
		case "{arguments?}":
			if arguments != "" {
				args = append(args, arguments)
			}
		default:
			args = append(args, a)
		}
	}

	command := resolveCommand(d.BaseDir, d.Command)
	cmd := exec.CommandContext(ctx, command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if d.Stdin == "text" {
		cmd.Stdin = strings.NewReader(arguments)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
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

func resolveCommand(baseDir, command string) string {
	if command == "" || filepath.IsAbs(command) || !strings.Contains(command, string(filepath.Separator)) {
		return command
	}
	if baseDir == "" {
		return command
	}
	return filepath.Clean(filepath.Join(baseDir, command))
}
