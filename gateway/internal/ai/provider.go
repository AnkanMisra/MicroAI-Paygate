package ai

import (
	"context"
	"fmt"
	"os"
	"strings"
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
		if err := IsMockAllowed(os.Getenv("NODE_ENV"), os.Getenv("APP_ENV"), os.Getenv("ALLOW_MOCK_PROVIDER")); err != nil {
			return nil, err
		}
		return NewMockProvider(), nil
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", providerType)
	}
}

// IsMockAllowed evaluates NODE_ENV, APP_ENV, and ALLOW_MOCK_PROVIDER to see if the mock provider can be run.
// It normalizes checks by lowercasing and trimming spaces, evaluating production prefixes,
// and treating truthy values for ALLOW_MOCK_PROVIDER.
func IsMockAllowed(nodeEnv, appEnv, allowMock string) error {
	nodeEnv = strings.ToLower(strings.TrimSpace(nodeEnv))
	appEnv = strings.ToLower(strings.TrimSpace(appEnv))
	allowMock = strings.ToLower(strings.TrimSpace(allowMock))

	isProd := strings.HasPrefix(nodeEnv, "prod") || strings.HasPrefix(appEnv, "prod")
	if isProd {
		return fmt.Errorf("mock AI provider is disabled in production environments")
	}

	isAllowedEnv := nodeEnv == "development" || nodeEnv == "local" || nodeEnv == "demo" || nodeEnv == "test" || nodeEnv == "dev" ||
		appEnv == "development" || appEnv == "local" || appEnv == "demo" || appEnv == "test" || appEnv == "dev"

	isExplicitlyAllowed := allowMock == "true" || allowMock == "1" || allowMock == "yes"

	if !isAllowedEnv && !isExplicitlyAllowed {
		return fmt.Errorf("mock AI provider is disabled. Enable it by setting ALLOW_MOCK_PROVIDER=true, or set NODE_ENV/APP_ENV to development/local/demo/test")
	}

	return nil
}
