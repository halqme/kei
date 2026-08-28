package context

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/halqme/kei/internal/provider"
)

const basePrompt = "You are a coding agent. Use tools when they help you complete the task."

type Builder struct {
	baseInstructions string
}

type Materialized struct {
	Messages []provider.Message
	Tools    []map[string]any
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

func (b *Builder) Materialize(tail []provider.Message, tools []map[string]any, instructions string) Materialized {
	if instructions == "" {
		instructions = b.BaseInstructions()
	}

	messages := make([]provider.Message, 0, len(tail)+1)
	if instructions != "" {
		messages = append(messages, provider.Message{Role: "system", Content: instructions})
	}
	messages = append(messages, tail...)

	return Materialized{
		Messages: messages,
		Tools:    append([]map[string]any(nil), tools...),
	}
}
