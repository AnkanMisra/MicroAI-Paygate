package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const paymentAuthorizationVersion = 2

type paymentRequestBinding struct {
	Audience    string
	Method      string
	Resource    string
	ContentType string
	RequestHash string
}

func buildPaymentRequestBinding(request *http.Request, body []byte) paymentRequestBinding {
	resource := request.URL.EscapedPath()
	if resource == "" {
		resource = "/"
	}
	if request.URL.RawQuery != "" {
		resource += "?" + request.URL.RawQuery
	}

	contentType := strings.TrimSpace(request.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}

	return paymentRequestBinding{
		Audience:    getPaygateAudience(),
		Method:      strings.ToUpper(request.Method),
		Resource:    resource,
		ContentType: contentType,
		RequestHash: fmt.Sprintf("0x%x", sha256.Sum256(body)),
	}
}

func getPaygateAudience() string {
	audience := strings.TrimSpace(os.Getenv("PAYGATE_AUDIENCE"))
	if audience == "" {
		return "http://localhost:3000"
	}
	return audience
}
