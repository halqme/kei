package provider

import (
	"slices"
	"strings"
	"testing"
)

func TestListReturnsCanonicalProviderTypes(t *testing.T) {
	want := []string{"anthropic", "azure", "codex", "gemini", "ollama", "openai"}
	if got := List(); !slices.Equal(got, want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
}

func TestNewRequiresProviderType(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected an empty provider type to fail")
	}
	if !strings.Contains(err.Error(), "provider type is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
