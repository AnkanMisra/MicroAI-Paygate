package main

import (
	"github.com/gin-gonic/gin"
)

// swaggerUIPage is the HTML served at GET /docs.
const swaggerUIPage = `
<!DOCTYPE html>
<html>
<head>
  <title>MicroAI Paygate Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: '/openapi.yaml',
      dom_id: '#swagger-ui'
    });
  </script>
</body>
</html>
`

// registerDocRoutes wires the documentation-meta routes. These serve the raw
// OpenAPI spec and the Swagger UI; they are deliberately NOT part of the
// public API contract documented in openapi.yaml, which is why the OpenAPI
// coverage test only inspects registerAPIRoutes.
func registerDocRoutes(r *gin.Engine) {
	r.StaticFile("/openapi.yaml", "openapi.yaml")
	r.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(200, swaggerUIPage)
	})
}

// registerAPIRoutes wires every public API route that must be documented in
// gateway/openapi.yaml. openapi_test.go iterates this surface and fails if
// any registered route is missing from the spec, or vice versa.
func registerAPIRoutes(r *gin.Engine) {
	r.GET("/healthz", handleHealthz)
	r.GET("/readyz", handleReadyz)

	aiGroup := r.Group("/api/ai")
	aiGroup.Use(RequestTimeoutMiddleware(getAITimeout()))
	if getCacheEnabled() {
		aiGroup.POST("/summarize", CacheMiddleware(), handleSummarize)
	} else {
		aiGroup.POST("/summarize", handleSummarize)
	}

	r.GET("/api/receipts/:id", handleGetReceipt)
}
