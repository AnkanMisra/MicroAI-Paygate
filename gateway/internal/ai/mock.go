package ai

import (
	"context"
	"strings"
)

// MockProvider returns deterministic summaries for local demos and CI.
type MockProvider struct{}

// NewMockProvider creates a deterministic provider with no external dependencies.
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// Generate returns a stable summary without calling a model provider.
func (p *MockProvider) Generate(ctx context.Context, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	normalized := strings.Join(strings.Fields(text), " ")
	if normalized == "" {
		return "Mock summary: no input text provided.", nil
	}
	if len(normalized) > 96 {
		normalized = strings.TrimSpace(normalized[:96]) + "..."
	}
	return "Mock summary: " + normalized, nil
}
