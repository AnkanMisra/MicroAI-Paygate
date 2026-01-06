// Package main implements the gateway HTTP server used by MicroAI-Paygate.
// It provides request handlers, middleware, and configuration helpers
// for timeouts and rate limiting.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gateway/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

/* -------------------- Types -------------------- */

type PaymentContext struct {
	Recipient string `json:"recipient"`
	Token     string `json:"token"`
	Amount    string `json:"amount"`
	Nonce     string `json:"nonce"`
	ChainID   int    `json:"chainId"`
}

type VerifyRequest struct {
	Context   PaymentContext `json:"context"`
	Signature string         `json:"signature"`
}

type VerifyResponse struct {
	IsValid          bool   `json:"is_valid"`
	RecoveredAddress string `json:"recovered_address"`
	Error            string `json:"error"`
}

type SummarizeRequest struct {
	Text string `json:"text"`
}

/* -------------------- Main -------------------- */

func main() {
	_ = godotenv.Load("../.env")
func validateConfig() error {
	required := []string{
		"OPENROUTER_API_KEY",
	}
	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missing)
	}
	return nil
}
func main() {
	// Try loading .env from current directory first, then fallback to parent
	err := godotenv.Load(".env")
	if err != nil {
		// fallback to parent
		err = godotenv.Load("../.env")
		if err != nil {
			log.Println("Warning: Error loading .env file")
		}
	}
	if err := validateConfig(); err != nil {
		fmt.Println("[Error] Missing required environment variables:")
		fmt.Println("  -", err.Error())
		fmt.Println()
		fmt.Println("Copy .env.example to .env and fill in the required values.")
		fmt.Println("See README.md for more configuration details.")
		os.Exit(1)
	}
	fmt.Println("[OK] Configuration validated")
	if port := os.Getenv("PORT"); port != "" {
		fmt.Printf("    - Port: %s\n", port)
	}
	if model := os.Getenv("MODEL"); model != "" {
		fmt.Printf("    - Model: %s\n", model)
	}
	if verifier := os.Getenv("VERIFIER_URL"); verifier != "" {
		fmt.Printf("    - Verifier: %s\n", verifier)
	}
	if chainID := os.Getenv("CHAIN_ID"); chainID != "" {
		fmt.Printf("    - Chain ID: %s\n", chainID)
	}
	if os.Getenv("PORT") == "" {
		fmt.Println("[WARN] PORT not set, using default: 3000")
	}
	if os.Getenv("MODEL") == "" {
		fmt.Println("[WARN] MODEL not set, using default model")
	}
	if os.Getenv("VERIFIER_URL") == "" {
		fmt.Println("[WARN] VERIFIER_URL not set, using default verifier")
	}
	if os.Getenv("CHAIN_ID") == "" {
		fmt.Println("[WARN] CHAIN_ID not set, using default: 8453(base)")
	}

	// Init structured logging
	middleware.InitLogger()

	r := gin.New()
	r.Use(
		gin.Recovery(),
		middleware.RequestLogger(),
	)

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3001"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "X-402-Signature", "X-402-Nonce"},
		ExposeHeaders:    []string{"Content-Length", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After"},
		AllowCredentials: true,
	}))

	// Initialize rate limiters if enabled
	if getRateLimitEnabled() {
		limiters := initRateLimiters()
		r.Use(RateLimitMiddleware(limiters))
		log.Println("Rate limiting enabled")
	}

	// Global request timeout middleware (default: 60s).
	// Note: route-specific timeouts (e.g. for AI endpoints) may shorten this
	// deadline; the middleware implementation always uses the earliest
	// deadline when nested timeouts are present to avoid surprising behavior.
	r.Use(RequestTimeoutMiddleware(getRequestTimeout()))

	// Health check with shorter timeout (2s)
	r.GET("/healthz", RequestTimeoutMiddleware(getHealthCheckTimeout()), handleHealth)

	// AI endpoints with AI-specific timeout (30s)
	aiGroup := r.Group("/api/ai")
	aiGroup.Use(RequestTimeoutMiddleware(getAITimeout()))
	aiGroup.POST("/summarize", handleSummarize)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Go Gateway running on port %s", port)
	r.Run(":" + port)
}

/* -------------------- Handlers -------------------- */

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "gateway",
	})
}

