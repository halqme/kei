package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/halqme/kei/internal/agent"
	"github.com/halqme/kei/internal/auth"
	"github.com/halqme/kei/internal/config"
	"github.com/halqme/kei/internal/control"
	"github.com/halqme/kei/internal/extension"
	"github.com/halqme/kei/internal/instruction"
	"github.com/halqme/kei/internal/provider"
	"github.com/halqme/kei/internal/skill"
)

func loadConfig(path string) (config.Config, error) { return config.Load(path) }

func discoverExtensions(cfg config.Config, workdir string) (*extension.Registry, error) {
	return extension.Discover(extension.SearchRoots(workdir, cfg.ExtensionDirs))
}

func makeSession(cfg config.Config, r *extension.Registry, id, workdir, providerOverride, modelOverride string) (*agent.Session, error) {
	if len(cfg.Providers) == 0 {
		cfg = withAvailableProviders(cfg)
	}
	pCfg, err := cfg.ResolveProvider(providerOverride, modelOverride)
	if err != nil {
		return nil, err
	}
	prov, err := provider.New(provider.Config{
		Type:      pCfg.Type,
		BaseURL:   pCfg.BaseURL,
		APIKeyEnv: pCfg.APIKeyEnv,
		Model:     pCfg.Model,
	})
	if err != nil {
		return nil, err
	}
	if err := requireProviderAuth(context.Background(), pCfg); err != nil {
		return nil, err
	}
	skills, err := skill.Discover(skill.SearchRoots(workdir))
	if err != nil {
		return nil, err
	}
	systemPrompt, err := instruction.Load(workdir, skills.CatalogPrompt())
	if err != nil {
		return nil, err
	}
	return &agent.Session{
		ID:           id,
		Tools:        r.Tools,
		Commands:     r.Commands,
		Skills:       skills,
		Controls:     control.New(cfg.Controls),
		SystemPrompt: systemPrompt,
		Workdir:      workdir,
		Provider:     prov,
	}, nil
}

func requireProviderAuth(ctx context.Context, p config.Provider) error {
	providerName := strings.ToLower(strings.TrimSpace(p.Type))
	if providerName == "" {
		return fmt.Errorf("provider target %q has no provider type", p.Name)
	}

	// Ollama is a local provider and does not have a login step.
	if providerName == "ollama" {
		return nil
	}
	if p.APIKeyEnv != "" && os.Getenv(p.APIKeyEnv) != "" {
		return nil
	}
	if ok, _ := auth.CheckAuth(ctx, providerName); ok {
		return nil
	}

	loginName := providerName
	switch providerName {
	case "openai-compatible":
		loginName = "openai"
	case "openai-codex":
		loginName = "codex"
	case "claude":
		loginName = "anthropic"
	case "google":
		loginName = "gemini"
	}
	if loginName == "azure" || loginName == "azure-openai" {
		return fmt.Errorf("provider %q is not authenticated; set its API key environment variable", providerName)
	}
	return fmt.Errorf("provider %q is not authenticated; run 'kei login %s' or set its API key environment variable", providerName, loginName)
}
