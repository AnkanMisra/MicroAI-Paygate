package main

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

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

// Receipt storage
var (
	receiptStore = make(map[string]*SignedReceipt)
	receiptMutex sync.RWMutex
	serverPrivateKey *ecdsa.PrivateKey
)

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Println("Warning: Error loading .env file")
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
		ExposeHeaders:    []string{"Content-Length", "X-402-Receipt"},
		AllowCredentials: true,
	}))

	r.GET("/healthz", handleHealth)
	r.POST("/api/ai/summarize", handleSummarize)
	r.GET("/api/receipts/:id", handleGetReceipt)

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

	// 4. Generate Receipt
	reqBody, _ := json.Marshal(req)
	respBody := []byte(summary)
	
	receipt, err := GenerateReceipt(
		context,
		verifyResp.RecoveredAddress,
		c.Request.URL.Path,
		reqBody,
		respBody,
	)
	
	if err != nil {
		log.Printf("Failed to generate receipt: %v", err)
		// Don't fail the request if receipt generation fails
	} else {
		// Store receipt
		storeReceipt(receipt)
		
		// Add receipt to headers (base64 encoded)
		receiptJSON, _ := json.Marshal(receipt)
		c.Header("X-402-Receipt", base64.StdEncoding.EncodeToString(receiptJSON))
		
		// Include receipt in response
		c.JSON(200, gin.H{
			"result":  summary,
			"receipt": receipt,
		})
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

// Receipt Management Functions

// getServerPrivateKey loads the server's private key from environment
func getServerPrivateKey() (*ecdsa.PrivateKey, error) {
	if serverPrivateKey != nil {
		return serverPrivateKey, nil
	}

	privateKeyHex := os.Getenv("SERVER_WALLET_PRIVATE_KEY")
	if privateKeyHex == "" {
		return nil, fmt.Errorf("SERVER_WALLET_PRIVATE_KEY not set in environment")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Cache for future use
	serverPrivateKey = privateKey
	return privateKey, nil
}

// storeReceipt stores a receipt in memory with TTL
func storeReceipt(receipt *SignedReceipt) {
	receiptMutex.Lock()
	defer receiptMutex.Unlock()

	receiptStore[receipt.Receipt.ID] = receipt

	// Start TTL cleanup goroutine
	go func(id string) {
		ttl := time.Duration(getReceiptTTL()) * time.Second
		time.Sleep(ttl)

		receiptMutex.Lock()
		defer receiptMutex.Unlock()
		delete(receiptStore, id)
	}(receipt.Receipt.ID)
}

// getReceipt retrieves a receipt by ID
func getReceipt(id string) (*SignedReceipt, bool) {
	receiptMutex.RLock()
	defer receiptMutex.RUnlock()

	receipt, exists := receiptStore[id]
	return receipt, exists
}

// getReceiptTTL returns the receipt TTL from environment (default 24h)
func getReceiptTTL() int {
	ttlStr := os.Getenv("RECEIPT_TTL")
	if ttlStr == "" {
		return 86400 // 24 hours
	}
	ttl, err := strconv.Atoi(ttlStr)
	if err != nil {
		return 86400
	}
	return ttl
}

// handleGetReceipt handles GET /api/receipts/:id
func handleGetReceipt(c *gin.Context) {
	receiptID := c.Param("id")

	receipt, exists := getReceipt(receiptID)
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

