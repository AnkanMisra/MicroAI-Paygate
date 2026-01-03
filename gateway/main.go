package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
	err := godotenv.Load("../.env")
	if err != nil {
		log.Println("Warning: Error loading .env file")
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
		ExposeHeaders:    []string{"Content-Length", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After"},
		AllowCredentials: true,
	}))

	// Initialize rate limiters if enabled
	if getRateLimitEnabled() {
		limiters := initRateLimiters()
		r.Use(RateLimitMiddleware(limiters))
		log.Println("Rate limiting enabled")
	}

	r.GET("/healthz", handleHealth)
	r.POST("/api/ai/summarize", handleSummarize)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Go Gateway running on port %s", port)
	r.Run(":" + port)
}

// - 500: Verifier or AI service failure (includes error details)
func handleSummarize(c *gin.Context) {
	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")

	// 1. Payment Required
	if signature == "" || nonce == "" {
		context := createPaymentContext()
		c.JSON(402, gin.H{
			"error":          "Payment Required",
			"message":        "Please sign the payment context",
			"paymentContext": context,
		})
		return
	}

	// 2. Verify Payment (Call Rust Service)
	context := PaymentContext{
		Recipient: getRecipientAddress(),
		Token:     "USDC",
		Amount:    getPaymentAmount(),
		Nonce:     nonce,
		ChainID:   getChainID(),
	}

	verifyReq := VerifyRequest{
		Context:   context,
		Signature: signature,
	}

	verifyBody, _ := json.Marshal(verifyReq)
	verifierURL := os.Getenv("VERIFIER_URL")
	if verifierURL == "" {
		verifierURL = "http://127.0.0.1:3002"
	}
	resp, err := http.Post(verifierURL+"/verify", "application/json", bytes.NewBuffer(verifyBody))
	if err != nil {
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

	// 3. Call AI Service
	var req SummarizeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}

	summary, err := callOpenRouter(req.Text)
	if err != nil {
		c.JSON(500, gin.H{"error": "AI Service Failed", "details": err.Error()})
		return
	}

	c.JSON(200, gin.H{"result": summary})
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
func callOpenRouter(text string) (string, error) {
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

	req, _ := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode AI response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
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
