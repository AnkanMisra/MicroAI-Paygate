package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

// OllamaProvider implements the Provider interface for Ollama API
type OllamaProvider struct {
	url   string
	model string
}

// NewOllamaProvider creates a new Ollama provider instance
func NewOllamaProvider() *OllamaProvider {
	url := os.Getenv("OLLAMA_URL")
	if url == "" {
		url = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "llama2"
	}

	return &OllamaProvider{
		url:   url,
		model: model,
	}
}

// Generate sends a prompt to Ollama and returns the response
func (p *OllamaProvider) Generate(ctx context.Context, text string) (string, error) {
	prompt := fmt.Sprintf("Summarize this text in 2 sentences: %s", text)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", p.url+"/api/generate", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create Ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return "", context.DeadlineExceeded
		}
		return "", fmt.Errorf("failed to connect to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	response, ok := result["response"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response from Ollama: missing response field")
	}

	return response, nil
}
