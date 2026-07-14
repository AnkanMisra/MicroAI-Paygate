// gateway/routes/models.go
package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ─── Ollama API types ─────────────────────────────────────────────────────────

type OllamaModel struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
}

type OllamaTagsResponse struct {
	Models []OllamaModel `json:"models"`
}

// ─── In-process model override (session-scoped, non-persistent) ───────────────

var (
	activeModelOverride string
	modelMu             sync.RWMutex
)

// GetActiveOllamaModel returns the in-memory override if set, else falls back
// to the OLLAMA_MODEL env var. Call this anywhere you need the current model.
func GetActiveOllamaModel() string {
	modelMu.RLock()
	defer modelMu.RUnlock()
	if activeModelOverride != "" {
		return activeModelOverride
	}
	return os.Getenv("OLLAMA_MODEL")
}

// ─── GET /api/models ──────────────────────────────────────────────────────────

// GetAvailableModels lists AI models available for the configured provider.
//
//   - ollama  → queries the local Ollama daemon at OLLAMA_URL/api/tags
//   - openrouter → returns the currently configured OPENROUTER_MODEL only
//     (full catalogue would require an OpenRouter API call; out of scope here)
func GetAvailableModels(c *gin.Context) {
	provider := os.Getenv("AI_PROVIDER")

	switch provider {
	case "ollama":
		handleOllamaModels(c)
	case "openrouter":
		handleOpenRouterModels(c)
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    fmt.Sprintf("Unknown AI_PROVIDER '%s'. Valid values: ollama, openrouter", provider),
			"provider": provider,
			"models":   []gin.H{},
		})
	}
}

func handleOllamaModels(c *gin.Context) {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	// Use a short timeout — Ollama is local; if it doesn't respond in 3s it's down
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/tags", ollamaURL))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":    "Cannot connect to Ollama. Is Ollama running? (expected at: " + ollamaURL + ")",
			"provider": "ollama",
			"hint":     "Run `ollama serve` or check OLLAMA_URL in your .env",
		})
		return
	}
	defer resp.Body.Close()

	var ollamaResp OllamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to parse Ollama /api/tags response: " + err.Error(),
		})
		return
	}

	currentModel := GetActiveOllamaModel()

	models := make([]gin.H, 0, len(ollamaResp.Models))
	for _, m := range ollamaResp.Models {
		models = append(models, gin.H{
			"id":       m.Name,
			"name":     m.Name,
			"provider": "ollama",
			// Convert bytes → GB, keep one decimal place
			"sizeGB":   fmt.Sprintf("%.1fGB", float64(m.Size)/1e9),
			"isActive": m.Name == currentModel,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"provider":     "ollama",
		"currentModel": currentModel,
		"models":       models,
	})
}

func handleOpenRouterModels(c *gin.Context) {
	currentModel := os.Getenv("OPENROUTER_MODEL")
	c.JSON(http.StatusOK, gin.H{
		"provider":     "openrouter",
		"currentModel": currentModel,
		"models": []gin.H{
			{
				"id":       currentModel,
				"name":     currentModel,
				"provider": "openrouter",
				"isActive": true,
			},
		},
		"note": "OpenRouter supports 300+ models. Update OPENROUTER_MODEL in .env to change the model.",
	})
}

// ─── POST /api/models/switch ──────────────────────────────────────────────────

type SwitchModelRequest struct {
	ModelID string `json:"modelId" binding:"required"`
}

// SwitchModel updates the active model for the current process lifetime.
// This does NOT persist across gateway restarts — users must update .env for
// permanent changes. Returns a confirmation with a clear session-only warning.
func SwitchModel(c *gin.Context) {
	var req SwitchModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Request body must include a non-empty 'modelId' field",
		})
		return
	}

	// Validate: for ollama, ensure the model is actually installed
	if os.Getenv("AI_PROVIDER") == "ollama" {
		if err := validateOllamaModelExists(req.ModelID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Model '%s' not found in Ollama. Run `ollama pull %s` first.", req.ModelID, req.ModelID),
			})
			return
		}
	}

	modelMu.Lock()
	activeModelOverride = req.ModelID
	modelMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"activeModel": req.ModelID,
		"note":        "Model switched for this session only. To persist, update OLLAMA_MODEL in your .env and restart.",
	})
}

// validateOllamaModelExists checks the model actually exists before switching.
func validateOllamaModelExists(modelID string) error {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/tags", ollamaURL))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var ollamaResp OllamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return err
	}
	for _, m := range ollamaResp.Models {
		if m.Name == modelID {
			return nil
		}
	}
	return fmt.Errorf("model not found")
}