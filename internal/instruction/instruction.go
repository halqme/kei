package instruction

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const basePrompt = "You are a coding agent. Use tools when they help you complete the task."

func Load(workdir, skillCatalog string) (string, error) {
	parts := []string{basePrompt}
	if skillCatalog = strings.TrimSpace(skillCatalog); skillCatalog != "" {
		parts = append(parts, skillCatalog)
	}
	if workdir != "" {
		b, err := os.ReadFile(filepath.Join(workdir, "AGENTS.md"))
		switch {
		case err == nil:
			if text := strings.TrimSpace(string(b)); text != "" {
				parts = append(parts, text)
			}
		case errors.Is(err, fs.ErrNotExist):
		default:
			return "", err
		}
	}
	return strings.Join(parts, "\n\n"), nil
}
