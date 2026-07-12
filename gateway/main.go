// Package main implements the gateway HTTP server used by MicroAI-Paygate.
// It provides request handlers, middleware, and configuration helpers
// for timeouts and rate limiting.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gateway/internal/ai"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PaymentContext struct {
	Recipient string `json:"recipient"`
	Token     string `json:"token"`
	Amount    string `json:"amount"`
	Nonce     string `json:"nonce"`
	ChainID   int    `json:"chainId"`
	Timestamp uint64 `json:"timestamp"`
}

type VerifyRequest struct {
	Context   PaymentContext `json:"context"`
	Signature string         `json:"signature"`
}

type VerifyResponse struct {
	IsValid          bool   `json:"is_valid"`
	RecoveredAddress string `json:"recovered_address"`
	Error            string `json:"error"`
	ErrorCode        string `json:"error_code"`
}

type SummarizeRequest struct {
	Text string `json:"text"`
}

var receiptIDPattern = regexp.MustCompile(`^rcpt_[a-f0-9]{12}$`)

// validateConfig validates all required environment variables at startup.
// It checks for OPENROUTER_API_KEY, SERVER_WALLET_PRIVATE_KEY, and conditionally REDIS_URL.
// Returns an error listing all missing variables if any are not set.
func validateConfig() error {
	required := []string{
		"SERVER_WALLET_PRIVATE_KEY", // Critical for signing receipts
		"VERIFIER_URL",              // Where the gateway calls /verify; loopback fallback would hide misconfig in prod
	}

	// Add provider-specific requirements
	providerType := os.Getenv("AI_PROVIDER")
	if providerType == "" || providerType == "openrouter" {
		required = append(required, "OPENROUTER_API_KEY")
	}

	if err := validateReceiptStoreMode(); err != nil {
		return err
	}

	// Add REDIS_URL to required if caching or Redis-backed receipts are enabled.
	if isRedisRequired() {
		required = append(required, "REDIS_URL")
	}

	// Iterate and collect all missing vars, returning a comprehensive error
	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missing)
	}

	// Validate SERVER_WALLET_PRIVATE_KEY format early (before server starts accepting traffic)
	if err := validateServerPrivateKey(); err != nil {
		return fmt.Errorf("SERVER_WALLET_PRIVATE_KEY validation failed: %w", err)
	}

	// Validate REDIS_URL format if Redis is required.
	if isRedisRequired() {
		if err := validateRedisURL(); err != nil {
			return fmt.Errorf("REDIS_URL validation failed: %w", err)
		}
	}

	// Normalize RECIPIENT_ADDRESS to EIP-55 canonical so wallet libraries
	// don't reject it during EIP-712 signing.
	if err := normalizeRecipientAddress(); err != nil {
		return fmt.Errorf("RECIPIENT_ADDRESS validation failed: %w", err)
	}

	return nil
}

// normalizeRecipientAddress reads RECIPIENT_ADDRESS, rejects it if it isn't a
// valid 20-byte hex address, and rewrites the env var to the EIP-55 canonical
// form so paymentContext.recipient is always checksummed correctly. A
// non-canonical case slipping through silently caused the
// `bad address checksum` rejection at the wallet during the first live deploy.
// An unset RECIPIENT_ADDRESS is allowed: getRecipientAddress() falls back to a
// hardcoded canonical default that is already EIP-55-correct.
func normalizeRecipientAddress() error {
	raw := os.Getenv("RECIPIENT_ADDRESS")
	if raw == "" {
		return nil
	}
	if !common.IsHexAddress(raw) {
		return fmt.Errorf("RECIPIENT_ADDRESS is not a valid hex address: %s", raw)
	}
	canonical := common.HexToAddress(raw).Hex()
	if canonical != raw {
		log.Printf("[INFO] normalized RECIPIENT_ADDRESS to EIP-55 canonical: %s -> %s", raw, canonical)
		if err := os.Setenv("RECIPIENT_ADDRESS", canonical); err != nil {
			return fmt.Errorf("failed to rewrite RECIPIENT_ADDRESS: %w", err)
		}
	}
	return nil
}

