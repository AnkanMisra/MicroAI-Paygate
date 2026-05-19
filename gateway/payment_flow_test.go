package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newPaymentFlowTestContext(req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	if correlationID := req.Header.Get("X-Correlation-ID"); correlationID != "" {
		c.Set("correlation_id", correlationID)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), CorrelationIDKey, correlationID))
	}
	return c, recorder
}

func TestVerifyPaidRequestWritesPaymentChallengeForMissingHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/ai/summarize", strings.NewReader(`{"text":"hello"}`))
	c, recorder := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c)

	require.False(t, ok)
	require.Nil(t, verified)
	require.Equal(t, http.StatusPaymentRequired, recorder.Code)

	var response struct {
		Error          string         `json:"error"`
		Message        string         `json:"message"`
		PaymentContext PaymentContext `json:"paymentContext"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "Payment Required", response.Error)
	require.Equal(t, "Please sign the payment context", response.Message)
	require.NotEmpty(t, response.PaymentContext.Recipient)
	require.NotEmpty(t, response.PaymentContext.Token)
	require.NotEmpty(t, response.PaymentContext.Amount)
	require.NotEmpty(t, response.PaymentContext.Nonce)
	require.Positive(t, response.PaymentContext.ChainID)
	require.Positive(t, response.PaymentContext.Timestamp)
}

func TestVerifyPaidRequestReturnsVerifiedPayment(t *testing.T) {
	withVerifierResponse(t, http.StatusOK, `{"is_valid":true,"recovered_address":"0xabc","error":""}`)
	req := signedSummarizeRequest(`{"text":"hello"}`)
	c, recorder := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c)

	require.True(t, ok)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "0xabc", verified.RecoveredAddress)
	require.Equal(t, "nonce-1", verified.PaymentContext.Nonce)
	require.Equal(t, uint64(1700000000), verified.PaymentContext.Timestamp)
}

func TestVerifyPaidRequestMapsVerifierTimeout(t *testing.T) {
	withSlowVerifier(t)

	req := signedSummarizeRequest(`{"text":"hello"}`)
	c, recorder := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c)

	require.False(t, ok)
	require.Nil(t, verified)
	require.Equal(t, http.StatusGatewayTimeout, recorder.Code)

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "verifier_timeout", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestVerifyPaidRequestRequiresRecoveredAddress(t *testing.T) {
	withVerifierResponse(t, http.StatusOK, `{"is_valid":true,"recovered_address":"","error":""}`)
	req := signedSummarizeRequest(`{"text":"hello"}`)
	c, recorder := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c)

	require.False(t, ok)
	require.Nil(t, verified)
	require.Equal(t, http.StatusBadGateway, recorder.Code)

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "verification_unavailable", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}
