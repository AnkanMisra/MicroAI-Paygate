package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenRouterErrorBodyLimit verifies that the OpenRouter provider caps error
// response body reads at maxErrorResponseBytes to prevent OOM from a malicious
// upstream.
func TestOpenRouterErrorBodyLimit(t *testing.T) {
	// Serve a body larger than the limit (8KB > 4KB limit)
	oversizedBody := strings.Repeat("X", 8192)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(oversizedBody))
	}))
	defer srv.Close()

	p := &OpenRouterProvider{
		apiKey: "test-key",
		model:  "test-model",
		url:    srv.URL,
	}

	_, err := p.Generate(context.Background(), "test prompt")
	if err == nil {
		t.Fatal("expected error from 500 response, got nil")
	}

	// The error message should NOT contain the full 8KB body
	errMsg := err.Error()
	if len(errMsg) > int(maxErrorResponseBytes)+200 {
		t.Errorf("error message too long (%d bytes), expected capped at ~%d", len(errMsg), maxErrorResponseBytes)
	}
}

// TestOllamaErrorBodyLimit verifies that the Ollama provider caps error
// response body reads at maxOllamaErrorResponseBytes.
func TestOllamaErrorBodyLimit(t *testing.T) {
	oversizedBody := strings.Repeat("Y", 8192)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(oversizedBody))
	}))
	defer srv.Close()

	p := &OllamaProvider{
		url:   srv.URL,
		model: "test-model",
	}

	_, err := p.Generate(context.Background(), "test prompt")
	if err == nil {
		t.Fatal("expected error from 500 response, got nil")
	}

	errMsg := err.Error()
	if len(errMsg) > int(maxOllamaErrorResponseBytes)+200 {
		t.Errorf("error message too long (%d bytes), expected capped at ~%d", len(errMsg), maxOllamaErrorResponseBytes)
	}
}

// TestOpenRouterContextCanceled verifies that context.Canceled is returned
// (not wrapped in a connection error) when the context is canceled.
func TestOpenRouterContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond — let the context cancel
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := &OpenRouterProvider{
		apiKey: "test-key",
		model:  "test-model",
		url:    srv.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := p.Generate(ctx, "test prompt")
	if err == nil {
		t.Fatal("expected error from canceled context, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// TestOllamaContextCanceled verifies the same for the Ollama provider.
func TestOllamaContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := &OllamaProvider{
		url:   srv.URL,
		model: "test-model",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Generate(ctx, "test prompt")
	if err == nil {
		t.Fatal("expected error from canceled context, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}
