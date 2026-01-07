package main

import (
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

var (
	activeRequestsWG sync.WaitGroup
	activeRequestCnt int64
)

// TrackInFlightRequests tracks active HTTP requests.
func TrackInFlightRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		activeRequestsWG.Add(1)
		atomic.AddInt64(&activeRequestCnt, 1)

		defer func() {
			atomic.AddInt64(&activeRequestCnt, -1)
			activeRequestsWG.Done()
		}()

		c.Next()
	}
}

// WaitForInFlightRequests blocks until all active requests finish.
func WaitForInFlightRequests() {
	activeRequestsWG.Wait()
}

// GetActiveRequestCount returns the current number of active requests.
func GetActiveRequestCount() int64 {
	return atomic.LoadInt64(&activeRequestCnt)
}
