package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)
func TestGracefulShutdown_WaitsForInFlightRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(TrackInFlightRequests())

	// Simulate slow handler
	r.GET("/slow", func(c *gin.Context) {
		time.Sleep(200 * time.Millisecond)
		c.Status(http.StatusOK)
	})

	srv := &http.Server{
		Handler: r,
	}

	// Start test server
	ln := httptest.NewUnstartedServer(r)
	ln.Config = srv
	ln.Start()
	defer ln.Close()

	// Make request in background
	done := make(chan struct{})
	go func() {
		resp, err := http.Get(ln.URL + "/slow")
		if err != nil {
			t.Errorf("request failed: %v", err)
			return
		}
		resp.Body.Close()
		close(done)
	}()

	// Give request time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	WaitForInFlightRequests()
	elapsed := time.Since(start)

	<-done

	// Assert shutdown waited for request
	if elapsed < 200*time.Millisecond {
		t.Fatalf("shutdown did not wait for in-flight request")
	}
}
