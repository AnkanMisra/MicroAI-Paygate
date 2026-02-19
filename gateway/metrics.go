package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

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
			Name: "gateway_request_duration_seconds",
			Help: "Request duration in seconds",
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