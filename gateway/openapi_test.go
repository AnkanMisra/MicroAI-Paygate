package main

import (
	"os"
	"strings"
	"testing"
)

func TestOpenAPISpecMatchesRoutes(t *testing.T) {
	data, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	spec := string(data)

	expectedPaths := []string{
		"/healthz",
		"/api/ai/summarize",
	}

	for _, path := range expectedPaths {
		if !strings.Contains(spec, path) {
			t.Errorf("OpenAPI spec missing path: %s", path)
		}
	}
}
