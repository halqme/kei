package config

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateGeneratesAndPreservesOrderedProviders(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "xdg-config"))
	configPath := filepath.Join(tmpDir, "generated", "config.json")
	t.Setenv("KEI_CONFIG", configPath)

	defaults := Config{
		Providers: []Provider{
			{Name: "first", Type: "ollama"},
			{Name: "second", Type: "openai"},
		},
		SystemPrompt: "generated prompt",
	}
	got, err := LoadOrCreate("", defaults)
	if err != nil {
		t.Fatalf("LoadOrCreate failed: %v", err)
	}
	if len(got.Providers) != 2 || got.Providers[0].Name != "first" || got.Providers[1].Name != "second" {
		t.Fatalf("unexpected generated providers: %+v", got.Providers)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("generated config is missing: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("generated config mode = %04o, want 0600", mode)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var saved Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("generated config is invalid JSON: %v", err)
	}
	if saved.Providers[0].Name != "first" || saved.Providers[1].Name != "second" {
		t.Fatalf("generated provider order was not preserved: %+v", saved.Providers)
	}

	got, err = LoadOrCreate("", Config{Providers: []Provider{{Name: "replacement", Type: "gemini"}}})
	if err != nil {
		t.Fatalf("LoadOrCreate second call failed: %v", err)
	}
	if got.Providers[0].Name != "first" {
		t.Fatalf("existing config was overwritten: %+v", got.Providers)
	}
}

func TestLoadOrCreateDoesNotCreateExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.json")
	if _, err := LoadOrCreate(path, Config{Providers: []Provider{{Name: "local", Type: "ollama"}}}); err == nil {
		t.Fatal("expected an explicit missing path to fail")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("explicit missing path was created: %v", err)
	}
}

func TestDefaultConfigPathUsesXDGConfigHome(t *testing.T) {
	t.Run("explicit XDG directory", func(t *testing.T) {
		xdgHome := filepath.Join(t.TempDir(), "xdg")
		t.Setenv("KEI_CONFIG", "")
		t.Setenv("XDG_CONFIG_HOME", xdgHome)

		path, err := DefaultConfigPath()
		if err != nil {
			t.Fatalf("DefaultConfigPath failed: %v", err)
		}
		expected := filepath.Join(xdgHome, "kei", "config.json")
		if path != expected {
			t.Fatalf("unexpected config path: got %s, want %s", path, expected)
		}
	})

	t.Run("home config directory fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("KEI_CONFIG", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", home)

		path, err := DefaultConfigPath()
		if err != nil {
			t.Fatalf("DefaultConfigPath failed: %v", err)
		}
		expected := filepath.Join(home, ".config", "kei", "config.json")
		if path != expected {
			t.Fatalf("unexpected config path: got %s, want %s", path, expected)
		}
	})
}
