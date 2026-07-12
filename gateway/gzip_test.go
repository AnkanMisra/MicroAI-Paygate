package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	gzipMiddleware "github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func TestGzipCompression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		acceptEncoding   string
		expectCompressed bool
	}{
		{
			name:             "with gzip accept-encoding",
			acceptEncoding:   "gzip",
			expectCompressed: true,
		},
		{
			name:             "without accept-encoding",
			acceptEncoding:   "",
			expectCompressed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(gzipMiddleware.Gzip(gzipMiddleware.DefaultCompression))

			r.GET("/test", func(c *gin.Context) {
				// Use larger response body to ensure compression is worthwhile
				// and avoid middleware skipping compression for tiny responses
				c.JSON(200, map[string]string{
					"message": "This is a test response for gzip compression middleware validation",
					"data":    "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.",
					"extra":   "Additional padding to ensure response is large enough for compression to be beneficial and avoid any middleware size thresholds.",
				})
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			contentEncoding := w.Header().Get("Content-Encoding")
			if tt.expectCompressed {
				if contentEncoding != "gzip" {
					t.Errorf("expected Content-Encoding: gzip, got: %s", contentEncoding)
				}

				reader, err := gzip.NewReader(w.Body)
				if err != nil {
					t.Fatalf("failed to create gzip reader: %v", err)
				}
				defer reader.Close()

				decompressed, err := io.ReadAll(reader)
				if err != nil {
					t.Fatalf("failed to decompress response: %v", err)
				}

				if len(decompressed) == 0 {
					t.Error("decompressed response is empty")
				}
			} else {
				if contentEncoding == "gzip" {
					t.Error("expected no compression, but got Content-Encoding: gzip")
				}
			}
		})
	}
}

func TestGzipExcludedPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(gzipMiddleware.Gzip(gzipMiddleware.DefaultCompression,
		gzipMiddleware.WithExcludedPaths([]string{"/metrics"})))

	r.GET("/metrics", func(c *gin.Context) {
		c.String(200, "metrics data")
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	contentEncoding := w.Header().Get("Content-Encoding")
	if contentEncoding == "gzip" {
		t.Error("metrics endpoint should not be compressed")
	}
}

func TestGzipTransparencyWithHashing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(gzipMiddleware.Gzip(gzipMiddleware.DefaultCompression))

	// Mock handler that simulates receipt generation behavior:
	// 1. Generate response body
	// 2. Hash the uncompressed body (like generateAndSendReceipt does)
	// 3. Send hash in header (simulating X-402-Receipt behavior)
	// 4. Return JSON response (which gets compressed by middleware)
	r.GET("/api/test", func(c *gin.Context) {
		responseBody := map[string]string{
			"result": "This is a test response to verify that GZIP compression is transparent to cryptographic signing and receipt verification.",
		}

		// Hash the uncompressed body (server-side, before compression)
		// This simulates what generateAndSendReceipt does
		uncompressedBytes, _ := json.Marshal(responseBody)
		serverHash := sha256.Sum256(uncompressedBytes)
		serverHashHex := hex.EncodeToString(serverHash[:])

		// Send hash in header (simulating receipt behavior)
		c.Header("X-Response-Hash", serverHashHex)

		// Send JSON response (middleware will compress this)
		c.JSON(200, responseBody)
	})

	// Client request with gzip support
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// Verify response is compressed
	contentEncoding := w.Header().Get("Content-Encoding")
	if contentEncoding != "gzip" {
		t.Fatalf("expected Content-Encoding: gzip, got: %s", contentEncoding)
	}

	// Get server hash from header
	serverHash := w.Header().Get("X-Response-Hash")
	if serverHash == "" {
		t.Fatal("server hash header is missing")
	}

	// Client decompresses the response
	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress response: %v", err)
	}

	// Client computes hash of decompressed body
	clientHash := sha256.Sum256(decompressedBody)
	clientHashHex := hex.EncodeToString(clientHash[:])

	// Verify that client hash matches server hash
	// This proves compression is transparent to receipt verification
	if clientHashHex != serverHash {
		t.Errorf("hash mismatch: client computed %s but server sent %s", clientHashHex, serverHash)
		t.Errorf("This means GZIP compression breaks receipt verification!")
	}

	// Verify the decompressed body is valid JSON
	var result map[string]string
	if err := json.Unmarshal(decompressedBody, &result); err != nil {
		t.Fatalf("failed to parse decompressed JSON: %v", err)
	}

	if result["result"] == "" {
		t.Error("decompressed response is missing expected data")
	}
}
