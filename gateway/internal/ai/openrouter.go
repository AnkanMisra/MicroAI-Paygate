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
	"strings"
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

// StreamGenerate sends a streaming chat completion request to OpenRouter and
// emits text deltas as they arrive.
func (p *OpenRouterProvider) StreamGenerate(ctx context.Context, text string) (<-chan StreamChunk, <-chan error) {
	chunks := make(chan StreamChunk)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		sendErr := func(err error) bool {
			select {
			case errs <- err:
				return true
			case <-ctx.Done():
				return false
			}
		}

		sendChunk := func(chunk StreamChunk) bool {
			select {
			case chunks <- chunk:
				return true
			case <-ctx.Done():
				_ = sendErr(ctx.Err())
				return false
			}
		}

		prompt := fmt.Sprintf("Summarize this text in 2 sentences: %s", text)
		reqBody, _ := json.Marshal(map[string]interface{}{
			"model":  p.model,
			"stream": true,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		})

		req, err := http.NewRequestWithContext(ctx, "POST", p.url, bytes.NewBuffer(reqBody))
		if err != nil {
			_ = sendErr(fmt.Errorf("failed to create OpenRouter stream request: %w", err))
			return
		}
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
				_ = sendErr(context.DeadlineExceeded)
				return
			}
			_ = sendErr(err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			_ = sendErr(fmt.Errorf("openrouter returned status %d: %s", resp.StatusCode, string(body)))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}

			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				_ = sendChunk(StreamChunk{Done: true})
				return
			}

			var event struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				_ = sendErr(fmt.Errorf("failed to decode OpenRouter stream event: %w", err))
				return
			}
			if len(event.Choices) == 0 {
				continue
			}
			if content := event.Choices[0].Delta.Content; content != "" {
				if !sendChunk(StreamChunk{Content: content}) {
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendErr(fmt.Errorf("failed reading OpenRouter stream: %w", err))
			return
		}
		_ = sendErr(fmt.Errorf("openrouter stream ended before done event"))
	}()

	return chunks, errs
}
