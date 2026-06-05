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
	case "mock":
		nodeEnv := os.Getenv("NODE_ENV")
		appEnv := os.Getenv("APP_ENV")
		allowMock := os.Getenv("ALLOW_MOCK_PROVIDER") == "true"

		if nodeEnv == "production" || appEnv == "production" {
			return nil, fmt.Errorf("mock AI provider is disabled in production environments")
		}

		isAllowedEnv := nodeEnv == "development" || nodeEnv == "local" || nodeEnv == "demo" || nodeEnv == "test" || nodeEnv == "dev" ||
			appEnv == "development" || appEnv == "local" || appEnv == "demo" || appEnv == "test" || appEnv == "dev"

		if !isAllowedEnv && !allowMock {
			return nil, fmt.Errorf("mock AI provider is disabled. Enable it by setting ALLOW_MOCK_PROVIDER=true, or set NODE_ENV/APP_ENV to development/local/demo/test")
		}
		return NewMockProvider(), nil
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", providerType)
	}
}
