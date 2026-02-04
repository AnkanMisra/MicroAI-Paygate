package main

import (
	"fmt"
	"net/http"
	"time"
)

func doRequestWithRetry(req *http.Request) (*http.Response, error) {
	backoff := 200 * time.Millisecond
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {

		// ------------------------------------------------
		// Reset request body (http.Client consumes it once)
		// ------------------------------------------------
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}

		resp, err := http.DefaultClient.Do(req)

		// ------------------------------------------------
		// SUCCESS CASES
		// ------------------------------------------------
		if err == nil {

			// 4xx → client error → DO NOT RETRY
			if resp.StatusCode < 500 {
				return resp, nil
			}

			// 5xx → retry → close body first to avoid leak
			resp.Body.Close()
		}

		// ------------------------------------------------
		// LAST ATTEMPT → exit
		// ------------------------------------------------
		if i == maxRetries-1 {
			break
		}

		// ------------------------------------------------
		// Context-aware backoff sleep
		// Stops immediately if request is cancelled/timeout
		// ------------------------------------------------
		select {
		case <-time.After(backoff):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}

		backoff *= 2 // exponential backoff
	}

	return nil, fmt.Errorf("retry failed after %d attempts", maxRetries)
}
