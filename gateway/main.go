package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

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
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	r.GET("/healthz", handleHealth)
	r.POST("/api/ai/summarize", handleSummarize)

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

func handleSummarize(c *gin.Context) {
	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")

	if signature == "" || nonce == "" {
		c.Set("payment_verified", false)

		ctx := createPaymentContext()
		c.JSON(402, gin.H{
			"error":          "Payment Required",
			"paymentContext": ctx,
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

	summary, err := callOpenRouter(req.Text)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
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
	}
	return "stub summary", nil
}
