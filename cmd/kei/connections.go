package main

import (
	"context"

	"github.com/halqme/kei/internal/auth"
	"github.com/halqme/kei/internal/config"
	"github.com/halqme/kei/internal/provider"
)

func availableProviderConfigs(ctx context.Context) []config.Provider {
	names := provider.List()
	providers := make([]config.Provider, 0, len(names))
	for _, name := range names {
		ok, _ := auth.CheckAuth(ctx, name)
		if !ok {
			continue
		}
		providers = append(providers, config.Provider{Name: name, Type: name})
	}
	return providers
}

func loadSessionConfig(path string) (config.Config, error) {
	available := availableProviderConfigs(context.Background())
	cfg, err := config.LoadOrCreate(path, config.Config{
		Providers:    available,
		SystemPrompt: config.Default().SystemPrompt,
	})
	if err != nil {
		return config.Config{}, err
	}
	if len(cfg.Providers) == 0 {
		cfg.Providers = available
	}
	return cfg, nil
}

func withAvailableProviders(cfg config.Config) config.Config {
	if len(cfg.Providers) == 0 {
		cfg.Providers = availableProviderConfigs(context.Background())
	}
	return cfg
}