// validateServerPrivateKey validates the private key format without loading it into memory.
// It checks that the key is valid hex, has proper length (31-32 bytes), and handles 0x prefix.
// This prevents runtime failures when the server tries to sign receipts.
func validateServerPrivateKey() error {
	keyHex := os.Getenv("SERVER_WALLET_PRIVATE_KEY")
	if keyHex == "" {
		return fmt.Errorf("SERVER_WALLET_PRIVATE_KEY not set")
	}

	// Remove 0x prefix if present
	keyHex = strings.TrimPrefix(keyHex, "0x")

	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return fmt.Errorf("invalid private key format: %w", err)
	}

	// Validate key length (same validation as getServerPrivateKey)
	if len(keyBytes) < 31 {
		return fmt.Errorf("private key too short: got %d bytes, expected at least 31 bytes", len(keyBytes))
	}

	if len(keyBytes) > 32 {
		return fmt.Errorf("private key must be at most 32 bytes, got %d bytes", len(keyBytes))
	}

	return nil
}

// validateRedisURL validates the Redis URL format without connecting.
// It supports both redis:// URLs and host:port format.
// Only called when CACHE_ENABLED=true to ensure Redis is properly configured.
func validateRedisURL() error {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return fmt.Errorf("REDIS_URL not set but Redis is required")
	}

	// Handle redis:// or rediss:// schemes
	if strings.HasPrefix(redisURL, "redis://") || strings.HasPrefix(redisURL, "rediss://") {
		// Parse the URL to extract the host:port
		u, err := url.Parse(redisURL)
		if err != nil || u.Host == "" {
			return fmt.Errorf("invalid REDIS_URL format")
		}
		// Verify host:port format
		parts := strings.Split(u.Host, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid REDIS_URL format")
		}
		return nil
	}

	// Handle plain host:port format
	parts := strings.Split(redisURL, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("REDIS_URL must be in format 'host:port'")
	}

	return nil
}

