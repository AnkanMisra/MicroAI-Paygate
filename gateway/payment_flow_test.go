package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	t.Setenv("PAYGATE_AUDIENCE", "https://gateway.example.com")
	body := []byte(`{"text":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/summarize?mode=brief&tag=a&tag=b", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	c, recorder := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c, body)

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
	require.Equal(t, 2, response.PaymentContext.AuthorizationVersion)
	require.Equal(t, "https://gateway.example.com", response.PaymentContext.Audience)
	require.Equal(t, http.MethodPost, response.PaymentContext.Method)
	require.Equal(t, "/api/ai/summarize?mode=brief&tag=a&tag=b", response.PaymentContext.Resource)
	require.Equal(t, "application/json", response.PaymentContext.ContentType)
	require.Equal(t, fmt.Sprintf("0x%x", sha256.Sum256(body)), response.PaymentContext.RequestHash)
}

func TestPaymentChallengeUsesConfiguredAudienceNotForwardedHost(t *testing.T) {
	t.Setenv("PAYGATE_AUDIENCE", "https://gateway.example.com")
	body := []byte(`{"text":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "http://internal:3000/api/ai/summarize", strings.NewReader(string(body)))
	req.Host = "attacker.example"
	req.Header.Set("Forwarded", "host=attacker.example;proto=https")
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	req.Header.Set("Content-Type", "application/json")
	c, recorder := newPaymentFlowTestContext(req)

	_, ok := verifyPaidRequest(c, body)

	require.False(t, ok)
	var response struct {
		PaymentContext PaymentContext `json:"paymentContext"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "https://gateway.example.com", response.PaymentContext.Audience)
}

func TestHandleSummarizeRejectsOversizedBodyBeforeVerification(t *testing.T) {
	var verifierCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifierCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Setenv("VERIFIER_URL", server.URL)

	router := newSummarizeTestRouter()
	request := signedSummarizeRequest(strings.Repeat("x", 10*1024*1024+1))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Zero(t, verifierCalls.Load())
}

func TestVerifyPaidRequestReturnsVerifiedPayment(t *testing.T) {
	withVerifierResponse(t, http.StatusOK, `{"is_valid":true,"recovered_address":"0x14791697260e4c9a71f18484c9f997b308e59325","error":""}`)
	req := signedSummarizeRequest(`{"text":"hello"}`)
	c, recorder := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c, []byte(`{"text":"hello"}`))

	require.True(t, ok)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "0x14791697260e4c9a71f18484c9f997b308e59325", verified.RecoveredAddress)
	require.Equal(t, "nonce-1", verified.PaymentContext.Nonce)
	require.Equal(t, uint64(1700000000), verified.PaymentContext.Timestamp)
}

func TestVerifyPaidRequestSendsServerReconstructedV2Context(t *testing.T) {
	t.Setenv("PAYGATE_AUDIENCE", "https://gateway.example.com")
	requests := make(chan VerifyRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request VerifyRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests <- request
		_, _ = w.Write([]byte(`{"is_valid":true,"recovered_address":"0x14791697260e4c9a71f18484c9f997b308e59325","error":""}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("VERIFIER_URL", server.URL)

	body := []byte(`{"text":"hello"}`)
	req := signedSummarizeRequest(string(body))
	req.URL.RawQuery = "mode=brief&tag=a&tag=b"
	req.Host = "attacker.example"
	c, _ := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c, body)

	require.True(t, ok)
	require.Equal(t, "0x14791697260e4c9a71f18484c9f997b308e59325", verified.RecoveredAddress)
	verifierRequest := <-requests
	require.Equal(t, "0x14791697260E4c9A71f18484C9f997B308e59325", verifierRequest.Payer)
	require.Equal(t, 2, verifierRequest.Context.AuthorizationVersion)
	require.Equal(t, "https://gateway.example.com", verifierRequest.Context.Audience)
	require.Equal(t, http.MethodPost, verifierRequest.Context.Method)
	require.Equal(t, "/api/ai/summarize?mode=brief&tag=a&tag=b", verifierRequest.Context.Resource)
	require.Equal(t, "application/json", verifierRequest.Context.ContentType)
	require.Equal(t, fmt.Sprintf("0x%x", sha256.Sum256(body)), verifierRequest.Context.RequestHash)
}

func TestVerifyPaidRequestRejectsMissingOrInvalidPayerBeforeCallingVerifier(t *testing.T) {
	var verifierCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifierCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Setenv("VERIFIER_URL", server.URL)

	for _, payer := range []string{"", "not-an-address"} {
		req := signedSummarizeRequest(`{"text":"hello"}`)
		req.Header.Set("X-402-Payer", payer)
		c, recorder := newPaymentFlowTestContext(req)

		verified, ok := verifyPaidRequest(c, []byte(`{"text":"hello"}`))

		require.False(t, ok)
		require.Nil(t, verified)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
	}
	require.Zero(t, verifierCalls.Load())
}

func TestVerifyPaidRequestMapsVerifierTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond)
	}))
	t.Cleanup(server.Close)
	t.Setenv("VERIFIER_URL", server.URL)
	t.Setenv("VERIFIER_TIMEOUT_SECONDS", "1")

	req := signedSummarizeRequest(`{"text":"hello"}`)
	c, recorder := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c, []byte(`{"text":"hello"}`))

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

	verified, ok := verifyPaidRequest(c, []byte(`{"text":"hello"}`))

	require.False(t, ok)
	require.Nil(t, verified)
	require.Equal(t, http.StatusBadGateway, recorder.Code)

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "verification_unavailable", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestVerifyPaidRequestRequiresRecoveredAddressToMatchClaimedPayer(t *testing.T) {
	withVerifierResponse(t, http.StatusOK, `{"is_valid":true,"recovered_address":"0x0000000000000000000000000000000000000001","error":""}`)
	req := signedSummarizeRequest(`{"text":"hello"}`)
	c, recorder := newPaymentFlowTestContext(req)

	verified, ok := verifyPaidRequest(c, []byte(`{"text":"hello"}`))

	require.False(t, ok)
	require.Nil(t, verified)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "verification_unavailable", response["error"])
}