// handleSummarize handles POST /api/ai/summarize requests. It validates
// payment headers, calls the verifier service to validate the signature, and
// forwards the text to the AI service. The handler respects context timeouts
// applied by middleware and returns appropriate HTTP errors (402, 403, 504,
// 500) to the client.
func handleSummarize(c *gin.Context) {
	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")

	if signature == "" || nonce == "" {
		c.Set("payment_verified", false)

		ctx := createPaymentContext()
		c.JSON(402, gin.H{
			"error":          "Payment Required",
			"paymentContext": ctx,
		paymentContext := createPaymentContext()
		c.JSON(402, gin.H{
			"error":          "Payment Required",
			"message":        "Please sign the payment context",
			"paymentContext": paymentContext,
		})
		return
	}

	verifyReq := VerifyRequest{
		Context: PaymentContext{
			Recipient: getRecipientAddress(),
			Token:     "USDC",
			Amount:    getPaymentAmount(),
			Nonce:     nonce,
			ChainID:   getChainID(),
		},
		Signature: signature,
	}

	body, _ := json.Marshal(verifyReq)
	resp, err := http.Post("http://127.0.0.1:3002/verify", "application/json", bytes.NewBuffer(body))
	if err != nil {
		c.JSON(500, gin.H{"error": "verifier unavailable"})
	// 2. Verify Payment (Call Rust Service)
	paymentCtx := PaymentContext{
		Recipient: getRecipientAddress(),
		Token:     "USDC",
		Amount:    getPaymentAmount(),
		Nonce:     nonce,
		ChainID:   getChainID(),
	}

	verifyReq := VerifyRequest{
		Context:   paymentCtx,
		Signature: signature,
	}

	verifyBody, err := json.Marshal(verifyReq)
	if err != nil {
		log.Printf("error marshaling verification request: %v", err)
		c.JSON(500, gin.H{"error": "Failed to create verification request"})
		return
	}
	verifierURL := os.Getenv("VERIFIER_URL")
	if verifierURL == "" {
		verifierURL = "http://127.0.0.1:3002"
	}
	// Call verifier with its own timeout
	verifierCtx, verifierCancel := context.WithTimeout(c.Request.Context(), getVerifierTimeout())
	defer verifierCancel()

	vreq, err := http.NewRequestWithContext(verifierCtx, "POST", verifierURL+"/verify", bytes.NewBuffer(verifyBody))
	if err != nil {
		// If the request cannot be created, return 500
		c.JSON(500, gin.H{"error": "Invalid verifier request", "details": err.Error()})
		return
	}
	vreq.Header.Set("Content-Type", "application/json")

	// Use http.DefaultClient and rely on verifierCtx for timeouts/cancellation.
	resp, err := http.DefaultClient.Do(vreq)
	if err != nil {
		// If the verifier or parent context timed out, return Gateway Timeout
		if errors.Is(err, context.DeadlineExceeded) || verifierCtx.Err() == context.DeadlineExceeded || c.Request.Context().Err() == context.DeadlineExceeded {
			c.JSON(504, gin.H{"error": "Gateway Timeout", "message": "Verifier request timed out"})
			return
		}
		c.JSON(500, gin.H{"error": "Verification service unavailable"})
		return
	}
	defer resp.Body.Close()

	var verifyResp VerifyResponse
	_ = json.NewDecoder(resp.Body).Decode(&verifyResp)

	if !verifyResp.IsValid {
		c.Set("payment_verified", false)
		c.JSON(403, gin.H{"error": "invalid signature"})
		return
	}

	c.Set("payment_verified", true)
	c.Set("user_wallet", verifyResp.RecoveredAddress)

	var req SummarizeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}

	summary, err := callOpenRouter(c.Request.Context(), req.Text)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		// If the error was due to a timeout, return 504
		if errors.Is(err, context.DeadlineExceeded) || c.Request.Context().Err() == context.DeadlineExceeded {
			c.JSON(504, gin.H{"error": "Gateway Timeout", "message": "AI request timed out"})
			return
		}
		c.JSON(500, gin.H{"error": "AI Service Failed", "details": err.Error()})
		return
	}

	c.JSON(200, gin.H{"result": summary})
}

/* -------------------- Helpers -------------------- */

func createPaymentContext() PaymentContext {
	return PaymentContext{
		Recipient: getRecipientAddress(),
		Token:     "USDC",
		Amount:    getPaymentAmount(),
		Nonce:     uuid.New().String(),
		ChainID:   getChainID(),
	}
}

func getRecipientAddress() string {
	addr := os.Getenv("RECIPIENT_ADDRESS")
	if addr == "" {
		return "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219"
	}
	return addr
}

func getPaymentAmount() string {
	a := os.Getenv("PAYMENT_AMOUNT")
	if a == "" {
		return "0.001"
	}
	return a
}

func getChainID() int {
	id := os.Getenv("CHAIN_ID")
	if id == "" {
		return 8453
	}
	n, err := strconv.Atoi(id)
	if err != nil {
		return 8453
	}
	return n
}

func callOpenRouter(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty text")
// callOpenRouter sends the given text to the OpenRouter chat completions API
// requesting a two-sentence summary and returns the generated summary.
// It reads OPENROUTER_API_KEY for authorization and OPENROUTER_MODEL to select
// the model (defaults to "z-ai/glm-4.5-air:free" if unset).
func callOpenRouter(ctx context.Context, text string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "z-ai/glm-4.5-air:free"
	}

	prompt := fmt.Sprintf("Summarize this text in 2 sentences: %s", text)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	openRouterURL := os.Getenv("OPENROUTER_URL")
	if openRouterURL == "" {
		openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create OpenRouter request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Use http.DefaultClient and rely on ctx for cancellation/timeouts.
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
	return "stub summary", nil
}

// Rate Limiting Functions

// initRateLimiters creates rate limiters for each tier
func initRateLimiters() map[string]RateLimiter {
	cleanupInterval := getEnvAsInt("RATE_LIMIT_CLEANUP_INTERVAL", 300)
	cleanupTTL := time.Duration(cleanupInterval) * time.Second

	return map[string]RateLimiter{
		"anonymous": NewTokenBucket(
			getEnvAsInt("RATE_LIMIT_ANONYMOUS_RPM", 10),
			getEnvAsInt("RATE_LIMIT_ANONYMOUS_BURST", 5),
			cleanupTTL,
		),
		"standard": NewTokenBucket(
			getEnvAsInt("RATE_LIMIT_STANDARD_RPM", 60),
			getEnvAsInt("RATE_LIMIT_STANDARD_BURST", 20),
			cleanupTTL,
		),
		"verified": NewTokenBucket(
			getEnvAsInt("RATE_LIMIT_VERIFIED_RPM", 120),
			getEnvAsInt("RATE_LIMIT_VERIFIED_BURST", 50),
			cleanupTTL,
		),
	}
}

// RateLimitMiddleware applies rate limiting to requests
func RateLimitMiddleware(limiters map[string]RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Determine rate limit key and tier
		key := getRateLimitKey(c)
		tier := selectRateLimitTier(c)
		limiter := limiters[tier]

		// Check if request is allowed
		if !limiter.Allow(key) {
			retryAfter := calculateRetryAfter(limiter, key)
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Header("X-RateLimit-Limit", strconv.Itoa(getLimitForTier(tier)))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", strconv.FormatInt(limiter.GetResetTime(key), 10))
			c.JSON(429, gin.H{
				"error":       "Too Many Requests",
				"message":     "Rate limit exceeded. Please retry later.",
				"retry_after": retryAfter,
			})
			c.Abort()
			return
		}

		// Add rate limit headers to successful responses
		c.Header("X-RateLimit-Limit", strconv.Itoa(getLimitForTier(tier)))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(limiter.GetRemaining(key)))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(limiter.GetResetTime(key), 10))

		c.Next()
	}
}

