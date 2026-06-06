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

// StreamChunk is one incremental provider response fragment.
type StreamChunk struct {
	Content string
	Done    bool
}

// StreamingProvider is implemented by providers that can return incremental
// output. Callers should gracefully fall back or fail when a provider does not
// implement this optional interface.
type StreamingProvider interface {
	StreamGenerate(ctx context.Context, prompt string) (<-chan StreamChunk, <-chan error)
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
