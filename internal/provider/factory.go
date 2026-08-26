package provider

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/halqme/kei/internal/auth"
)

type Config struct {
	Type      string
	BaseURL   string
	APIKeyEnv string
	APIKey    string
	Model     string
}

type FactoryFunc func(cfg Config) (Provider, error)

var (
	registryMu sync.RWMutex
	factories  = map[string]FactoryFunc{}
)

func Register(name string, f FactoryFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	factories[strings.ToLower(name)] = f
}

func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	seen := map[string]struct{}{}
	for name := range factories {
		switch name {
		case "openai-compatible":
			name = "openai"
		case "openai-codex":
			name = "codex"
		case "claude":
			name = "anthropic"
		case "google":
			name = "gemini"
		case "azure-openai":
			name = "azure"
		}
		seen[name] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for name := range seen {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

func resolveKey(providerName string, cfg Config, defaultEnv string) string {
	if cfg.APIKey != "" {
		return cfg.APIKey
	}
	if cfg.APIKeyEnv != "" {
		if k := os.Getenv(cfg.APIKeyEnv); k != "" {
			return k
		}
	}
	if defaultEnv != "" {
		if k := os.Getenv(defaultEnv); k != "" {
			return k
		}
	}
	if creds, err := auth.GetDefaultStore().GetCredential(context.Background(), providerName); err == nil && creds != nil {
		if creds.APIKey != "" {
			return creds.APIKey
		}
		if creds.AccessToken != "" {
			return creds.AccessToken
		}
	}
	return ""
}

func init() {
	Register("openai", func(cfg Config) (Provider, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		model := cfg.Model
		if model == "" {
			model = "gpt-5.6"
		}
		apiKey := resolveKey("openai", cfg, "OPENAI_API_KEY")
		return &OpenAICompatible{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   model,
		}, nil
	})

	Register("openai-compatible", func(cfg Config) (Provider, error) {
		f, _ := getFactory("openai")
		return f(cfg)
	})

	Register("codex", func(cfg Config) (Provider, error) {
		apiKey := resolveKey("codex", cfg, "CODEX_ACCESS_TOKEN")
		return &Codex{
			BaseURL:     cfg.BaseURL,
			Model:       cfg.Model,
			StaticToken: apiKey,
		}, nil
	})

	Register("openai-codex", func(cfg Config) (Provider, error) {
		f, _ := getFactory("codex")
		return f(cfg)
	})

	Register("anthropic", func(cfg Config) (Provider, error) {
		apiKey := resolveKey("anthropic", cfg, "ANTHROPIC_API_KEY")
		return &Anthropic{
			BaseURL: cfg.BaseURL,
			APIKey:  apiKey,
			Model:   cfg.Model,
		}, nil
	})

	Register("claude", func(cfg Config) (Provider, error) {
		f, _ := getFactory("anthropic")
		return f(cfg)
	})

	Register("gemini", func(cfg Config) (Provider, error) {
		apiKey := resolveKey("gemini", cfg, "GEMINI_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("GOOGLE_API_KEY")
		}
		return &Gemini{
			BaseURL: cfg.BaseURL,
			APIKey:  apiKey,
			Model:   cfg.Model,
		}, nil
	})

	Register("google", func(cfg Config) (Provider, error) {
		f, _ := getFactory("gemini")
		return f(cfg)
	})

	Register("ollama", func(cfg Config) (Provider, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			if h := os.Getenv("OLLAMA_HOST"); h != "" {
				baseURL = strings.TrimRight(h, "/") + "/v1"
			} else {
				baseURL = "http://localhost:11434/v1"
			}
		}
		model := cfg.Model
		if model == "" {
			model = "llama3.3"
		}
		return &OpenAICompatible{
			BaseURL: baseURL,
			APIKey:  "ollama",
			Model:   model,
		}, nil
	})

	Register("azure", func(cfg Config) (Provider, error) {
		apiKey := resolveKey("azure", cfg, "AZURE_OPENAI_API_KEY")
		return &OpenAICompatible{
			BaseURL: cfg.BaseURL,
			APIKey:  apiKey,
			Model:   cfg.Model,
		}, nil
	})

	Register("azure-openai", func(cfg Config) (Provider, error) {
		f, _ := getFactory("azure")
		return f(cfg)
	})
}

func getFactory(name string) (FactoryFunc, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := factories[strings.ToLower(name)]
	return f, ok
}

func New(cfg Config) (Provider, error) {
	providerType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if providerType == "" {
		return nil, fmt.Errorf("provider type is required (available: %s)", strings.Join(List(), ", "))
	}

	factory, ok := getFactory(providerType)
	if !ok {
		return nil, fmt.Errorf("unsupported provider type %q (available: %s)", cfg.Type, strings.Join(List(), ", "))
	}

	return factory(cfg)
}
