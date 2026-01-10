// Package main implements the gateway HTTP server used by MicroAI-Paygate.
// It provides request handlers, middleware, and configuration helpers
// for timeouts and rate limiting.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
    "os/signal"
    "syscall"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

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

	r := gin.Default()

	r.StaticFile("/openapi.yaml", "openapi.yaml")

	r.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(200, `
<!DOCTYPE html>
<html>
<head>
  <title>MicroAI Paygate Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: '/openapi.yaml',
      dom_id: '#swagger-ui'
    });
  </script>
</body>
</html>
`)
	})

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3001"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "X-402-Signature", "X-402-Nonce"},
		ExposeHeaders:    []string{"Content-Length", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After", "X-402-Receipt"},
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
	r.Use(TrackInFlightRequests())

	// Health check with shorter timeout (2s)
	r.GET("/healthz", RequestTimeoutMiddleware(getHealthCheckTimeout()), handleHealth)

	// AI endpoints with AI-specific timeout (30s)
	aiGroup := r.Group("/api/ai")
	aiGroup.Use(RequestTimeoutMiddleware(getAITimeout()))
	aiGroup.POST("/summarize", handleSummarize)

	// Receipt lookup endpoint
	// Note: Rate limiting applies only if enabled globally via RATE_LIMIT_ENABLED=true
	// Random 12-char receipt IDs (2^48 space) make brute-force enumeration impractical
	r.GET("/api/receipts/:id", handleGetReceipt)

	// Initialize receipt cleanup goroutine
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer func() {
		cleanupCancel()
		// Perform final cleanup on shutdown to prevent receipt leak
		cleanupExpiredReceipts()
		log.Println("Final receipt cleanup completed on shutdown")
	}()
	go startReceiptCleanup(cleanupCtx)
	log.Println("Receipt cleanup goroutine started")

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

addr := ":" + port

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}


	go func() {
		log.Printf("[INFO] Gateway listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] listen error: %v", err)
		}
	}()

	// ---- Graceful shutdown ----
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	log.Println("[INFO] Shutdown signal received, draining connections...")

	active := GetActiveRequestCount()
	if active > 0 {
		log.Printf("[INFO] Waiting for %d in-flight request(s)...", active)
		WaitForInFlightRequests()
		log.Println("[INFO] All in-flight requests completed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] Server forced to shutdown: %v", err)
	} else {
		log.Println("[OK] Server shutdown completed")
	}


}

// handleSummarize handles POST /api/ai/summarize requests. It validates
// payment headers, calls the verifier service to validate the signature, and
// forwards the text to the AI service. The handler respects context timeouts
// applied by middleware and returns appropriate HTTP errors (402, 403, 504,
// 500) to the client.
func handleSummarize(c *gin.Context) {
	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")

	// 1. Payment Required
	if signature == "" || nonce == "" {
		paymentContext := createPaymentContext()
		c.JSON(402, gin.H{
			"error":          "Payment Required",
			"message":        "Please sign the payment context",
			"paymentContext": paymentContext,
		})
		return
	}

	// Capture request body for receipt generation
	// Limit request body to 10MB to prevent memory exhaustion attacks
	maxBodySize := int64(10 * 1024 * 1024)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodySize)

	requestBody, err := c.GetRawData()
	if err != nil {
		log.Printf("error reading request body: %v", err)
		// Return 413 if body exceeds size limit, 500 for other errors
		if err.Error() == "http: request body too large" {
			c.JSON(413, gin.H{"error": "Payload too large", "max_size": "10MB"})
		} else {
			c.JSON(500, gin.H{"error": "Failed to read request body"})
		}
		return
	}
	// Set body to NoBody since we've already read it into requestBody
	// We'll use json.Unmarshal(requestBody, &req) later instead of c.BindJSON
	c.Request.Body = http.NoBody

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
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		c.JSON(500, gin.H{"error": "Failed to decode verification response"})
		return
	}

	if !verifyResp.IsValid {
		c.JSON(403, gin.H{"error": "Invalid Signature", "details": verifyResp.Error})
		return
	}

	// 3. Parse request body
	var req SummarizeRequest
	if err := json.Unmarshal(requestBody, &req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}

	// 4. Call AI Service
	summary, err := callOpenRouter(c.Request.Context(), req.Text)
	if err != nil {
		// If the error was due to a timeout, return 504
		if errors.Is(err, context.DeadlineExceeded) || c.Request.Context().Err() == context.DeadlineExceeded {
			c.JSON(504, gin.H{"error": "Gateway Timeout", "message": "AI request timed out"})
			return
		}
		c.JSON(500, gin.H{"error": "AI Service Failed", "details": err.Error()})
		return
	}

	// 5. Generate cryptographic receipt
	// NOTE: Response hashing is performed on the AI response body
	// Large responses (>1MB) may cause slight delays during hashing
	// Expected typical response size: <100KB for summaries
	responseBody := []byte(summary) // Response body for hashing
	receipt, err := GenerateReceipt(paymentCtx, verifyResp.RecoveredAddress, c.Request.URL.Path, requestBody, responseBody)
	if err != nil {
		log.Printf("error generating receipt: %v", err)
		c.JSON(500, gin.H{"error": "Failed to generate receipt", "details": err.Error()})
		return
	}

	// 6. Store receipt with TTL
	if err := storeReceipt(receipt, getReceiptTTL()); err != nil {
		log.Printf("error storing receipt: %v", err)
		c.JSON(500, gin.H{"error": "Failed to store receipt"})
		return
	}

	// 7. Encode receipt for header
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		log.Printf("error marshaling receipt: %v", err)
		c.JSON(500, gin.H{"error": "Failed to encode receipt"})
		return
	}
	receiptBase64 := base64.StdEncoding.EncodeToString(receiptJSON)

	// 8. Add receipt to response
	c.Header("X-402-Receipt", receiptBase64)
	c.JSON(200, gin.H{
		"result":  summary,
		"receipt": receipt,
	})
}

