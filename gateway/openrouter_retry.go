package main

import (
	"fmt"
	"net/http"
	"time"
)

func doRequestWithRetry(req *http.Request) (*http.Response, error) {

	backoff := 200 * time.Millisecond

	for i := 0; i < 3; i++ {

		resp, err := http.DefaultClient.Do(req)

		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		time.Sleep(backoff)
		backoff *= 2
	}

	return nil, fmt.Errorf("retry failed")
}
