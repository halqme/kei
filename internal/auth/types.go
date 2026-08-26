package auth

import (
	"context"
	"time"
)

// Credentials represents unified authentication credentials across all providers.
type Credentials struct {
	AccessToken  string            `json:"access_token,omitempty"`
	RefreshToken string            `json:"refresh_token,omitempty"`
	IDToken      string            `json:"id_token,omitempty"`
	AccountID    string            `json:"account_id,omitempty"`
	APIKey       string            `json:"api_key,omitempty"`
	ExpiresAt    time.Time         `json:"expires_at,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
}

type LoginOptions struct {
	Device  bool
	Browser bool
	OutPath string
	Notify  func(msg string)
}

type Authenticator interface {
	Name() string
	Description() string
	DefaultSavePath() (string, error)
	Login(ctx context.Context, opts LoginOptions) (*Credentials, error)
}
