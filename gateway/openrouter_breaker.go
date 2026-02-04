package main

import (
	"time"

	"github.com/sony/gobreaker"
)

var openRouterCB *gobreaker.CircuitBreaker

func init() {
	openRouterCB = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "OpenRouter",
		MaxRequests: 5,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= 3
		},
	})
}
