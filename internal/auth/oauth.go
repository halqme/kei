package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthConfig defines endpoint parameters for standard OAuth 2.0 PKCE and Device Code flows.
type OAuthConfig struct {
	ClientID             string
	AuthorizeURL         string
	TokenURL             string
	RedirectURI          string
	Port                 int
	Scope                string
	DeviceUserCodeURL    string
	DeviceTokenURL       string
	DeviceAuthURL        string
	DeviceRedirectURI    string
	ExtraAuthorizeParams map[string]string
}

// GeneratePKCE creates a PKCE code verifier and S256 code challenge.
func GeneratePKCE() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

func generateState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// ExchangeOAuthCode exchanges an authorization code for access and refresh tokens.
func ExchangeOAuthCode(ctx context.Context, client *http.Client, tokenURL, clientID, code, verifier, redirectURI string) (*Credentials, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (%s): %s", resp.Status, string(b))
	}

	var tr tokenResponse
	if err := json.UnmarshalRead(resp.Body, &tr); err != nil {
		return nil, err
	}

	creds := &Credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		IDToken:      tr.IDToken,
	}
	if tr.ExpiresIn > 0 {
		creds.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return creds, nil
}

// RefreshOAuthToken refreshes an expired access token using the refresh token.
func RefreshOAuthToken(ctx context.Context, client *http.Client, tokenURL, clientID, refreshToken string) (*Credentials, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (%s): %s", resp.Status, string(b))
	}

	var tr tokenResponse
	if err := json.UnmarshalRead(resp.Body, &tr); err != nil {
		return nil, err
	}

	creds := &Credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		IDToken:      tr.IDToken,
	}
	if tr.ExpiresIn > 0 {
		creds.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return creds, nil
}

// RunBrowserFlow executes PKCE OAuth flow with a local callback server.
func RunBrowserFlow(ctx context.Context, cfg OAuthConfig, notifyURL func(url string)) (*Credentials, error) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}
	state := generateState()

	u, err := url.Parse(cfg.AuthorizeURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", cfg.RedirectURI)
	q.Set("scope", cfg.Scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)

	for k, v := range cfg.ExtraAuthorizeParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	if notifyURL != nil {
		notifyURL(u.String())
	}

	port := cfg.Port
	if port <= 0 {
		port = 1455
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	defer listener.Close()

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/auth/callback" {
				http.NotFound(w, r)
				return
			}
			if r.URL.Query().Get("state") != state {
				http.Error(w, "State mismatch", http.StatusBadRequest)
				errChan <- errors.New("state mismatch during OAuth callback")
				return
			}
			code := r.URL.Query().Get("code")
			if code == "" {
				http.Error(w, "Missing code", http.StatusBadRequest)
				errChan <- errors.New("missing authorization code in OAuth callback")
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><h2>Authentication complete!</h2><p>You can close this window and return to kei.</p></body></html>`))
			codeChan <- code
		}),
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			errChan <- err
		}
	}()

	var code string
	select {
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return nil, ctx.Err()
	case err := <-errChan:
		_ = server.Shutdown(context.Background())
		return nil, err
	case code = <-codeChan:
		_ = server.Shutdown(context.Background())
	}

	return ExchangeOAuthCode(ctx, http.DefaultClient, cfg.TokenURL, cfg.ClientID, code, verifier, cfg.RedirectURI)
}

// DeviceAuthResponse holds response from requesting a device usercode.
type DeviceAuthResponse struct {
	DeviceAuthID    string `json:"device_auth_id"`
	UserCode        string `json:"user_code"`
	IntervalSeconds int    `json:"interval"`
}

// RunDeviceCodeFlow executes OAuth device code polling flow.
func RunDeviceCodeFlow(ctx context.Context, cfg OAuthConfig, notify func(userCode, verificationURI string)) (*Credentials, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// 1. Request user code
	body, _ := json.Marshal(map[string]string{"client_id": cfg.ClientID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.DeviceUserCodeURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device auth request failed (%s): %s", resp.Status, string(b))
	}

	var devInfo DeviceAuthResponse
	if err := json.UnmarshalRead(resp.Body, &devInfo); err != nil {
		return nil, err
	}
	if devInfo.IntervalSeconds <= 0 {
		devInfo.IntervalSeconds = 5
	}

	if notify != nil {
		notify(devInfo.UserCode, cfg.DeviceAuthURL)
	}

	// 2. Poll token endpoint
	interval := time.Duration(devInfo.IntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pollBody, _ := json.Marshal(map[string]string{
		"device_auth_id": devInfo.DeviceAuthID,
		"user_code":      devInfo.UserCode,
	})

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			pReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.DeviceTokenURL, bytes.NewReader(pollBody))
			if err != nil {
				return nil, err
			}
			pReq.Header.Set("Content-Type", "application/json")

			pResp, err := client.Do(pReq)
			if err != nil {
				continue
			}

			pBytes, _ := io.ReadAll(pResp.Body)
			pResp.Body.Close()

			if pResp.StatusCode == http.StatusOK {
				var tokenSuccess struct {
					AuthorizationCode string `json:"authorization_code"`
					CodeVerifier      string `json:"code_verifier"`
				}
				if err := json.Unmarshal(pBytes, &tokenSuccess); err == nil && tokenSuccess.AuthorizationCode != "" {
					return ExchangeOAuthCode(ctx, client, cfg.TokenURL, cfg.ClientID, tokenSuccess.AuthorizationCode, tokenSuccess.CodeVerifier, cfg.DeviceRedirectURI)
				}
			}

			var errPayload struct {
				Error any `json:"error"`
			}
			_ = json.Unmarshal(pBytes, &errPayload)
			errStr := fmt.Sprint(errPayload.Error)
			if strings.Contains(errStr, "slow_down") {
				ticker.Reset(interval + 3*time.Second)
			}
		}
	}
}

// ExtractJWTClaims decodes unverified payload claims from a JWT token.
func ExtractJWTClaims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT format")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, err
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}
