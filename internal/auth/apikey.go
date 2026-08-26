package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

type APIKeyAuthenticator struct {
	providerName string
	envVarName   string
	desc         string
}

func NewAPIKeyAuthenticator(name, envVar, desc string) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{
		providerName: name,
		envVarName:   envVar,
		desc:         desc,
	}
}

func (a *APIKeyAuthenticator) Name() string {
	return a.providerName
}

func (a *APIKeyAuthenticator) Description() string {
	return a.desc
}

func (a *APIKeyAuthenticator) DefaultSavePath() (string, error) {
	return DefaultKeiAuthSavePath(), nil
}

func (a *APIKeyAuthenticator) Login(ctx context.Context, opts LoginOptions) (*Credentials, error) {
	if opts.Notify != nil {
		opts.Notify(fmt.Sprintf("API key for %s (or export %s)", a.providerName, a.envVarName))
	}
	fmt.Printf("Enter API key for %s: ", a.providerName)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(line)
	if key == "" {
		return nil, errors.New("API key cannot be empty")
	}

	creds := &Credentials{
		APIKey: key,
	}
	if err := GetDefaultStore().SetCredential(a.providerName, creds); err != nil {
		return nil, err
	}

	return creds, nil
}

func init() {
	Register("openai", NewAPIKeyAuthenticator("openai", "OPENAI_API_KEY", "OpenAI platform API key"))
	Register("anthropic", NewAPIKeyAuthenticator("anthropic", "ANTHROPIC_API_KEY", "Anthropic Claude API key"))
	Register("gemini", NewAPIKeyAuthenticator("gemini", "GEMINI_API_KEY", "Google Gemini API key"))
}
