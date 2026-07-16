package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
)

type verifiedPayment struct {
	PaymentContext   PaymentContext
	RecoveredAddress string
}

func hasPaymentHeaders(c *gin.Context) bool {
	return c.GetHeader("X-402-Signature") != "" && c.GetHeader("X-402-Nonce") != ""
}

func writePaymentChallenge(c *gin.Context, requestBody []byte) {
	paymentAuthorizationTotal.WithLabelValues("2", "challenge").Inc()
	c.JSON(http.StatusPaymentRequired, gin.H{
		"error":          "Payment Required",
		"message":        "Please sign the payment context",
		"paymentContext": createPaymentChallengeContext(c.Request, requestBody),
	})
}

func verifyPaidRequest(c *gin.Context, requestBody []byte) (*verifiedPayment, bool) {
	signature := c.GetHeader("X-402-Signature")
	nonce := c.GetHeader("X-402-Nonce")

	if !hasPaymentHeaders(c) {
		writePaymentChallenge(c, requestBody)
		return nil, false
	}

	timestampValue, ok := paymentTimestamp(c)
	if !ok {
		return nil, false
	}

	payer := c.GetHeader("X-402-Payer")
	if !common.IsHexAddress(payer) {
		paymentAuthorizationTotal.WithLabelValues("2", "invalid_context").Inc()
		respondError(c, http.StatusBadRequest, "invalid_authorization_context", fmt.Errorf("missing or invalid X-402-Payer header"))
		return nil, false
	}

	paymentCtx := createPaymentContext(
		buildPaymentRequestBinding(c.Request, requestBody),
		nonce,
		timestampValue,
	)
	verifyResp, err := verifyPayment(c.Request.Context(), signature, payer, paymentCtx)
	if err != nil {
		verificationTotal.WithLabelValues("error").Inc()
		paymentAuthorizationTotal.WithLabelValues("2", "verification_error").Inc()

		ctxErr := c.Request.Context().Err()
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
			ctxErr == context.DeadlineExceeded || ctxErr == context.Canceled {
			respondError(c, http.StatusGatewayTimeout, "verifier_timeout", err)
		} else {
			respondError(c, http.StatusBadGateway, "verification_unavailable", err)
		}
		return nil, false
	}

	if !verifyResp.IsValid {
		verificationTotal.WithLabelValues("invalid").Inc()
		paymentAuthorizationTotal.WithLabelValues("2", paymentAuthorizationOutcome(verifyResp.ErrorCode)).Inc()
		respondVerificationFailure(c, verifyResp)
		return nil, false
	}
	if verifyResp.RecoveredAddress == "" {
		verificationTotal.WithLabelValues("error").Inc()
		paymentAuthorizationTotal.WithLabelValues("2", "verification_error").Inc()
		respondError(c, http.StatusBadGateway, "verification_unavailable", fmt.Errorf("verifier success missing recovered_address"))
		return nil, false
	}
	if !common.IsHexAddress(verifyResp.RecoveredAddress) ||
		common.HexToAddress(verifyResp.RecoveredAddress) != common.HexToAddress(payer) {
		verificationTotal.WithLabelValues("error").Inc()
		paymentAuthorizationTotal.WithLabelValues("2", "verification_error").Inc()
		respondError(c, http.StatusBadGateway, "verification_unavailable", fmt.Errorf("verifier success recovered address does not match claimed payer"))
		return nil, false
	}

	verificationTotal.WithLabelValues("success").Inc()
	paymentAuthorizationTotal.WithLabelValues("2", "success").Inc()

	return &verifiedPayment{
		PaymentContext:   paymentCtx,
		RecoveredAddress: verifyResp.RecoveredAddress,
	}, true
}

func paymentAuthorizationOutcome(errorCode string) string {
	switch errorCode {
	case "invalid_authorization_context":
		return "invalid_context"
	case "authorization_version_too_old":
		return "downgrade"
	case "signer_mismatch":
		return "signer_mismatch"
	case "nonce_already_used":
		return "replay"
	default:
		return "rejected"
	}
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
