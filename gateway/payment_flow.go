package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type verifiedPayment struct {
	PaymentContext   PaymentContext
	RecoveredAddress string
}

func verifyPaidRequest(c *gin.Context) (*verifiedPayment, bool) {
	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")

	if signature == "" || nonce == "" {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":          "Payment Required",
			"message":        "Please sign the payment context",
			"paymentContext": createPaymentContext(),
		})
		return nil, false
	}

	timestampValue, ok := paymentTimestamp(c)
	if !ok {
		return nil, false
	}

	verifyResp, paymentCtx, err := verifyPayment(c.Request.Context(), signature, nonce, timestampValue)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			respondError(c, http.StatusGatewayTimeout, "verifier_timeout", err)
		} else {
			respondError(c, http.StatusBadGateway, "verification_unavailable", err)
		}
		return nil, false
	}

	if verifyResp == nil {
		respondError(c, http.StatusBadGateway, "verification_unavailable", fmt.Errorf("missing verifier response"))
		return nil, false
	}
	if !verifyResp.IsValid {
		respondVerificationFailure(c, verifyResp)
		return nil, false
	}
	if verifyResp.RecoveredAddress == "" {
		respondError(c, http.StatusBadGateway, "verification_unavailable", fmt.Errorf("verifier success missing recovered_address"))
		return nil, false
	}

	return &verifiedPayment{
		PaymentContext:   *paymentCtx,
		RecoveredAddress: verifyResp.RecoveredAddress,
	}, true
}

func paymentTimestamp(c *gin.Context) (uint64, bool) {
	timestampHeader := c.GetHeader("X-402-Timestamp")
	if timestampHeader == "" {
		respondError(c, http.StatusBadRequest, "invalid_timestamp", fmt.Errorf("missing X-402-Timestamp header"))
		return 0, false
	}

	timestampValue, err := strconv.ParseUint(timestampHeader, 10, 64)
	if err != nil || timestampValue == 0 {
		respondError(c, http.StatusBadRequest, "invalid_timestamp", fmt.Errorf("invalid X-402-Timestamp header"))
		return 0, false
	}

	return timestampValue, true
}

func sendPaidResult(c *gin.Context, payment *verifiedPayment, requestBody []byte, result string) error {
	if payment == nil {
		err := fmt.Errorf("missing verified payment")
		respondError(c, http.StatusInternalServerError, "receipt_generation_failed", err)
		return err
	}

	return generateAndSendReceipt(c, payment.PaymentContext, payment.RecoveredAddress, requestBody, result)
}
