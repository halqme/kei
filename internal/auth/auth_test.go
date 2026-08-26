package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"path/filepath"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}
	if len(verifier) == 0 || len(challenge) == 0 {
		t.Fatalf("empty verifier or challenge")
	}

	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])
	if challenge != expected {
		t.Errorf("expected challenge %s, got %s", expected, challenge)
	}
}

func TestExtractJWTClaims(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claims := map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acc_codex_999",
		},
	}
	claimsBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsBytes)
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))

	jwt := header + "." + payload + "." + sig

	parsed, err := ExtractJWTClaims(jwt)
	if err != nil {
		t.Fatalf("ExtractJWTClaims failed: %v", err)
	}

	authMap, ok := parsed["https://api.openai.com/auth"].(map[string]any)
	if !ok {
		t.Fatalf("expected auth claim map")
	}
	if authMap["chatgpt_account_id"] != "acc_codex_999" {
		t.Errorf("expected acc_codex_999, got %v", authMap["chatgpt_account_id"])
	}
}

func TestAuthStore(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "auth.json")
	t.Setenv("KEI_AUTH_FILE", authFile)

	store := NewStore(nil)
	if err := store.SetCredential("anthropic", &Credentials{APIKey: "sk-ant-test"}); err != nil {
		t.Fatalf("SetCredential failed: %v", err)
	}
	if err := store.SetCredential("codex", &Credentials{AccessToken: "tok-123", AccountID: "acc-123"}); err != nil {
		t.Fatalf("SetCredential failed: %v", err)
	}

	loadedStore := NewStore(nil)
	if err := loadedStore.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	antCreds, err := loadedStore.GetCredential(nil, "anthropic")
	if err != nil || antCreds == nil || antCreds.APIKey != "sk-ant-test" {
		t.Errorf("unexpected anthropic creds: %+v, err: %v", antCreds, err)
	}

	codexCreds, err := loadedStore.GetCredential(nil, "codex")
	if err != nil || codexCreds == nil || codexCreds.AccessToken != "tok-123" || codexCreds.AccountID != "acc-123" {
		t.Errorf("unexpected codex creds: %+v, err: %v", codexCreds, err)
	}
}

func TestParseCodexAuthJSON(t *testing.T) {
	nestedData := []byte(`{
		"auth_mode": "chatgpt",
		"tokens": {
			"access_token": "acc-token-123",
			"refresh_token": "ref-token-456",
			"id_token": "id-token-789",
			"account_id": "acc_org_1"
		}
	}`)

	creds, err := parseCodexAuthJSON(nestedData)
	if err != nil {
		t.Fatalf("parseCodexAuthJSON failed: %v", err)
	}
	if creds.AccessToken != "acc-token-123" || creds.RefreshToken != "ref-token-456" || creds.AccountID != "acc_org_1" {
		t.Errorf("unexpected parsed creds: %+v", creds)
	}
}

func TestAuthRegistry(t *testing.T) {
	list := List()
	if len(list) < 4 {
		t.Errorf("expected at least 4 authenticators, got %d", len(list))
	}

	if _, ok := Get("codex"); !ok {
		t.Errorf("expected 'codex' in auth registry")
	}
	if _, ok := Get("anthropic"); !ok {
		t.Errorf("expected 'anthropic' in auth registry")
	}
	if _, ok := Get("openai"); !ok {
		t.Errorf("expected 'openai' in auth registry")
	}
	if _, ok := Get("gemini"); !ok {
		t.Errorf("expected 'gemini' in auth registry")
	}
}
