// gateway/routes/models_test.go
package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/api/models", GetAvailableModels)
	r.POST("/api/models/switch", SwitchModel)
	return r
}

// ─── GET /api/models ──────────────────────────────────────────────────────────

func TestGetModels_OllamaNotRunning(t *testing.T) {
	os.Setenv("AI_PROVIDER", "ollama")
	os.Setenv("OLLAMA_URL", "http://localhost:19999") // port nobody listens on

	r := setupRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d — body: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["error"]; !ok {
		t.Error("expected 'error' field in response body")
	}
}

func TestGetModels_OpenRouter_ReturnsCurrent(t *testing.T) {
	os.Setenv("AI_PROVIDER", "openrouter")
	os.Setenv("OPENROUTER_MODEL", "z-ai/glm-4.5-air:free")

	r := setupRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)

	if body["provider"] != "openrouter" {
		t.Errorf("expected provider=openrouter, got %v", body["provider"])
	}
	if body["currentModel"] != "z-ai/glm-4.5-air:free" {
		t.Errorf("expected currentModel to match env, got %v", body["currentModel"])
	}
}

func TestGetModels_UnknownProvider(t *testing.T) {
	os.Setenv("AI_PROVIDER", "definitely-not-real")

	r := setupRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── POST /api/models/switch ──────────────────────────────────────────────────

func TestSwitchModel_MissingModelId(t *testing.T) {
	os.Setenv("AI_PROVIDER", "openrouter") // skip ollama validation

	r := setupRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/models/switch",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing modelId, got %d", w.Code)
	}
}

func TestSwitchModel_OpenRouter_Success(t *testing.T) {
	os.Setenv("AI_PROVIDER", "openrouter")

	r := setupRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/models/switch",
		strings.NewReader(`{"modelId":"anthropic/claude-3-haiku"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", w.Code, w.Body.String())
	}

	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["activeModel"] != "anthropic/claude-3-haiku" {
		t.Errorf("expected activeModel in response, got %v", body["activeModel"])
	}
	if body["success"] != true {
		t.Error("expected success=true")
	}
}

func TestGetActiveOllamaModel_FallsBackToEnv(t *testing.T) {
	// Reset override
	modelMu.Lock()
	activeModelOverride = ""
	modelMu.Unlock()

	os.Setenv("OLLAMA_MODEL", "llama3:8b")
	result := GetActiveOllamaModel()
	if result != "llama3:8b" {
		t.Errorf("expected llama3:8b, got %s", result)
	}
}

func TestGetActiveOllamaModel_OverrideWins(t *testing.T) {
	os.Setenv("OLLAMA_MODEL", "llama3:8b")

	modelMu.Lock()
	activeModelOverride = "mistral:7b"
	modelMu.Unlock()

	result := GetActiveOllamaModel()
	if result != "mistral:7b" {
		t.Errorf("expected mistral:7b override, got %s", result)
	}

	// Cleanup
	modelMu.Lock()
	activeModelOverride = ""
	modelMu.Unlock()
}