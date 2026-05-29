package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func getMetricsEnabled() bool {
	return strings.ToLower(strings.TrimSpace(os.Getenv("METRICS_ENABLED"))) != "false"
}

func getMetricsPath() string {
	path := strings.TrimSpace(os.Getenv("METRICS_PATH"))
	if path == "" {
		return "/metrics"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func validateMetricsPath(path string) error {
	if strings.ContainsAny(path, ":*") {
		return fmt.Errorf("metrics path %q must be a literal route without Gin wildcards", path)
	}
	switch path {
	case "/healthz", "/readyz", "/docs", "/openapi.yaml", "/api/ai/summarize", "/api/receipts/:id":
		return fmt.Errorf("metrics path %q conflicts with an existing route", path)
	}
	if strings.HasPrefix(path, "/api/receipts/") {
		return fmt.Errorf("metrics path %q conflicts with the receipt lookup route", path)
	}
	return nil
}

var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	requestsDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	cacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_cache_hits_total",
			Help: "Total cache hits",
		},
		[]string{"path"},
	)
	cacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_cache_misses_total",
			Help: "Total cache misses",
		},
		[]string{"path"},
	)
	rateLimitHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_hits_total",
			Help: "Rate limit rejections",
		},
		[]string{"path"},
	)
	verificationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_verification_total",
			Help: "Signature verification results",
		},
		[]string{"result"},
	)
	activeRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_active_requests",
			Help: "Current in-flight requests",
		},
	)
)