// Global AI provider instance
var aiProvider ai.Provider

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

	// Initialize AI provider
	aiProvider, err = ai.NewProvider()
	if err != nil {
		fmt.Printf("[Error] Failed to initialize AI provider: %v\n", err)
		os.Exit(1)
	}
	providerType := os.Getenv("AI_PROVIDER")
	if providerType == "" {
		providerType = "openrouter"
	}
	fmt.Printf("[OK] AI Provider initialized: %s\n", providerType)

	if port := os.Getenv("PORT"); port != "" {
		fmt.Printf("    - Port: %s\n", port)
	}
	modelKey := "OPENROUTER_MODEL"
	if providerType == "ollama" {
		modelKey = "OLLAMA_MODEL"
	}
	if model := os.Getenv(modelKey); model != "" {
		fmt.Printf("    - %s: %s\n", modelKey, model)
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
	if os.Getenv(modelKey) == "" {
		fmt.Printf("[WARN] %s not set, using provider default model\n", modelKey)
	}
	if os.Getenv("CHAIN_ID") == "" {
		fmt.Println("[WARN] CHAIN_ID not set, using default: 84532(Base Sepolia)")
	}

	r := gin.Default()

	// Restrict trusted proxies to prevent X-Forwarded-For spoofing.
	// IP-based rate limiting relies on c.ClientIP(), which reads
	// X-Forwarded-For only from proxies in this list. An empty list
	// means only the direct remote address is trusted.
	// Set TRUSTED_PROXIES env var (comma-separated CIDRs) for production.
	if trustedProxies := getTrustedProxies(); len(trustedProxies) > 0 {
		if err := r.SetTrustedProxies(trustedProxies); err != nil {
			_ = r.SetTrustedProxies(nil)
			log.Printf("[WARN] invalid TRUSTED_PROXIES value, falling back to no trusted proxies: %v", err)
		}
	} else {
		// Trust no proxies: always use direct RemoteAddr for ClientIP.
		_ = r.SetTrustedProxies(nil)
	}

	// Register the Correlation ID middleware first so every request,
	// including those rejected by later middleware, carries an ID for tracing.
	r.Use(CorrelationIDMiddleware())

	metricsPath := getMetricsPath()

	if getMetricsEnabled() {
		if err := validateMetricsPath(metricsPath); err != nil {
			log.Fatalf("Invalid metrics configuration: %v", err)
		}
		r.Use(MetricsMiddleware())
		r.GET(metricsPath, gin.WrapH(promhttp.Handler()))
	}

	// Configure GZIP compression for API responses
	// - Uses DefaultCompression for balance between speed and size
	// - Excludes /metrics endpoint (if added in future)
	// - Compression is transparent to receipt verification (hashes uncompressed body)
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{metricsPath})))

	// Initialize Redis early to fail-fast if Redis required but unavailable
	if err := initRedis(); err != nil {
		log.Fatalf("Redis initialization failed: %v", err)
	}
	if err := initReceiptStore(); err != nil {
		log.Fatalf("Receipt store initialization failed: %v", err)
	}

	// Documentation routes are registered before CORS / rate-limit / timeout
	// middleware so the Swagger UI and raw spec are served without those
	// constraints.
	registerDocRoutes(r)

	r.Use(cors.New(cors.Config{
		AllowOrigins: getAllowedOrigins(),
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"X-402-Signature",
			"X-402-Nonce",
			"X-402-Timestamp",
			"X-Correlation-ID",
		},
		ExposeHeaders: []string{
			"Content-Length",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			"Retry-After",
			"X-402-Receipt",
			"X-Correlation-ID",
		},
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

	// Public API routes. Kept in registerAPIRoutes so the OpenAPI coverage
	// test in openapi_test.go can introspect them without booting middleware.
	// Rate limiting (if enabled) applies via the global r.Use above; random
	// 12-char receipt IDs (2^48 space) make /api/receipts/:id brute-force
	// enumeration impractical.
	registerAPIRoutes(r)

	// Initialize receipt cleanup goroutine
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer func() {
		cleanupCancel()
		// Perform final cleanup on shutdown to prevent receipt leak
		cleanupExpiredReceipts(context.Background())
		log.Println("Final receipt cleanup completed on shutdown")
		// Close Redis connection if active
		if redisClient != nil {
			redisClient.Close()
			log.Println("Redis connection closed")
		}
	}()
	go startReceiptCleanup(cleanupCtx)
	log.Println("Receipt cleanup goroutine started")

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	startupErrCh := make(chan error, 1)
	go func() {
		log.Printf("Go Gateway listening on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			startupErrCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("Signal %s received, shutting down (max 30s)...", sig)
	case err := <-startupErrCh:
		log.Printf("Server failed to start: %v", err)
		return
	}

	signal.Stop(quit)

	cleanupCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server stopped gracefully")
	}
}

// handleSummarize handles POST /api/ai/summarize requests. It validates
// payment headers, calls the verifier service to validate the signature, and
// forwards the text to the AI service. The handler respects context timeouts
// applied by middleware and returns appropriate HTTP errors (402, 403, 504,
// 500) to the client.
func handleSummarize(c *gin.Context) {
	// 1. Payment Verification
	// Note: CacheMiddleware aborts on cache HIT, so this handler only runs on cache MISS or when caching is disabled
	var requestBody []byte
	var err error

	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")
	timestampHeader := c.GetHeader("X-402-Timestamp")

	// Basic check
	if signature == "" || nonce == "" {
		c.JSON(402, gin.H{
			"error":          "Payment Required",
			"message":        "Please sign the payment context",
			"paymentContext": createPaymentContext(),
		})
		return
	}

	if timestampHeader == "" {
		respondError(c, 400, "invalid_timestamp", fmt.Errorf("missing X-402-Timestamp header"))
		return
	}

	timestampValue, err := strconv.ParseUint(timestampHeader, 10, 64)
	if err != nil || timestampValue == 0 {
		respondError(c, 400, "invalid_timestamp", fmt.Errorf("invalid X-402-Timestamp header"))
		return
	}

	// Check if body already read by middleware
	if body, exists := c.Get("request_body"); exists {
		// Cache middleware always sets this as []byte, safe to assert
		requestBody = body.([]byte)
	}

	// Read body if not already available
	if requestBody == nil {
		// Read body with limit (only if middleware didn't process it)
		const maxBodySize = 10 * 1024 * 1024
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxBodySize))
		requestBody, err = io.ReadAll(c.Request.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				c.JSON(413, gin.H{"error": "Payload too large", "max_size": "10MB"})
			} else {
				respondError(c, 500, "request_body_read_failed", err)
			}
			return
		}
	}

	// Verify
	verifyResp, paymentCtx, err := verifyPayment(c.Request.Context(), signature, nonce, uint64(timestampValue))
	if err != nil {
		verificationTotal.WithLabelValues("error").Inc()

		if errors.Is(err, context.DeadlineExceeded) {
			respondError(c, 504, "verifier_timeout", err)
		} else {
			respondError(c, 502, "verification_unavailable", err)
		}
		return
	}

	if !verifyResp.IsValid {
		verificationTotal.WithLabelValues("invalid").Inc()

		respondVerificationFailure(c, verifyResp)
		return
	}
	if verifyResp.RecoveredAddress == "" {
		verificationTotal.WithLabelValues("error").Inc()
		respondError(c, 502, "verification_unavailable", fmt.Errorf("verifier success missing recovered_address"))
		return
	}

	verificationTotal.WithLabelValues("success").Inc()

	// 2. Parse Request
	var req SummarizeRequest
	if err := json.Unmarshal(requestBody, &req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate text is not empty (also validated in cache middleware, but needed here for non-cached requests)
	if req.Text == "" {
		c.JSON(400, gin.H{"error": "Invalid request", "message": "text field cannot be empty"})
		return
	}

	// 3. Call AI Service
	summary, err := aiProvider.Generate(c.Request.Context(), req.Text)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(c.Request.Context().Err(), context.DeadlineExceeded) {
			respondError(c, 504, "upstream_timeout", err)
		} else {
			respondError(c, 502, "upstream_unavailable", err)
		}
		return
	}

	// 4. Generate & Send Receipt
	if err := generateAndSendReceipt(c, *paymentCtx, verifyResp.RecoveredAddress, requestBody, summary); err != nil {
		log.Printf("Failed to generate receipt: %v", err)
		// generateAndSendReceipt sends error response if it fails?
		// No, it returns error, we might have already written status if we aren't careful.
		// Let's implement generateAndSendReceipt to handle sending response.
		return
	}
}

