package ai

import "context"

type MockProvider struct {
	Name string
}

func NewMockProvider() *MockProvider {
	return &MockProvider{Name: "Mock"}
}

func (m *MockProvider) Generate(ctx context.Context, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "This is a deterministic mock summary of the input text for local/demo testing.", nil
}
