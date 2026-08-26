package provider

import (
	"testing"
)

func TestFactoryNew(t *testing.T) {
	// 1. OpenAI Compatible (default)
	pOpenAI, err := New(Config{
		Type:      "openai",
		BaseURL:   "https://api.openai.com/v1",
		APIKeyEnv: "",
		APIKey:    "test-key",
		Model:     "gpt-5.6",
	})
	if err != nil {
		t.Fatalf("New(openai) failed: %v", err)
	}
	if _, ok := pOpenAI.(*OpenAICompatible); !ok {
		t.Errorf("expected *OpenAICompatible, got %T", pOpenAI)
	}

	// 2. Codex
	pCodex, err := New(Config{
		Type:    "codex",
		BaseURL: "https://chatgpt.com/backend-api/codex",
		Model:   "gpt-5.5",
	})
	if err != nil {
		t.Fatalf("New(codex) failed: %v", err)
	}
	if _, ok := pCodex.(*Codex); !ok {
		t.Errorf("expected *Codex, got %T", pCodex)
	}

	// 3. openai-codex alias
	pCodexAlias, err := New(Config{
		Type: "openai-codex",
	})
	if err != nil {
		t.Fatalf("New(openai-codex) failed: %v", err)
	}
	if _, ok := pCodexAlias.(*Codex); !ok {
		t.Errorf("expected *Codex, got %T", pCodexAlias)
	}

	// 4. Anthropic
	pAnthropic, err := New(Config{
		Type:   "anthropic",
		APIKey: "sk-ant-test",
		Model:  "claude-3-7-sonnet-20250219",
	})
	if err != nil {
		t.Fatalf("New(anthropic) failed: %v", err)
	}
	if _, ok := pAnthropic.(*Anthropic); !ok {
		t.Errorf("expected *Anthropic, got %T", pAnthropic)
	}

	// 5. Gemini
	pGemini, err := New(Config{
		Type:   "gemini",
		APIKey: "test-gemini-key",
		Model:  "gemini-2.5-flash",
	})
	if err != nil {
		t.Fatalf("New(gemini) failed: %v", err)
	}
	if _, ok := pGemini.(*Gemini); !ok {
		t.Errorf("expected *Gemini, got %T", pGemini)
	}

	// 6. Ollama
	pOllama, err := New(Config{
		Type: "ollama",
	})
	if err != nil {
		t.Fatalf("New(ollama) failed: %v", err)
	}
	if _, ok := pOllama.(*OpenAICompatible); !ok {
		t.Errorf("expected *OpenAICompatible, got %T", pOllama)
	}

	// 7. Invalid provider type
	_, err = New(Config{
		Type: "unsupported-provider",
	})
	if err == nil {
		t.Errorf("expected error for unsupported provider type, got nil")
	}
}
