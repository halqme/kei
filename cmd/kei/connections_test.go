package main

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/halqme/kei/internal/config"
)

func TestLoadSessionConfigGeneratesAvailableTargets(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmpDir, "state"))
	configPath := filepath.Join(tmpDir, "generated.json")
	t.Setenv("KEI_CONFIG", configPath)

	cfg, err := loadSessionConfig("")
	if err != nil {
		t.Fatalf("loadSessionConfig failed: %v", err)
	}
	if len(cfg.Providers) == 0 {
		t.Fatal("expected an available connection target")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("generated config is missing: %v", err)
	}
	var saved config.Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("generated config is invalid JSON: %v", err)
	}
	if len(saved.Providers) != len(cfg.Providers) || saved.Providers[0].Name != cfg.Providers[0].Name {
		t.Fatalf("generated targets do not match active targets: saved=%+v active=%+v", saved.Providers, cfg.Providers)
	}
}
