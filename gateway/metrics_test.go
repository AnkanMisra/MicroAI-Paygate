package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGetMetricsEnabledDefaultsAndFalseVariants(t *testing.T) {
	t.Setenv("METRICS_ENABLED", "")
	require.True(t, getMetricsEnabled())

	t.Setenv("METRICS_ENABLED", "false")
	require.False(t, getMetricsEnabled())

	t.Setenv("METRICS_ENABLED", "False")
	require.False(t, getMetricsEnabled())

	t.Setenv("METRICS_ENABLED", "FALSE")
	require.False(t, getMetricsEnabled())
}

func TestGetMetricsPathDefaultsAndNormalizesSlash(t *testing.T) {
	t.Setenv("METRICS_PATH", "")
	require.Equal(t, "/metrics", getMetricsPath())

	t.Setenv("METRICS_PATH", "internal/metrics")
	require.Equal(t, "/internal/metrics", getMetricsPath())

	t.Setenv("METRICS_PATH", "/custom-metrics")
	require.Equal(t, "/custom-metrics", getMetricsPath())
}

func TestMetricsPathRejectsReservedRoutes(t *testing.T) {
	require.NoError(t, validateMetricsPath("/metrics"))
	require.NoError(t, validateMetricsPath("/internal/metrics"))

	for _, path := range []string{
		"/healthz",
		"/readyz",
		"/docs",
		"/openapi.yaml",
		"/api/ai/summarize",
		"/api/receipts/:id",
		"/api/receipts/metrics",
		"/api/receipts/metrics/nested",
		"/:name",
		"/api/:name",
		"/*path",
		"/metrics/*rest",
	} {
		t.Run(path, func(t *testing.T) {
			require.Error(t, validateMetricsPath(path))
		})
	}
}

func TestMetricsMiddlewareRecordsTimeoutStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(MetricsMiddleware())
	r.Use(RequestTimeoutMiddleware(10 * time.Millisecond))
	r.GET("/metrics-timeout-status", func(c *gin.Context) {
		time.Sleep(50 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	before := testutil.ToFloat64(requestsTotal.WithLabelValues("GET", "/metrics-timeout-status", "504"))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics-timeout-status", nil)
	r.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusGatewayTimeout, recorder.Code)
	after := testutil.ToFloat64(requestsTotal.WithLabelValues("GET", "/metrics-timeout-status", "504"))
	require.Equal(t, float64(1), after-before)
}

func TestMetricsMiddlewareRecordsPanickedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(MetricsMiddleware())
	r.GET("/panic-metrics", func(c *gin.Context) {
		panic("boom")
	})

	before := testutil.ToFloat64(requestsTotal.WithLabelValues("GET", "/panic-metrics", "500"))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic-metrics", nil)
	r.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	after := testutil.ToFloat64(requestsTotal.WithLabelValues("GET", "/panic-metrics", "500"))
	require.Equal(t, float64(1), after-before)
}

func TestRateLimitMiddlewareRecordsRejectedRequests(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	t.Setenv("RATE_LIMIT_ANONYMOUS_RPM", "60")
	t.Setenv("RATE_LIMIT_ANONYMOUS_BURST", "1")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimitMiddleware(initRateLimiters()))
	r.GET("/rate-limit-metrics", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	before := testutil.ToFloat64(rateLimitHits.WithLabelValues("/rate-limit-metrics"))

	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/rate-limit-metrics", nil)
		r.ServeHTTP(recorder, req)
	}

	after := testutil.ToFloat64(rateLimitHits.WithLabelValues("/rate-limit-metrics"))
	require.Equal(t, float64(1), after-before)
}

func TestRateLimitMiddlewareRecordsUnknownRouteLabel(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	t.Setenv("RATE_LIMIT_ANONYMOUS_RPM", "60")
	t.Setenv("RATE_LIMIT_ANONYMOUS_BURST", "1")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimitMiddleware(initRateLimiters()))

	before := testutil.ToFloat64(rateLimitHits.WithLabelValues("unknown"))

	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/not-registered", nil)
		r.ServeHTTP(recorder, req)
	}

	after := testutil.ToFloat64(rateLimitHits.WithLabelValues("unknown"))
	require.Equal(t, float64(1), after-before)
}