// verifyPayment calls the verification service.

func verifyPayment(ctx context.Context, signature, nonce string, timestamp uint64) (*VerifyResponse, *PaymentContext, error) {
	paymentCtx := PaymentContext{
		Recipient: getRecipientAddress(),
		Token:     "USDC",
		Amount:    getPaymentAmount(),
		Nonce:     nonce,
		ChainID:   getChainID(),
		Timestamp: timestamp,
	}

	verifyReq := VerifyRequest{
		Context:   paymentCtx,
		Signature: signature,
	}

	verifyBody, err := json.Marshal(verifyReq)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal verification request: %w", err)
	}

	// VERIFIER_URL is guaranteed non-empty here: validateConfig() exits at
	// startup if it's unset, so no loopback fallback is needed.
	verifierURL := os.Getenv("VERIFIER_URL")

	// Use a separate context for verifier timeout to avoid hanging
	verifierCtx, verifierCancel := context.WithTimeout(ctx, getVerifierTimeout())
	defer verifierCancel()

	vreq, err := http.NewRequestWithContext(verifierCtx, "POST", verifierURL+"/verify", bytes.NewBuffer(verifyBody))
	if err != nil {
		return nil, nil, fmt.Errorf("create verifier request: %w", err)
	}
	vreq.Header.Set("Content-Type", "application/json")

	if cid, ok := ctx.Value(CorrelationIDKey).(string); ok {
		vreq.Header.Set("X-Correlation-ID", cid)
	}

	// Use http.DefaultClient and rely on verifierCtx for timeouts/cancellation.
	resp, err := http.DefaultClient.Do(vreq)
	if err != nil {
		return nil, nil, fmt.Errorf("verifier request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		bodyText := strings.TrimSpace(string(body))
		var verifyResp VerifyResponse
		if bodyText != "" && json.Unmarshal(body, &verifyResp) == nil && isVerifierBusinessRejection(&verifyResp) {
			return &verifyResp, &paymentCtx, nil
		}
		if bodyText == "" {
			return nil, nil, fmt.Errorf("verifier returned status %d", resp.StatusCode)
		}
		return nil, nil, fmt.Errorf("verifier returned status %d: %s", resp.StatusCode, bodyText)
	}

	var verifyResp VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return nil, nil, fmt.Errorf("decode verification response: %w", err)
	}

	return &verifyResp, &paymentCtx, nil
}

