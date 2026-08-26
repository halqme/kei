package auth

import (
	"context"
	"fmt"
)

const (
	CodexClientID              = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexAuthorizeURL          = "https://auth.openai.com/oauth/authorize"
	CodexTokenURL              = "https://auth.openai.com/oauth/token"
	CodexRedirectURI           = "http://localhost:1455/auth/callback"
	CodexDeviceUserCodeURL     = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	CodexDeviceTokenURL        = "https://auth.openai.com/api/accounts/deviceauth/token"
	CodexDeviceAuthURL         = "https://auth.openai.com/codex/device"
	CodexDeviceRedirectURI     = "https://auth.openai.com/deviceauth/callback"
	CodexScope                 = "openid profile email offline_access"
)

var codexOAuthConfig = OAuthConfig{
	ClientID:          CodexClientID,
	AuthorizeURL:      CodexAuthorizeURL,
	TokenURL:          CodexTokenURL,
	RedirectURI:       CodexRedirectURI,
	Port:              1455,
	Scope:             CodexScope,
	DeviceUserCodeURL: CodexDeviceUserCodeURL,
	DeviceTokenURL:    CodexDeviceTokenURL,
	DeviceAuthURL:     CodexDeviceAuthURL,
	DeviceRedirectURI: CodexDeviceRedirectURI,
	ExtraAuthorizeParams: map[string]string{
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"originator":                 "kei",
	},
}

type CodexAuthenticator struct{}

func (a *CodexAuthenticator) Name() string {
	return "codex"
}

func (a *CodexAuthenticator) Description() string {
	return "OpenAI ChatGPT Plus/Pro OAuth authentication"
}

func (a *CodexAuthenticator) DefaultSavePath() (string, error) {
	return DefaultKeiAuthSavePath(), nil
}

func (a *CodexAuthenticator) Login(ctx context.Context, opts LoginOptions) (*Credentials, error) {
	var creds *Credentials
	var err error

	if opts.Device {
		if opts.Notify != nil {
			opts.Notify("Starting device code login...")
		}
		creds, err = RunDeviceCodeFlow(ctx, codexOAuthConfig, func(userCode, verificationURI string) {
			if opts.Notify != nil {
				opts.Notify(fmt.Sprintf("\nTo authenticate, open:\n  %s\nand enter the code:\n  %s\n\nWaiting for authorization...\n", verificationURI, userCode))
			}
		})
	} else {
		if opts.Notify != nil {
			opts.Notify("Starting browser login on localhost:1455...")
		}
		creds, err = RunBrowserFlow(ctx, codexOAuthConfig, func(authURL string) {
			if opts.Notify != nil {
				opts.Notify(fmt.Sprintf("\nPlease open the following URL in your browser to log in:\n  %s\n\nWaiting for callback on http://localhost:1455/auth/callback ...\n", authURL))
			}
		})
		if err != nil && !opts.Browser {
			if opts.Notify != nil {
				opts.Notify("Browser callback listener could not be completed, falling back to device code login...")
			}
			creds, err = RunDeviceCodeFlow(ctx, codexOAuthConfig, func(userCode, verificationURI string) {
				if opts.Notify != nil {
					opts.Notify(fmt.Sprintf("\nTo authenticate, open:\n  %s\nand enter the code:\n  %s\n\nWaiting for authorization...\n", verificationURI, userCode))
				}
			})
		}
	}

	if err != nil {
		return nil, err
	}

	if creds.AccountID == "" && creds.AccessToken != "" {
		creds.AccountID = extractCodexAccountID(creds.AccessToken)
	}

	if err := GetDefaultStore().SetCredential("codex", creds); err != nil {
		return nil, err
	}

	return creds, nil
}

func init() {
	Register("codex", &CodexAuthenticator{})
	Register("openai-codex", &CodexAuthenticator{})
}
