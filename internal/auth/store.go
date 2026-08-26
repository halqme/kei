package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu        sync.Mutex
	filePath  string
	client    *http.Client
	providers map[string]*Credentials
}

var (
	defaultStoreOnce sync.Once
	defaultStore     *Store
)

func GetDefaultStore() *Store {
	defaultStoreOnce.Do(func() {
		defaultStore = NewStore(nil)
	})
	return defaultStore
}

func NewStore(client *http.Client) *Store {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Store{
		client:    client,
		providers: map[string]*Credentials{},
	}
}

// DefaultKeiAuthSavePath returns the primary write target for authentication credentials.
// Adheres to $XDG_STATE_HOME/kei/auth.json (~/.local/state/kei/auth.json).
func DefaultKeiAuthSavePath() string {
	if custom := os.Getenv("KEI_AUTH_FILE"); custom != "" {
		return custom
	}
	if stateDir := os.Getenv("XDG_STATE_HOME"); stateDir != "" {
		return filepath.Join(stateDir, "kei", "auth.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "kei", "auth.json")
	}
	return "auth.json"
}

// DefaultKeiAuthSearchPaths returns candidate paths for loading credentials.
func DefaultKeiAuthSearchPaths() []string {
	var paths []string
	if custom := os.Getenv("KEI_AUTH_FILE"); custom != "" {
		paths = append(paths, custom)
	}

	// 1. $XDG_STATE_HOME/kei/auth.json
	if stateDir := os.Getenv("XDG_STATE_HOME"); stateDir != "" {
		paths = append(paths, filepath.Join(stateDir, "kei", "auth.json"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".local", "state", "kei", "auth.json"))
	}

	// 2. $XDG_DATA_HOME/kei/auth.json
	if dataDir := os.Getenv("XDG_DATA_HOME"); dataDir != "" {
		paths = append(paths, filepath.Join(dataDir, "kei", "auth.json"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".local", "share", "kei", "auth.json"))
	}

	return paths
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range DefaultKeiAuthSearchPaths() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		var root struct {
			Providers map[string]*Credentials `json:"providers"`
		}
		if err := json.Unmarshal(data, &root); err == nil && root.Providers != nil {
			s.filePath = p
			s.providers = root.Providers
			return nil
		}

		var flat map[string]*Credentials
		if err := json.Unmarshal(data, &flat); err == nil && len(flat) > 0 {
			s.filePath = p
			s.providers = flat
			return nil
		}
	}

	s.filePath = DefaultKeiAuthSavePath()
	return nil
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.filePath
	if path == "" {
		path = DefaultKeiAuthSavePath()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	root := map[string]any{
		"providers": s.providers,
	}

	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func (s *Store) SetCredential(providerName string, creds *Credentials) error {
	_ = s.Load()
	s.mu.Lock()
	s.providers[strings.ToLower(providerName)] = creds
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) GetCredential(ctx context.Context, providerName string) (*Credentials, error) {
	name := strings.ToLower(providerName)
	_ = s.Load()

	s.mu.Lock()
	creds := s.providers[name]
	s.mu.Unlock()

	if creds != nil {
		return creds, nil
	}

	// Provider specific fallback loaders (e.g. ~/.codex/auth.json for Codex)
	if name == "codex" || name == "openai-codex" {
		return loadCodexNativeCredentials()
	}

	return nil, errors.New("no credentials found")
}

// CheckAuth checks whether credentials exist for providerName and returns auth status & source description.
func CheckAuth(ctx context.Context, providerName string) (bool, string) {
	name := strings.ToLower(providerName)
	switch name {
	case "codex", "openai-codex":
		if token := os.Getenv("CODEX_ACCESS_TOKEN"); token != "" {
			return true, "env: CODEX_ACCESS_TOKEN"
		}
		if token := os.Getenv("OPENAI_CODEX_TOKEN"); token != "" {
			return true, "env: OPENAI_CODEX_TOKEN"
		}
		if creds, err := GetDefaultStore().GetCredential(ctx, "codex"); err == nil && creds != nil && creds.AccessToken != "" {
			return true, "login"
		}
		return false, ""
	case "anthropic", "claude":
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			return true, "env: ANTHROPIC_API_KEY"
		}
		if creds, err := GetDefaultStore().GetCredential(ctx, "anthropic"); err == nil && creds != nil && creds.APIKey != "" {
			return true, "login"
		}
		return false, ""
	case "gemini", "google":
		if os.Getenv("GEMINI_API_KEY") != "" {
			return true, "env: GEMINI_API_KEY"
		}
		if os.Getenv("GOOGLE_API_KEY") != "" {
			return true, "env: GOOGLE_API_KEY"
		}
		if creds, err := GetDefaultStore().GetCredential(ctx, "gemini"); err == nil && creds != nil && creds.APIKey != "" {
			return true, "login"
		}
		return false, ""
	case "openai", "openai-compatible":
		if os.Getenv("OPENAI_API_KEY") != "" {
			return true, "env: OPENAI_API_KEY"
		}
		if creds, err := GetDefaultStore().GetCredential(ctx, "openai"); err == nil && creds != nil && creds.APIKey != "" {
			return true, "login"
		}
		return false, ""
	case "azure", "azure-openai":
		if os.Getenv("AZURE_OPENAI_API_KEY") != "" {
			return true, "env: AZURE_OPENAI_API_KEY"
		}
		if creds, err := GetDefaultStore().GetCredential(ctx, "azure"); err == nil && creds != nil && creds.APIKey != "" {
			return true, "login"
		}
		return false, ""
	case "ollama":
		// Ollama runs locally and does not require credentials.
		return true, "local"
	default:
		if creds, err := GetDefaultStore().GetCredential(ctx, name); err == nil && creds != nil {
			return true, "login"
		}
		return false, ""
	}
}

// loadCodexNativeCredentials checks ~/.codex/auth.json or env vars.
func loadCodexNativeCredentials() (*Credentials, error) {
	if token := os.Getenv("CODEX_ACCESS_TOKEN"); token != "" {
		accountID := os.Getenv("CODEX_ACCOUNT_ID")
		if accountID == "" {
			accountID = extractCodexAccountID(token)
		}
		return &Credentials{
			AccessToken: token,
			AccountID:   accountID,
		}, nil
	}
	if token := os.Getenv("OPENAI_CODEX_TOKEN"); token != "" {
		accountID := os.Getenv("CODEX_ACCOUNT_ID")
		if accountID == "" {
			accountID = extractCodexAccountID(token)
		}
		return &Credentials{
			AccessToken: token,
			AccountID:   accountID,
		}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	codexPath := filepath.Join(home, ".codex", "auth.json")
	data, err := os.ReadFile(codexPath)
	if err != nil {
		return nil, err
	}

	return parseCodexAuthJSON(data)
}

func parseCodexAuthJSON(data []byte) (*Credentials, error) {
	var raw struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			IDToken      string `json:"id_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
		PersonalAccessToken string `json:"personal_access_token"`
		AccessToken         string `json:"access_token"`
		RefreshToken        string `json:"refresh_token"`
		IDToken             string `json:"id_token"`
		AccountID           string `json:"account_id"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	creds := &Credentials{}
	if raw.Tokens.AccessToken != "" {
		creds.AccessToken = raw.Tokens.AccessToken
		creds.RefreshToken = raw.Tokens.RefreshToken
		creds.IDToken = raw.Tokens.IDToken
		creds.AccountID = raw.Tokens.AccountID
	} else if raw.AccessToken != "" {
		creds.AccessToken = raw.AccessToken
		creds.RefreshToken = raw.RefreshToken
		creds.IDToken = raw.IDToken
		creds.AccountID = raw.AccountID
	} else if raw.PersonalAccessToken != "" {
		creds.AccessToken = raw.PersonalAccessToken
	}

	if creds.AccountID == "" && creds.AccessToken != "" {
		creds.AccountID = extractCodexAccountID(creds.AccessToken)
	}

	return creds, nil
}

func extractCodexAccountID(token string) string {
	claims, err := ExtractJWTClaims(token)
	if err != nil {
		return ""
	}
	if authMap, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if id, ok := authMap["chatgpt_account_id"].(string); ok && id != "" {
			return id
		}
	}
	if id, ok := claims["chatgpt_account_id"].(string); ok && id != "" {
		return id
	}
	return ""
}
