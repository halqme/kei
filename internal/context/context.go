package context

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/halqme/kei/internal/transcript"
)

const basePrompt = "You are a coding agent. Use tools when they help you complete the task."

type Builder struct {
	baseInstructions string
}

type Request struct {
	Instructions string
	Tail         []transcript.Entry
	Tools        []map[string]any
}

func New(baseInstructions string) *Builder {
	return &Builder{baseInstructions: strings.TrimSpace(baseInstructions)}
}

func NewForWorkspace(workdir, skillCatalog string) (*Builder, error) {
	parts := []string{basePrompt}
	if workdir != "" {
		b, err := os.ReadFile(filepath.Join(workdir, "AGENTS.md"))
		switch {
		case err == nil:
			if text := strings.TrimSpace(string(b)); text != "" {
				parts = append(parts, text)
			}
		case errors.Is(err, fs.ErrNotExist):
		default:
			return nil, err
		}
	}
	if skillCatalog = strings.TrimSpace(skillCatalog); skillCatalog != "" {
		parts = append(parts, skillCatalog)
	}
	return New(strings.Join(parts, "\n\n")), nil
}

func (b *Builder) BaseInstructions() string {
	if b == nil {
		return ""
	}
	return b.baseInstructions
}

func (b *Builder) Materialize(tail []transcript.Entry, tools []map[string]any, instructions string) Request {
	if instructions == "" {
		instructions = b.BaseInstructions()
	}

	return Request{
		Instructions: instructions,
		Tail:         cloneTail(tail),
		Tools:        append([]map[string]any(nil), tools...),
	}
}

func cloneTail(tail []transcript.Entry) []transcript.Entry {
	out := make([]transcript.Entry, len(tail))
	for i, entry := range tail {
		out[i] = entry
		out[i].ToolCalls = append([]transcript.ToolCall(nil), entry.ToolCalls...)
	}
	return out
}
