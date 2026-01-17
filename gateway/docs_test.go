package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDocs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/docs", func(c *gin.Context) {
		data := struct{ Version string }{Version: "5.11.0"}
		if err := swaggerTmpl.Execute(c.Writer, data); err != nil {
			t.Fatal(err)
		}
	})

	req, _ := http.NewRequest("GET", "/docs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "swagger-ui") {
		t.Fatal("swagger ui not rendered")
	}
}