// createPaymentContext constructs a PaymentContext prefilled with the recipient address (from RECIPIENT_ADDRESS or a fallback), the USDC token, amount "0.001", a newly generated UUID nonce, and chain ID 8453.
func createPaymentContext() PaymentContext {
	return PaymentContext{
		Recipient: getRecipientAddress(),
		Token:     "USDC",
		Amount:    getPaymentAmount(),
		Nonce:     uuid.New().String(),
		ChainID:   getChainID(),
	}
}

// getRecipientAddress retrieves the recipient address from the RECIPIENT_ADDRESS environment variable.
// If RECIPIENT_ADDRESS is unset, it logs a warning and returns the default address "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219".
func getRecipientAddress() string {
	addr := os.Getenv("RECIPIENT_ADDRESS")
	if addr == "" {
		log.Println("Warning: RECIPIENT_ADDRESS not set, using default")
		return "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219"
	}
	return addr
}

// getPaymentAmount returns the payment amount from the PAYMENT_AMOUNT environment variable.
// If unset, it defaults to "0.001".
func getPaymentAmount() string {
	amount := os.Getenv("PAYMENT_AMOUNT")
	if amount == "" {
		return "0.001"
	}
	return amount
}

// getChainID returns the blockchain chain ID from the CHAIN_ID environment variable.
// If unset or invalid, it defaults to 8453 (Base).
func getChainID() int {
	chainIDStr := os.Getenv("CHAIN_ID")
	if chainIDStr == "" {
		return 8453
	}
	chainID, err := strconv.Atoi(chainIDStr)
	if err != nil {
		log.Printf("Warning: Invalid CHAIN_ID '%s', using default 8453", chainIDStr)
		return 8453
	}
	return chainID
}

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

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "gateway"})
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

// Receipt Management Functions

var (
	receiptStoreMu         sync.RWMutex
	receiptStore           = make(map[string]*receiptEntry)
	receiptCleanupInterval = 5 * time.Minute
)

type receiptEntry struct {
	receipt   *SignedReceipt
	expiresAt time.Time
}

// startReceiptCleanup runs periodic cleanup in a single goroutine
// This prevents goroutine leaks by using a single background worker
// instead of spawning one goroutine per receipt
func startReceiptCleanup(ctx context.Context) {
	ticker := time.NewTicker(receiptCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Receipt cleanup goroutine stopped")
			return
		case <-ticker.C:
			cleanupExpiredReceipts()
		}
	}
}

// cleanupExpiredReceipts removes expired receipts from the store
func cleanupExpiredReceipts() {
	now := time.Now()
	receiptStoreMu.Lock()
	defer receiptStoreMu.Unlock()

	count := 0
	for id, entry := range receiptStore {
		if now.After(entry.expiresAt) {
			delete(receiptStore, id)
			count++
		}
	}

	if count > 0 {
		log.Printf("Cleaned up %d expired receipts", count)
	}
}