// generateAndSendReceipt handles receipt generation, storage, and sending the final JSON response.
// The receipt is sent ONLY in the X-402-Receipt header, not in the response body,
// to ensure the ResponseHash in the receipt matches the actual JSON body clients receive.
func generateAndSendReceipt(c *gin.Context, paymentCtx PaymentContext, recoveredAddr string, requestBody []byte, aiResult string) error {
	// Construct the response body that will be sent to client (without receipt)
	responseMap := map[string]interface{}{
		"result": aiResult,
	}
	responseBody, err := json.Marshal(responseMap)
	if err != nil {
		respondError(c, 500, "response_encoding_failed", err)
		return err
	}

	// Generate receipt with the actual response body hash
	receipt, err := GenerateReceipt(paymentCtx, recoveredAddr, c.Request.URL.Path, requestBody, responseBody)
	if err != nil {
		respondError(c, 500, "receipt_generation_failed", err)
		return err
	}

	if err := storeReceiptWithContext(c.Request.Context(), receipt, getReceiptTTL()); err != nil {
		respondError(c, 500, "receipt_store_failed", err)
		return err
	}

	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		respondError(c, 500, "receipt_encoding_failed", err)
		return err
	}
	receiptBase64 := base64.StdEncoding.EncodeToString(receiptJSON)

	// Send receipt in header only (not in body) so ResponseHash matches body
	c.Header("X-402-Receipt", receiptBase64)
	c.JSON(200, responseMap)
	return nil
}

