package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Provider describes a named connection target and its provider API settings.
type Provider struct {
	Name      string   `json:"name"`
	Type      string   `json:"type,omitempty"` // "openai", "codex", "anthropic", "gemini", "ollama", "azure"
	BaseURL   string   `json:"base_url,omitempty"`
	APIKeyEnv string   `json:"api_key_env,omitempty"`
	Model     string   `json:"model,omitempty"`
	Models    []string `json:"models,omitempty"` // list of available models for this provider
}

type Control struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type Config struct {
	// Providers is an ordered list of named connection targets. The first
	// target is used when no connection target override is supplied.
	Providers []Provider `json:"providers,omitempty"`
	// Models provides shortcuts/aliases (e.g. "fast": "gpt-5.5-mini", "smart": "claude-3-7-sonnet").
	Models map[string]string `json:"models,omitempty"`
	// ExtensionDirs are additional extension roots searched after the
	// workspace and standard XDG data locations.
	ExtensionDirs []string  `json:"extension_dirs,omitempty"`
	Controls      []Control `json:"controls,omitempty"`
	SystemPrompt  string    `json:"system_prompt,omitempty"`
}

func Default() Config {
	return Config{
		SystemPrompt: "You are a coding agent. Use tools when they help you complete the task.",
	}
}

// ResolveProvider returns the selected connection target and model override.
func (c *Config) ResolveProvider(providerName, modelOverride string) (Provider, error) {
	var p Provider
	if providerName == "" {
		if len(c.Providers) == 0 {
			return Provider{}, fmt.Errorf("no provider targets configured")
		}
		p = c.Providers[0]
	} else {
		name := strings.ToLower(strings.TrimSpace(providerName))
		for _, candidate := range c.Providers {
			candidateName := strings.ToLower(strings.TrimSpace(candidate.Name))
			if candidateName == "" {
				candidateName = strings.ToLower(strings.TrimSpace(candidate.Type))
			}
			if candidateName == name {
				p = candidate
				break
			}
		}
		if p.Name == "" && p.Type == "" {
			names := make([]string, 0, len(c.Providers))
			for _, candidate := range c.Providers {
				candidateName := strings.TrimSpace(candidate.Name)
				if candidateName == "" {
					candidateName = strings.TrimSpace(candidate.Type)
				}
				if candidateName != "" {
					names = append(names, candidateName)
				}
			}
			if len(names) == 0 {
				return Provider{}, fmt.Errorf("unknown provider target %q (no configured targets)", providerName)
			}
			return Provider{}, fmt.Errorf("unknown provider target %q (configured: %s)", providerName, strings.Join(names, ", "))
		}
	}

	if p.Name == "" {
		p.Name = p.Type
	}
	if modelOverride != "" {
		if target, ok := c.Models[modelOverride]; ok {
			modelOverride = target
		}
		p.Model = modelOverride
	}

	if p.Model == "" && len(p.Models) > 0 {
		p.Model = p.Models[0]
	}
	if strings.TrimSpace(p.Type) == "" {
		return Provider{}, fmt.Errorf("provider target %q has no provider type", p.Name)
	}
	return p, nil
}

func DefaultConfigPaths() []string {
	var paths []string
	add := func(path string) {
		if path == "" {
			return
		}
		for _, existing := range paths {
			if existing == path {
				return
			}
		}
		paths = append(paths, path)
	}

	add(os.Getenv("KEI_CONFIG"))
	add(filepath.Join(".kei", "config.json"))
	if configDir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(configDir) {
		add(filepath.Join(configDir, "kei", "config.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".config", "kei", "config.json"))
	}
	return paths
}

// LoadOrCreate loads a configuration or creates one at the default user path.
// An explicit path is never created implicitly.
func LoadOrCreate(path string, defaults Config) (Config, error) {
	cfg, found, err := loadConfig(path)
	if err != nil {
		return Config{}, err
	}
	if found {
		return cfg, nil
	}
	if path != "" {
		return Config{}, fmt.Errorf("config %s was not found", path)
	}
	if defaults.SystemPrompt == "" {
		defaults.SystemPrompt = Default().SystemPrompt
	}
	path, err = DefaultConfigPath()
	if err != nil {
		return Config{}, err
	}
	if err := Save(path, defaults); err != nil {
		return Config{}, fmt.Errorf("failed to create config %s: %w", path, err)
	}
	return defaults, nil
}

// DefaultConfigPath returns the user configuration path used for new files.
func DefaultConfigPath() (string, error) {
	if p := os.Getenv("KEI_CONFIG"); p != "" {
		return p, nil
	}
	if configDir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(configDir) {
		return filepath.Join(configDir, "kei", "config.json"), nil
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "kei", "config.json"), nil
	}
	return "", errors.New("could not determine a configuration path")
}

// Save writes a configuration file with owner-only permissions.
func Save(path string, cfg Config) error {
	if path == "" {
		return errors.New("configuration path is empty")
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return os.WriteFile(path, b, 0600)
}

func loadConfig(path string) (Config, bool, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Config{}, false, fmt.Errorf("failed to read config %s: %w", path, err)
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Config{}, false, fmt.Errorf("failed to parse config %s: %w", path, err)
		}
		return cfg, true, nil
	}

	for _, p := range DefaultConfigPaths() {
		b, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Config{}, false, err
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Config{}, false, fmt.Errorf("failed to parse config %s: %w", p, err)
		}
		return cfg, true, nil
	}

	return cfg, false, nil
}

func Load(path string) (Config, error) {
	cfg, _, err := loadConfig(path)
	return cfg, err
}
