package ai

import (
	"context"
	"fmt"
	"os"
)

// Provider defines the interface for AI service providers
type Provider interface {
	// Generate takes a context and prompt text, returns the AI-generated response
	Generate(ctx context.Context, prompt string) (string, error)
}

// NewProvider creates an AI provider based on the AI_PROVIDER environment variable
// Supported providers: "openrouter" (default), "ollama"
func NewProvider() (Provider, error) {
	providerType := os.Getenv("AI_PROVIDER")
	if providerType == "" {
		providerType = "openrouter"
	}

	switch providerType {
	case "openrouter":
		return NewOpenRouterProvider(), nil
	case "ollama":
		return NewOllamaProvider(), nil
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", providerType)
	}
}
