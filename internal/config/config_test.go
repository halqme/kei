package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProvider(t *testing.T) {
	cfg := Config{
		Providers: []Provider{
			{
				Name:   "openai",
				Type:   "openai",
				Model:  "gpt-5.6",
				Models: []string{"gpt-5.6", "gpt-5.5"},
			},
			{
				Name:   "claude",
				Type:   "anthropic",
				Model:  "claude-3-7-sonnet-20250219",
				Models: []string{"claude-3-7-sonnet-20250219", "claude-3-5-haiku-20241022"},
			},
		},
		Models: map[string]string{
			"fast":  "gpt-5.5",
			"smart": "claude-3-7-sonnet-20250219",
		},
	}

	// The first configured target is used without an override.
	p, err := cfg.ResolveProvider("", "")
	if err != nil {
		t.Fatalf("ResolveProvider failed: %v", err)
	}
	if p.Name != "openai" || p.Type != "openai" || p.Model != "gpt-5.6" {
		t.Errorf("unexpected first provider: %+v", p)
	}

	pClaude, err := cfg.ResolveProvider("claude", "")
	if err != nil {
		t.Fatalf("ResolveProvider(claude) failed: %v", err)
	}
	if pClaude.Name != "claude" || pClaude.Type != "anthropic" || pClaude.Model != "claude-3-7-sonnet-20250219" {
		t.Errorf("unexpected claude provider: %+v", pClaude)
	}

	pFast, err := cfg.ResolveProvider("", "fast")
	if err != nil {
		t.Fatalf("ResolveProvider('', 'fast') failed: %v", err)
	}
	if pFast.Name != "openai" || pFast.Model != "gpt-5.5" {
		t.Errorf("unexpected fast target: %+v", pFast)
	}

	pOverride, err := cfg.ResolveProvider("claude", "claude-3-5-haiku-20241022")
	if err != nil {
		t.Fatalf("ResolveProvider override failed: %v", err)
	}
	if pOverride.Name != "claude" || pOverride.Model != "claude-3-5-haiku-20241022" {
		t.Errorf("unexpected model override: %+v", pOverride)
	}

	empty := Config{}
	if _, err := empty.ResolveProvider("", ""); err == nil {
		t.Fatal("expected an empty provider list to fail")
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")

	content := `{
		"providers": [
			{
				"name": "codex",
				"type": "codex",
				"model": "gpt-5.5",
				"models": ["gpt-5.5", "gpt-5.5-mini"]
			}
		],
		"models": {
			"mini": "gpt-5.5-mini"
		}
	}`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "codex" || cfg.Providers[0].Type != "codex" || cfg.Providers[0].Model != "gpt-5.5" {
		t.Errorf("unexpected loaded providers: %+v", cfg.Providers)
	}
	if len(cfg.Providers[0].Models) != 2 || cfg.Models["mini"] != "gpt-5.5-mini" {
		t.Errorf("unexpected models: %+v, map: %+v", cfg.Providers[0].Models, cfg.Models)
	}
}
