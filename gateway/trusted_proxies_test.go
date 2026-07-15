package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetTrustedProxies(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "unset", want: nil},
		{name: "empty", value: "", want: nil},
		{name: "whitespace only", value: "  \t", want: nil},
		{
			name:  "trims entries and preserves order",
			value: " 10.0.0.1, 2001:db8::/32, , 192.0.2.0/24 ",
			want:  []string{"10.0.0.1", "2001:db8::/32", "192.0.2.0/24"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXIES", tt.value)
			require.Equal(t, tt.want, getTrustedProxies())
		})
	}
}

func TestTrustedProxiesGinConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	valid := []string{"10.0.0.1", "192.0.2.0/24", "2001:db8::/32"}
	r := gin.New()
	require.NoError(t, r.SetTrustedProxies(valid))

	invalid := []string{"not-an-ip-or-cidr"}
	require.Error(t, r.SetTrustedProxies(invalid))
	require.NoError(t, r.SetTrustedProxies(nil))
}

func TestTrustedProxiesClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		trusted      []string
		remoteAddr   string
		forwardedFor string
		want         string
	}{
		{
			name:         "untrusted peer cannot spoof forwarded address",
			trusted:      []string{"10.0.0.1"},
			remoteAddr:   "192.0.2.10:1234",
			forwardedFor: "198.51.100.25",
			want:         "192.0.2.10",
		},
		{
			name:         "trusted peer may forward client address",
			trusted:      []string{"10.0.0.1"},
			remoteAddr:   "10.0.0.1:1234",
			forwardedFor: "198.51.100.25",
			want:         "198.51.100.25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			require.NoError(t, r.SetTrustedProxies(tt.trusted))
			r.GET("/client-ip", func(c *gin.Context) {
				c.String(http.StatusOK, c.ClientIP())
			})

			req := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
			req.RemoteAddr = tt.remoteAddr
			req.Header.Set("X-Forwarded-For", tt.forwardedFor)
			response := httptest.NewRecorder()

			r.ServeHTTP(response, req)

			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, tt.want, response.Body.String())
		})
	}
}