// storeReceipt stores a receipt with TTL
// Returns error for future extensibility (Redis/Postgres implementations)
func storeReceipt(receipt *SignedReceipt, ttl time.Duration) error {
	// Validate receipt format before storage
	if err := validateReceipt(receipt); err != nil {
		return fmt.Errorf("invalid receipt format: %w", err)
	}

	receiptStoreMu.Lock()
	defer receiptStoreMu.Unlock()

	receiptStore[receipt.Receipt.ID] = &receiptEntry{
		receipt:   receipt,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// validateReceipt validates that a receipt has all required fields
func validateReceipt(receipt *SignedReceipt) error {
	if receipt == nil {
		return fmt.Errorf("receipt is nil")
	}

	// Validate receipt fields
	if receipt.Receipt.ID == "" {
		return fmt.Errorf("receipt ID is empty")
	}
	if !strings.HasPrefix(receipt.Receipt.ID, "rcpt_") {
		return fmt.Errorf("receipt ID must start with 'rcpt_'")
	}
	if receipt.Receipt.Version == "" {
		return fmt.Errorf("receipt version is empty")
	}
	if receipt.Receipt.Timestamp.IsZero() {
		return fmt.Errorf("receipt timestamp is zero")
	}

	// Validate payment details
	if receipt.Receipt.Payment.Payer == "" {
		return fmt.Errorf("payer address is empty")
	}
	if receipt.Receipt.Payment.Recipient == "" {
		return fmt.Errorf("recipient address is empty")
	}
	if receipt.Receipt.Payment.Amount == "" {
		return fmt.Errorf("payment amount is empty")
	}
	if receipt.Receipt.Payment.Token == "" {
		return fmt.Errorf("token is empty")
	}
	if receipt.Receipt.Payment.Nonce == "" {
		return fmt.Errorf("nonce is empty")
	}

	// Validate service details
	if receipt.Receipt.Service.Endpoint == "" {
		return fmt.Errorf("service endpoint is empty")
	}
	if receipt.Receipt.Service.RequestHash == "" {
		return fmt.Errorf("request hash is empty")
	}
	if receipt.Receipt.Service.ResponseHash == "" {
		return fmt.Errorf("response hash is empty")
	}

	// Validate signature
	if receipt.Signature == "" {
		return fmt.Errorf("signature is empty")
	}
	if !strings.HasPrefix(receipt.Signature, "0x") {
		return fmt.Errorf("signature must start with '0x'")
	}

	// Validate server public key
	if receipt.ServerPublicKey == "" {
		return fmt.Errorf("server public key is empty")
	}
	if !strings.HasPrefix(receipt.ServerPublicKey, "0x") {
		return fmt.Errorf("server public key must start with '0x'")
	}

	return nil
}

// getReceipt retrieves a receipt by ID
func getReceipt(id string) (*SignedReceipt, bool) {
	receiptStoreMu.RLock()
	defer receiptStoreMu.RUnlock()

	entry, exists := receiptStore[id]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.receipt, true
}

// getReceiptTTL returns configured TTL or default 24h
func getReceiptTTL() time.Duration {
	ttlSeconds := getEnvAsInt("RECEIPT_TTL", 86400)
	return time.Duration(ttlSeconds) * time.Second
}

// handleGetReceipt handles GET /api/receipts/:id
func handleGetReceipt(c *gin.Context) {
	id := c.Param("id")

	receipt, exists := getReceipt(id)
	if !exists {
		c.JSON(404, gin.H{
			"error":   "Receipt not found",
			"message": "Receipt may have expired or never existed",
		})
		return
	}

	c.JSON(200, gin.H{
		"receipt":           receipt.Receipt,
		"signature":         receipt.Signature,
		"server_public_key": receipt.ServerPublicKey,
		"status":            "valid",
	})
}

// Server private key management
var (
	serverPrivateKey     *ecdsa.PrivateKey
	serverPrivateKeyOnce sync.Once
	serverPrivateKeyErr  error
)

// getServerPrivateKey loads the server's private key (cached with sync.Once)
// This prevents race conditions and ensures the key is loaded only once
func getServerPrivateKey() (*ecdsa.PrivateKey, error) {
	serverPrivateKeyOnce.Do(func() {
		keyHex := os.Getenv("SERVER_WALLET_PRIVATE_KEY")
		if keyHex == "" {
			serverPrivateKeyErr = fmt.Errorf("SERVER_WALLET_PRIVATE_KEY not set")
			return
		}

		// Remove 0x prefix if present
		keyHex = strings.TrimPrefix(keyHex, "0x")

		keyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			serverPrivateKeyErr = fmt.Errorf("invalid private key format: %w", err)
			return
		}

		// Validate minimum key length to prevent trivially weak keys
		// Keys shorter than 16 bytes (128 bits) are cryptographically insecure
		if len(keyBytes) < 16 {
			serverPrivateKeyErr = fmt.Errorf("private key too short: got %d bytes, expected at least 16 bytes (128 bits)", len(keyBytes))
			return
		}

		// Left-pad to 32 bytes if necessary (handles keys with leading zeros like 0x0001...)
		// Keys between 16-31 bytes are valid but need padding
		if len(keyBytes) < 32 {
			padded := make([]byte, 32)
			copy(padded[32-len(keyBytes):], keyBytes)
			keyBytes = padded
		} else if len(keyBytes) > 32 {
			serverPrivateKeyErr = fmt.Errorf("private key must be at most 32 bytes, got %d bytes", len(keyBytes))
			return
		}

		privateKey, err := crypto.ToECDSA(keyBytes)
		if err != nil {
			serverPrivateKeyErr = fmt.Errorf("failed to parse private key: %w", err)
			return
		}

		serverPrivateKey = privateKey
		log.Println("Server private key loaded successfully")
	})

	return serverPrivateKey, serverPrivateKeyErr
}
