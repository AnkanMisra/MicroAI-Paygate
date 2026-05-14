package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func respondError(c *gin.Context, code int, publicMsg string, internalErr error) {
	correlationID := responseCorrelationID(c)
	if internalErr != nil {
		log.Printf(
			"[ERROR] correlation_id=%s status=%d error=%s internal=%v",
			correlationID,
			code,
			publicMsg,
			internalErr,
		)
	}

	c.JSON(code, gin.H{
		"error":          publicMsg,
		"correlation_id": correlationID,
	})
}

func respondVerificationFailure(c *gin.Context, verifyResp *VerifyResponse) {
	if verifyResp == nil {
		respondError(c, http.StatusBadGateway, "verification_unavailable", fmt.Errorf("missing verifier response"))
		return
	}

	internalErr := fmt.Errorf("verifier rejected payment: code=%s error=%s", verifyResp.ErrorCode, verifyResp.Error)
	code, publicMsg := verifierFailureResponse(verifyResp)
	respondError(c, code, publicMsg, internalErr)
}

func verifierFailureResponse(verifyResp *VerifyResponse) (int, string) {
	switch verifyResp.ErrorCode {
	case "chain_id_mismatch":
		return http.StatusBadRequest, "chain_id_mismatch"
	case "nonce_already_used":
		return http.StatusConflict, "nonce_already_used"
	case "timestamp_expired", "timestamp_future", "timestamp_missing":
		return http.StatusBadRequest, "invalid_timestamp"
	case "invalid_signature":
		return http.StatusForbidden, "invalid_signature"
	}

	// Backward compatibility for older verifier responses without error_code.
	if strings.HasPrefix(verifyResp.Error, "E007") ||
		strings.HasPrefix(verifyResp.Error, "E008") ||
		strings.HasPrefix(verifyResp.Error, "E009") {
		return http.StatusBadRequest, "invalid_timestamp"
	}

	return http.StatusForbidden, "invalid_signature"
}

func isVerifierBusinessRejection(verifyResp *VerifyResponse) bool {
	if verifyResp == nil {
		return false
	}

	switch verifyResp.ErrorCode {
	case "chain_id_mismatch",
		"nonce_already_used",
		"timestamp_expired",
		"timestamp_future",
		"timestamp_missing",
		"invalid_signature":
		return true
	default:
		return false
	}
}

func responseCorrelationID(c *gin.Context) string {
	if value, exists := c.Get("correlation_id"); exists {
		if correlationID, ok := value.(string); ok && correlationID != "" {
			return safeCorrelationID(correlationID)
		}
	}

	if c.Request != nil {
		if correlationID, ok := c.Request.Context().Value(CorrelationIDKey).(string); ok && correlationID != "" {
			return safeCorrelationID(correlationID)
		}
		if correlationID := c.GetHeader("X-Correlation-ID"); correlationID != "" {
			return safeCorrelationID(correlationID)
		}
	}

	return "unknown"
}
