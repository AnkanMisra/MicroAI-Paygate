package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	// Match an optional 0x prefix so real private keys (which are usually
	// 0x-prefixed) are caught — a leading \b fails between "x" and the first
	// hex digit because both are word characters. The trailing \b avoids
	// chopping into longer hex runs.
	hex64Regex = regexp.MustCompile(`(?:0x)?[0-9a-fA-F]{64}\b`)
	// Allow dashes so versioned keys like sk-or-v1-<secret> are redacted in
	// full, not just the "sk-or-v1" prefix.
	openRouterKeyRegex = regexp.MustCompile(`sk-or-[a-zA-Z0-9-]+`)
)

func sanitizeErrorString(s string) string {
	// Redact API keys before hex so a key whose secret happens to contain a
	// 64-hex run collapses to a single [redacted_api_key] token.
	s = openRouterKeyRegex.ReplaceAllString(s, "[redacted_api_key]")
	s = hex64Regex.ReplaceAllString(s, "[redacted_hex_64]")
	return s
}

func respondError(c *gin.Context, code int, publicMsg string, internalErr error) {
	// Only mark the payment failed if an earlier stage hasn't already recorded
	// an outcome. handleSummarize sets payment_status="success" once the
	// verifier accepts the signature; a later receipt-generation error must not
	// overwrite that and misreport a paid request as a payment failure.
	if _, ok := c.Get("payment_status"); !ok {
		c.Set("payment_status", "failed")
	}
	c.Set("payment_error", publicMsg)

	var sanitizedErr string
	if internalErr != nil {
		sanitizedErr = sanitizeErrorString(internalErr.Error())
		c.Set("internal_error", sanitizedErr)
	}

	correlationID := responseCorrelationID(c)
	if internalErr != nil && os.Getenv("LOG_FORMAT") != "json" {
		log.Printf(
			"[ERROR] correlation_id=%s status=%d error=%s internal=%s",
			correlationID,
			code,
			publicMsg,
			sanitizedErr,
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