// createPaymentContext constructs a PaymentContext prefilled with the recipient address (from RECIPIENT_ADDRESS or a fallback), the USDC token, amount "0.001", a newly generated UUID nonce, and the configured chain ID.
func createPaymentContext() PaymentContext {
	return PaymentContext{
		Recipient: getRecipientAddress(),
		Token:     "USDC",
		Amount:    getPaymentAmount(),
		Nonce:     uuid.New().String(),
		ChainID:   getChainID(),
		Timestamp: uint64(time.Now().Unix()),
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
// If unset or invalid, it defaults to 84532 (Base Sepolia).
func getChainID() int {
	chainIDStr := os.Getenv("CHAIN_ID")
	if chainIDStr == "" {
		return 84532
	}
	chainID, err := strconv.Atoi(chainIDStr)
	if err != nil {
		log.Printf("Warning: Invalid CHAIN_ID '%s', using default 84532", chainIDStr)
		return 84532
	}
	return chainID
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

			routePath := c.FullPath()
			if routePath == "" {
				routePath = "unknown"
			}
			rateLimitHits.WithLabelValues(routePath).Inc()

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

// getRateLimitKey determines the key for rate limiting (always uses IP)
func getRateLimitKey(c *gin.Context) string {
	// REMOVED: Nonce-based keying
	// Nonces must be unique per request (replay attack prevention),
	// which creates infinite buckets and memory leaks.
	// ALWAYS use IP for now to prevent infinite-bucket attacks
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

// getTrustedProxies returns the list of trusted proxy CIDRs/IPs from the
// TRUSTED_PROXIES environment variable (comma-separated). Returns nil when
// the variable is unset so the caller can disable proxy trust entirely.
func getTrustedProxies() []string {
	raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	trusted := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			trusted = append(trusted, trimmed)
		}
	}
	return trusted
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

// validateReceipt validates that a receipt has all required fields
func validateReceipt(receipt *SignedReceipt) error {
	if receipt == nil {
		return fmt.Errorf("receipt is nil")
	}

	// Validate receipt fields
	if !isValidReceiptID(receipt.Receipt.ID) {
		return fmt.Errorf("invalid receipt ID format")
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

// getReceiptTTL returns configured TTL or default 24h
func getReceiptTTL() time.Duration {
	ttlSeconds := getEnvAsInt("RECEIPT_TTL", 86400)
	if ttlSeconds <= 0 {
		ttlSeconds = 86400
	}
	return time.Duration(ttlSeconds) * time.Second
}
func isValidReceiptID(id string) bool {
	return receiptIDPattern.MatchString(id)
}

// handleGetReceipt handles GET /api/receipts/:id
func handleGetReceipt(c *gin.Context) {
	id := c.Param("id")

	// Reject malformed IDs early (no store hit)
	if !isValidReceiptID(id) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid receipt id format",
			"message": "receipt id must start with rcpt_ followed by exactly 12 lowercase hexadecimal characters",
		})
		return
	}

	receipt, exists, err := getReceiptWithContext(c.Request.Context(), id)
	if err != nil {
		log.Printf("Failed to retrieve receipt %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve receipt",
		})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Receipt not found",
			"message": "Receipt may have expired or never existed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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
		// Keys shorter than 31 bytes are cryptographically insecure or malformed
		if len(keyBytes) < 31 {
			serverPrivateKeyErr = fmt.Errorf("private key too short: got %d bytes, expected at least 31 bytes", len(keyBytes))
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

// handleHealthz implements the liveness probe for the gateway service.
// It returns a 200 OK status if the server is running and reachable.
// Response format: {"status": "ok", "service": "gateway", "timestamp": <unix_time>}
func handleHealthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "gateway", "timestamp": time.Now().Unix()})
}

// handleReadyz implements the readiness probe for the gateway service.
// It performs a comprehensive health check by verifying:
// 1. Connectivity to the Verifier service
// 2. Availability of the active AI provider
// 3. Redis connectivity (if caching or Redis-backed receipts are enabled)
// 4. Self-health metrics (goroutine count, memory usage)
// Returns 200 OK if all dependencies are healthy, otherwise 503 Service Unavailable.
func handleReadyz(c *gin.Context) {
	checks := make(map[string]interface{})

	//1. check verifier connectivity
	verifierStatus := checkVerifierHealth()
	checks["verifier"] = verifierStatus

	//2. Check active AI provider availability
	providerType := os.Getenv("AI_PROVIDER")
	if providerType == "" {
		providerType = "openrouter"
	}
	aiStatus := "unsupported"
	switch providerType {
	case "openrouter":
		aiStatus = checkOpenRouterHealth()
		checks["openrouter"] = aiStatus
	case "ollama":
		aiStatus = checkOllamaHealth()
		checks["ollama"] = aiStatus
	}
	checks["ai_provider"] = gin.H{
		"provider": providerType,
		"status":   aiStatus,
	}

	//3. Check Redis connectivity (if caching or Redis-backed receipts are enabled)
	redisStatus := "disabled"
	if isRedisRequired() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		if redisClient == nil {
			redisStatus = "unreachable"
		} else if err := redisClient.Ping(ctx).Err(); err != nil {
			redisStatus = "unreachable"
		} else {
			redisStatus = "ok"
		}
	}
	checks["redis"] = redisStatus

	//4. Self-health metrics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	checks["gateway"] = gin.H{
		"goroutines":      runtime.NumGoroutine(),
		"memory_alloc_mb": memStats.Alloc / 1024 / 1024,
		"memory_sys_mb":   memStats.Sys / 1024 / 1024,
		"status":          "ok",
	}

	//Overall status logic - include Redis if caching or Redis-backed receipts are enabled
	ready := verifierStatus == "ok" && aiStatus == "ok"
	if isRedisRequired() {
		ready = ready && redisStatus == "ok"
	}

	statusCode := http.StatusOK
	if !ready {
		statusCode = http.StatusServiceUnavailable
	}
	c.JSON(statusCode, gin.H{"ready": ready, "timestamp": time.Now().Unix(), "checks": checks})
}