func TestGatewayVerificationMetricRecordsInvalidResponses(t *testing.T) {
	withVerifierResponse(t, http.StatusOK, `{"is_valid":false,"recovered_address":null,"error":"bad signature"}`)
	router := newSummarizeTestRouter()

	before := testutil.ToFloat64(verificationTotal.WithLabelValues("invalid"))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	after := testutil.ToFloat64(verificationTotal.WithLabelValues("invalid"))
	require.Equal(t, float64(1), after-before)
}

func TestGatewayVerificationMetricRecordsMalformedSuccessResponses(t *testing.T) {
	withVerifierResponse(t, http.StatusOK, `{"is_valid":true,"recovered_address":"","error":""}`)
	router := newSummarizeTestRouter()

	before := testutil.ToFloat64(verificationTotal.WithLabelValues("error"))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"hello"}`))

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	after := testutil.ToFloat64(verificationTotal.WithLabelValues("error"))
	require.Equal(t, float64(1), after-before)
}

func TestCacheMiddlewareRecordsHitsAndMisses(t *testing.T) {
	origClient := redisClient
	redisServer := miniredis.RunT(t)
	redisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		if redisClient != nil && redisClient != origClient {
			_ = redisClient.Close()
		}
		redisClient = origClient
	})

	withVerifierResponse(t, http.StatusOK, `{"is_valid":false,"recovered_address":null,"error":"bad signature"}`)
	router := newCachedSummarizeTestRouter()

	missBefore := testutil.ToFloat64(cacheMisses.WithLabelValues("/api/ai/summarize"))
	missRecorder := httptest.NewRecorder()
	router.ServeHTTP(missRecorder, signedSummarizeRequest(`{"text":"cache miss metrics"}`))
	require.Equal(t, http.StatusForbidden, missRecorder.Code)
	missAfter := testutil.ToFloat64(cacheMisses.WithLabelValues("/api/ai/summarize"))
	require.Equal(t, float64(1), missAfter-missBefore)

	hitText := "cache hit metrics"
	cachedBody := `{"result":"cached summary","cached_at":1700000000}`
	cacheKey := getCacheKey(hitText, "z-ai/glm-4.5-air:free")
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, cachedBody, time.Hour).Err())

	hitBefore := testutil.ToFloat64(cacheHits.WithLabelValues("/api/ai/summarize"))
	hitRecorder := httptest.NewRecorder()
	router.ServeHTTP(hitRecorder, signedSummarizeRequest(`{"text":"`+hitText+`"}`))
	require.Equal(t, http.StatusForbidden, hitRecorder.Code)
	hitAfter := testutil.ToFloat64(cacheHits.WithLabelValues("/api/ai/summarize"))
	require.Equal(t, float64(1), hitAfter-hitBefore)
}

func TestCacheHitVerificationMetricRecordsInvalidResponses(t *testing.T) {
	origClient := redisClient
	redisServer := miniredis.RunT(t)
	redisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		if redisClient != nil && redisClient != origClient {
			_ = redisClient.Close()
		}
		redisClient = origClient
	})

	withVerifierResponse(t, http.StatusOK, `{"is_valid":false,"recovered_address":null,"error":"bad signature"}`)
	router := newCachedSummarizeTestRouter()

	hitText := "cache hit invalid metrics"
	cachedBody := `{"result":"cached summary","cached_at":1700000000}`
	cacheKey := getCacheKey(hitText, "z-ai/glm-4.5-air:free")
	require.NoError(t, redisClient.Set(context.Background(), cacheKey, cachedBody, time.Hour).Err())

	before := testutil.ToFloat64(verificationTotal.WithLabelValues("invalid"))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedSummarizeRequest(`{"text":"`+hitText+`"}`))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	after := testutil.ToFloat64(verificationTotal.WithLabelValues("invalid"))
	require.Equal(t, float64(1), after-before)
}
