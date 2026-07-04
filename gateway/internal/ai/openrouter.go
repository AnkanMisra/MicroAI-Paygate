package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return "", context.DeadlineExceeded
		}
		return "", err
	}
	defer resp.Body.Close()

	// Check status code before decoding
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openrouter returned status %d: %s", resp.StatusCode, string(body))
	}

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

type openRouterStream struct {
	resp   *http.Response
	reader *bufio.Reader
}

func (s *openRouterStream) Recv() (string, error) {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			return "", err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte("data: ")) {
			data := bytes.TrimPrefix(line, []byte("data: "))
			if string(data) == "[DONE]" {
				return "", io.EOF
			}
			var chunk map[string]interface{}
			if err := json.Unmarshal(data, &chunk); err != nil {
				continue // skip invalid json
			}
			choices, ok := chunk["choices"].([]interface{})
			if !ok || len(choices) == 0 {
				continue
			}
			choice, ok := choices[0].(map[string]interface{})
			if !ok {
				continue
			}
			delta, ok := choice["delta"].(map[string]interface{})
			if !ok {
				continue
			}
			if content, ok := delta["content"].(string); ok {
				return content, nil
			}
		}
	}
}

func (s *openRouterStream) Close() error {
	return s.resp.Body.Close()
}

func (p *OpenRouterProvider) GenerateStream(ctx context.Context, text string) (Stream, error) {
	prompt := fmt.Sprintf("Summarize this text in 2 sentences: %s", text)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": true,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", p.url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenRouter request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return nil, context.DeadlineExceeded
		}
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openrouter returned status %d: %s", resp.StatusCode, string(body))
	}

	return &openRouterStream{
		resp:   resp,
		reader: bufio.NewReader(resp.Body),
	}, nil
}