// getRateLimitKey determines the key for rate limiting (nonce/wallet > IP)
func getRateLimitKey(c *gin.Context) string {
	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")

	// Only use nonce-based key if BOTH signature and nonce are present
	// This prevents attackers from bypassing IP rate limits with fake nonces
	if signature != "" && nonce != "" {
		hash := sha256.Sum256([]byte(nonce))
		// Use 32 hex chars (128 bits) for better collision resistance
		return "nonce:" + hex.EncodeToString(hash[:])[:32]
	}

	return "ip:" + c.ClientIP()
}

// selectRateLimitTier determines which tier to apply based on request
func selectRateLimitTier(c *gin.Context) string {
	// Check if request has signature (authenticated)
	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")

	if signature != "" && nonce != "" {
		// Future: Check if user is verified/premium
		// For now, all signed requests get standard tier
		return "standard"
	}

	// Unsigned requests get anonymous tier
	return "anonymous"
}

// calculateRetryAfter calculates seconds until rate limit resets
func calculateRetryAfter(limiter RateLimiter, key string) int {
	resetTime := limiter.GetResetTime(key)
	now := time.Now().Unix()
	retryAfter := int(resetTime - now)
	if retryAfter < 1 {
		return 1
	}
	return retryAfter
}

// getLimitForTier returns the RPM limit for a given tier
func getLimitForTier(tier string) int {
	switch tier {
	case "anonymous":
		return getEnvAsInt("RATE_LIMIT_ANONYMOUS_RPM", 10)
	case "standard":
		return getEnvAsInt("RATE_LIMIT_STANDARD_RPM", 60)
	case "verified":
		return getEnvAsInt("RATE_LIMIT_VERIFIED_RPM", 120)
	default:
		return 10
	}
}

// getRateLimitEnabled checks if rate limiting is enabled
func getRateLimitEnabled() bool {
	enabled := strings.ToLower(os.Getenv("RATE_LIMIT_ENABLED"))
	return enabled == "true" || enabled == "1"
}

// getEnvAsInt retrieves an environment variable as an integer with a default value
func getEnvAsInt(key string, defaultValue int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		log.Printf("Warning: Invalid value for %s: %s, using default %d", key, valStr, defaultValue)
		return defaultValue
	}
	return val
}
