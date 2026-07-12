package ai

import (
	"testing"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		wantType     string
		wantErr      bool
	}{
		{
			name:         "default to openrouter",
			providerType: "",
			wantType:     "*ai.OpenRouterProvider",
			wantErr:      false,
		},
		{
			name:         "explicit openrouter",
			providerType: "openrouter",
			wantType:     "*ai.OpenRouterProvider",
			wantErr:      false,
		},
		{
			name:         "ollama provider",
			providerType: "ollama",
			wantType:     "*ai.OllamaProvider",
			wantErr:      false,
		},
		{
			name:         "unsupported provider",
			providerType: "invalid",
			wantType:     "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AI_PROVIDER", tt.providerType)

			provider, err := NewProvider()

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewProvider() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewProvider() unexpected error: %v", err)
				return
			}

			if provider == nil {
				t.Errorf("NewProvider() returned nil provider")
				return
			}

			// Check provider type
			switch tt.wantType {
			case "*ai.OpenRouterProvider":
				if _, ok := provider.(*OpenRouterProvider); !ok {
					t.Errorf("NewProvider() returned %T, want *OpenRouterProvider", provider)
				}
			case "*ai.OllamaProvider":
				if _, ok := provider.(*OllamaProvider); !ok {
					t.Errorf("NewProvider() returned %T, want *OllamaProvider", provider)
				}
			}
		})
	}
}

func TestNewOpenRouterProvider(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_MODEL", "test-model")
	t.Setenv("OPENROUTER_URL", "http://test.com")

	provider := NewOpenRouterProvider()

	if provider.apiKey != "test-key" {
		t.Errorf("expected apiKey 'test-key', got '%s'", provider.apiKey)
	}
	if provider.model != "test-model" {
		t.Errorf("expected model 'test-model', got '%s'", provider.model)
	}
	if provider.url != "http://test.com" {
		t.Errorf("expected url 'http://test.com', got '%s'", provider.url)
	}
}

func TestNewOpenRouterProvider_Defaults(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_MODEL", "")
	t.Setenv("OPENROUTER_URL", "")

	provider := NewOpenRouterProvider()

	if provider.model != "z-ai/glm-4.5-air:free" {
		t.Errorf("expected default model 'z-ai/glm-4.5-air:free', got '%s'", provider.model)
	}
	if provider.url != "https://openrouter.ai/api/v1/chat/completions" {
		t.Errorf("expected default url, got '%s'", provider.url)
	}
}

func TestNewOllamaProvider(t *testing.T) {
	t.Setenv("OLLAMA_URL", "http://localhost:11434")
	t.Setenv("OLLAMA_MODEL", "llama2")

	provider := NewOllamaProvider()

	if provider.url != "http://localhost:11434" {
		t.Errorf("expected url 'http://localhost:11434', got '%s'", provider.url)
	}
	if provider.model != "llama2" {
		t.Errorf("expected model 'llama2', got '%s'", provider.model)
	}
}

func TestNewOllamaProvider_Defaults(t *testing.T) {
	t.Setenv("OLLAMA_URL", "")
	t.Setenv("OLLAMA_MODEL", "")

	provider := NewOllamaProvider()

	if provider.url != "http://localhost:11434" {
		t.Errorf("expected default url 'http://localhost:11434', got '%s'", provider.url)
	}
	if provider.model != "llama2" {
		t.Errorf("expected default model 'llama2', got '%s'", provider.model)
	}
}
