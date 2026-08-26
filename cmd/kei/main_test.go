package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(f func() error) (string, error) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := f()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), err
}

func TestHelpCmd(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		contain string
	}{
		{
			name:    "general help",
			args:    nil,
			wantErr: false,
			contain: "kei - Unix-native harness for coding agents",
		},
		{
			name:    "help run",
			args:    []string{"run"},
			wantErr: false,
			contain: "Usage: kei run",
		},
		{
			name:    "help models",
			args:    []string{"models"},
			wantErr: false,
			contain: "Usage: kei models",
		},
		{
			name:    "help extensions",
			args:    []string{"extensions"},
			wantErr: false,
			contain: "Usage: kei extensions",
		},
		{
			name:    "help tools",
			args:    []string{"tools"},
			wantErr: false,
			contain: "Usage: kei tools",
		},
		{
			name:    "help commands",
			args:    []string{"commands"},
			wantErr: false,
			contain: "Usage: kei commands",
		},
		{
			name:    "help exec",
			args:    []string{"exec"},
			wantErr: false,
			contain: "Usage: kei exec",
		},
		{
			name:    "help acp",
			args:    []string{"acp"},
			wantErr: false,
			contain: "Usage: kei acp",
		},
		{
			name:    "help login",
			args:    []string{"login"},
			wantErr: false,
			contain: "Usage: kei login",
		},
		{
			name:    "help version",
			args:    []string{"version"},
			wantErr: false,
			contain: "Usage: kei version",
		},
		{
			name:    "help help",
			args:    []string{"help"},
			wantErr: false,
			contain: "kei - Unix-native harness for coding agents",
		},
		{
			name:    "help unknown",
			args:    []string{"unknown-cmd"},
			wantErr: true,
			contain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := captureStdout(func() error {
				return helpCmd(tt.args)
			})
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.contain != "" && !strings.Contains(out, tt.contain) {
				t.Errorf("output does not contain %q, got: %s", tt.contain, out)
			}
		})
	}
}

func TestModelsCmd(t *testing.T) {
	exampleConfig := filepath.Join("..", "..", "examples", "config.example.json")

	t.Run("human readable output", func(t *testing.T) {
		out, err := captureStdout(func() error {
			return models([]string{"-config", exampleConfig})
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{
			"Configured Connection Targets:",
			"Available Connection Targets:",
			"Model Aliases:",
			"Supported Provider Types:",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q, got %s", want, out)
			}
		}
	})

	t.Run("json output", func(t *testing.T) {
		out, err := captureStdout(func() error {
			return models([]string{"-config", exampleConfig, "-json"})
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var parsed modelsOutput
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if len(parsed.ConfiguredProviders) != 5 || parsed.ConfiguredProviders[0].Name != "openai" {
			t.Errorf("unexpected configured providers: %+v", parsed.ConfiguredProviders)
		}
		if parsed.ConfiguredProviders[1].Name != "claude" || parsed.ConfiguredProviders[1].Type != "anthropic" {
			t.Errorf("unexpected second configured provider: %+v", parsed.ConfiguredProviders[1])
		}
		if parsed.Aliases["fast"] != "gpt-5.5-mini" {
			t.Errorf("expected fast alias to be gpt-5.5-mini, got %s", parsed.Aliases["fast"])
		}
		if len(parsed.AvailableProviders) == 0 {
			t.Errorf("expected available providers")
		}
		if len(parsed.SupportedProviderTypes) == 0 {
			t.Errorf("expected non-empty supported provider types")
		}
	})
}

func TestRunRequiresProviderAuthentication(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"providers":[{"name":"openai","type":"openai"}]}`), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	err := run([]string{"-config", configPath, "-p", "hello"})
	if err == nil {
		t.Fatal("expected unauthenticated provider error")
	}
	if !strings.Contains(err.Error(), "kei login openai") {
		t.Fatalf("expected login guidance, got %v", err)
	}

	interactiveErr := run([]string{"-config", configPath})
	if interactiveErr == nil || !strings.Contains(interactiveErr.Error(), "kei login openai") {
		t.Fatalf("expected interactive run to require authentication, got %v", interactiveErr)
	}
}

func TestRunInfoCommandDoesNotRequireProviderAuthentication(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmpDir, "state"))
	configPath := filepath.Join(tmpDir, "missing.json")
	t.Setenv("KEI_CONFIG", configPath)
	t.Setenv("OPENAI_API_KEY", "")

	out, err := captureStdout(func() error {
		return run([]string{"-p", "/help"})
	})
	if err != nil {
		t.Fatalf("expected /help to work without authentication: %v", err)
	}
	if !strings.Contains(out, "Interactive REPL Commands:") {
		t.Fatalf("expected REPL help output, got %s", out)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("info command created a config: %v", err)
	}
}

func TestRunReportsUnknownProvider(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmpDir, "state"))
	t.Setenv("KEI_CONFIG", filepath.Join(tmpDir, "missing.json"))

	err := run([]string{"-provider", "not-a-provider", "-p", "hello"})
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
	if !strings.Contains(err.Error(), "unknown provider target") {
		t.Fatalf("expected unknown provider target error, got %v", err)
	}
}
