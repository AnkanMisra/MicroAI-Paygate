package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
)

// OpenRouterProvider implements the Provider interface for OpenRouter API
type OpenRouterProvider struct {
	apiKey string
	model  string
	url    string
}

// NewOpenRouterProvider creates a new OpenRouter provider instance
func NewOpenRouterProvider() *OpenRouterProvider {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "z-ai/glm-4.5-air:free"
	}

	url := os.Getenv("OPENROUTER_URL")
	if url == "" {
		url = "https://openrouter.ai/api/v1/chat/completions"
	}

	return &OpenRouterProvider{
		apiKey: apiKey,
		model:  model,
		url:    url,
	}
}

// Generate sends a prompt to OpenRouter and returns the response
func (p *OpenRouterProvider) Generate(ctx context.Context, text string) (string, error) {
	prompt := fmt.Sprintf("Summarize this text in 2 sentences: %s", text)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", p.url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create OpenRouter request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Pass Correlation ID if available in context
	// Note: correlationIDKey is defined in middleware.go as contextKey("correlation_id")
	type contextKey string
	const correlationIDKey contextKey = "correlation_id"
	if cid, ok := ctx.Value(correlationIDKey).(string); ok {
		req.Header.Set("X-Correlation-ID", cid)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return "", context.DeadlineExceeded
		}
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode AI response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		log.Printf("OpenRouter response: %+v", result)
		return "", fmt.Errorf("invalid response from AI provider: no choices")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response from AI provider: malformed choice")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response from AI provider: malformed message")
	}

	content, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response from AI provider: missing content")
	}

	return content, nil
}