// checkVerifierHealth pings the Verifier service's health endpoint.
// It uses HEALTH_CHECK_TIMEOUT_SECONDS to prevent hanging.
// Returns:
// - "ok": Verifier is healthy (200 OK)
// - "degraded": Verifier is reachable but returned non-200 status
// - "unreachable": Verifier could not be contacted
var checkVerifierHealth = func() string {
	// VERIFIER_URL is guaranteed non-empty: validateConfig() exits at
	// startup if it's unset.
	verifierURL := os.Getenv("VERIFIER_URL")
	ctx, cancel := context.WithTimeout(context.Background(), getHealthCheckTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", verifierURL+"/health", nil)
	if err != nil {
		return "unreachable"
	}
	//req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "degraded"
	}
	return "ok"
}

// checkOllamaHealth checks the availability of the configured Ollama API.
// It attempts to fetch the local model list with HEALTH_CHECK_TIMEOUT_SECONDS.
// Returns:
// - "ok": API is reachable (200 OK)
// - "degraded": API is reachable but returned non-200 status
// - "unreachable": API could not be contacted
var checkOllamaHealth = func() string {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	healthURL := strings.TrimSuffix(ollamaURL, "/") + "/api/tags"

	ctx, cancel := context.WithTimeout(context.Background(), getHealthCheckTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return "unreachable"
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "degraded"
	}
	return "ok"
}

func getOpenRouterModelsURL() string {
	openRouterURL := strings.TrimRight(os.Getenv("OPENROUTER_URL"), "/")
	if openRouterURL == "" {
		return "https://openrouter.ai/api/v1/models"
	}
	if strings.HasSuffix(openRouterURL, "/chat/completions") {
		return strings.TrimSuffix(openRouterURL, "/chat/completions") + "/models"
	}
	if strings.HasSuffix(openRouterURL, "/api/v1") {
		return openRouterURL + "/models"
	}
	if strings.HasSuffix(openRouterURL, "/models") {
		return openRouterURL
	}
	return openRouterURL + "/api/v1/models"
}

// checkOpenRouterHealth checks the availability of the OpenRouter API.
// It attempts to fetch the list of models with HEALTH_CHECK_TIMEOUT_SECONDS.
// Returns:
// - "ok": API is reachable (200 OK)
// - "unconfigured": OPENROUTER_API_KEY is not set
// - "degraded": API is reachable but returned non-200 status
// - "unreachable": API could not be contacted
var checkOpenRouterHealth = func() string {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "unconfigured"
	}
	healthURL := getOpenRouterModelsURL()

	ctx, cancel := context.WithTimeout(context.Background(), getHealthCheckTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return "unreachable"
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "degraded"
	}
	return "ok"
}
