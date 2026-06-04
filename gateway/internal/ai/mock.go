package ai

import "context"

// MockProvider implements the Provider interface for local/demo testing
type MockProvider struct{}

// NewMockProvider creates a new MockProvider instance
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// Generate returns a deterministic mock summary response
func (p *MockProvider) Generate(ctx context.Context, text string) (string, error) {
	return "This is a deterministic mock summary of the input text for local/demo testing.", nil
}
