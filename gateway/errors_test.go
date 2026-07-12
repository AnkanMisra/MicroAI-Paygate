package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type failingProvider struct {
	err error
}

func (p failingProvider) Generate(context.Context, string) (string, error) {
	return "", p.err
}

func newSummarizeTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationIDMiddleware())
	r.POST("/api/ai/summarize", handleSummarize)
	return r
}

func newCachedSummarizeTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationIDMiddleware())
	r.POST("/api/ai/summarize", CacheMiddleware(), handleSummarize)
	return r
}

func signedSummarizeRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/ai/summarize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-402-Signature", "0xsigned")
	req.Header.Set("X-402-Nonce", "nonce-1")
	req.Header.Set("X-402-Timestamp", "1700000000")
	req.Header.Set("X-Correlation-ID", "test-correlation-id")
	return req
}

func withVerifierResponse(t *testing.T, status int, body string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	t.Setenv("VERIFIER_URL", server.URL)
}

func withCachedSummary(t *testing.T, text string) {
	t.Helper()

	origClient := redisClient
	redisServer := miniredis.RunT(t)
	redisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		if redisClient != nil && redisClient != origClient {
			_ = redisClient.Close()
		}
		redisClient = origClient
	})

	cachedBody, err := json.Marshal(CachedResponse{
		Result:   "cached summary",
		CachedAt: time.Now().Unix(),
	})
	require.NoError(t, err)

	cacheKey := getCacheKey(text, "z-ai/glm-4.5-air:free")
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, cachedBody, time.Hour).Err())
}

func TestHandleSummarizeSanitizesOpenRouterProviderError(t *testing.T) {
	origProvider := aiProvider
	t.Cleanup(func() { aiProvider = origProvider })
	aiProvider = failingProvider{
		err: errors.New("openrouter returned status 429: SENSITIVE_PROVIDER_DETAIL"),
	}
	withVerifierResponse(t, http.StatusOK, `{"is_valid":true,"recovered_address":"0xabc","error":""}`)

	router := newSummarizeTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "SENSITIVE_PROVIDER_DETAIL")

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "upstream_unavailable", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestHandleSummarizeSanitizesVerifierInvalidSignatureDetail(t *testing.T) {
	withVerifierResponse(t, http.StatusOK, `{"is_valid":false,"recovered_address":null,"error":"bad signature: SENSITIVE_VERIFIER_DETAIL"}`)

	router := newSummarizeTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "SENSITIVE_VERIFIER_DETAIL")

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "invalid_signature", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestHandleSummarizeSanitizesVerifierTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond)
	}))
	t.Cleanup(server.Close)
	t.Setenv("VERIFIER_URL", server.URL)
	t.Setenv("VERIFIER_TIMEOUT_SECONDS", "1")

	router := newSummarizeTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusGatewayTimeout, recorder.Code)

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "verifier_timeout", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestHandleSummarizeMapsVerifierNonceReplay(t *testing.T) {
	withVerifierResponse(t, http.StatusConflict, `{"is_valid":false,"recovered_address":null,"error":"SENSITIVE_REPLAY_DETAIL","error_code":"nonce_already_used"}`)

	router := newSummarizeTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "SENSITIVE_REPLAY_DETAIL")

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "nonce_already_used", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestHandleSummarizeMapsVerifierChainMismatch(t *testing.T) {
	withVerifierResponse(t, http.StatusBadRequest, `{"is_valid":false,"recovered_address":null,"error":"SENSITIVE_CHAIN_DETAIL","error_code":"chain_id_mismatch"}`)

	router := newSummarizeTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "SENSITIVE_CHAIN_DETAIL")

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "chain_id_mismatch", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestHandleSummarizeMapsVerifierInvalidSignatureParse(t *testing.T) {
	withVerifierResponse(t, http.StatusBadRequest, `{"is_valid":false,"recovered_address":null,"error":"bad signature: SENSITIVE_PARSE_DETAIL","error_code":"invalid_signature"}`)

	router := newSummarizeTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "SENSITIVE_PARSE_DETAIL")

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "invalid_signature", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestCacheHitMapsVerifierNonceReplay(t *testing.T) {
	text := "cached replay text"
	withCachedSummary(t, text)
	withVerifierResponse(t, http.StatusConflict, `{"is_valid":false,"recovered_address":null,"error":"SENSITIVE_CACHE_REPLAY_DETAIL","error_code":"nonce_already_used"}`)

	router := newCachedSummarizeTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"`+text+`"}`))

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "SENSITIVE_CACHE_REPLAY_DETAIL")

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "nonce_already_used", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}

func TestHandleSummarizePreservesAIProviderTimeoutStatus(t *testing.T) {
	origProvider := aiProvider
	t.Cleanup(func() { aiProvider = origProvider })
	aiProvider = failingProvider{err: context.DeadlineExceeded}
	withVerifierResponse(t, http.StatusOK, `{"is_valid":true,"recovered_address":"0xabc","error":""}`)

	router := newSummarizeTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusGatewayTimeout, recorder.Code)

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "upstream_timeout", response["error"])
	require.Equal(t, "test-correlation-id", response["correlation_id"])
}
