package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// ginPathToOpenAPI converts a gin route pattern to its OpenAPI path equivalent.
// gin uses :name and *name for params; OpenAPI uses {name}.
func ginPathToOpenAPI(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if len(part) == 0 {
			continue
		}
		if part[0] == ':' || part[0] == '*' {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

// TestOpenAPISpecMatchesRoutes enforces bidirectional alignment between the
// API surface registered by registerAPIRoutes, plus the default metrics route,
// and the paths documented in openapi.yaml. Documentation-meta routes (/docs,
// /openapi.yaml) live in registerDocRoutes and are intentionally excluded from
// the API contract.
func TestOpenAPISpecMatchesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	t.Setenv("METRICS_PATH", "")
	registerAPIRoutes(r)
	r.GET(getMetricsPath(), func(c *gin.Context) {})

	data, err := os.ReadFile(filepath.Join(".", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}

	registered := make(map[string]bool, len(r.Routes()))
	for _, route := range r.Routes() {
		path := ginPathToOpenAPI(route.Path)
		registered[path] = true
		if _, ok := spec.Paths[path]; !ok {
			t.Errorf("route %s %s is registered but missing from openapi.yaml paths", route.Method, path)
		}
	}

	for path := range spec.Paths {
		if !registered[path] {
			t.Errorf("openapi.yaml documents path %s but no API route is registered for it", path)
		}
	}

	// Defense-in-depth: hard-require the four paths called out in issue #164.
	required := []string{"/healthz", "/readyz", "/metrics", "/api/ai/summarize", "/api/receipts/{id}"}
	for _, p := range required {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("openapi.yaml is missing required path: %s", p)
		}
	}
}
