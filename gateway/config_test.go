package main

import (
	"strings"
	"testing"
	"time"
)

func TestValidateConfig_MissingRequiredEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "")
	t.Setenv("CACHE_ENABLED", "false")

	err := validateConfig()
	if err == nil {
		t.Fatalf("expected error when required env vars are missing, got nil")
	}

	expectedVars := []string{"OPENROUTER_API_KEY", "SERVER_WALLET_PRIVATE_KEY"}
	errStr := err.Error()
	for _, v := range expectedVars {
		if !strings.Contains(errStr, v) {
			t.Errorf("expected error to mention missing var %s, got: %v", v, err)
		}
	}
}

func TestValidateConfig_WithRequiredEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")

	err := validateConfig()
	if err != nil {
		t.Fatalf("expected no error when required env vars are set, got: %v", err)
	}
}

func TestValidateConfig_CacheEnabledRequiresRedis(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("REDIS_URL", "")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")

	err := validateConfig()
	if err == nil {
		t.Fatalf("expected error when CACHE_ENABLED=true but REDIS_URL is missing, got nil")
	}

	if !strings.Contains(err.Error(), "REDIS_URL") {
		t.Errorf("expected error to mention REDIS_URL, got: %v", err)
	}
}

func TestValidateConfig_CacheEnabledWithValidRedis(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("REDIS_URL", "localhost:6379")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")

	err := validateConfig()
	if err != nil {
		t.Fatalf("expected no error when all required vars are set, got: %v", err)
	}
}

func TestValidateServerPrivateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid 32-byte key",
			key:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantErr: false,
		},
		{
			name:    "valid 31-byte key",
			key:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd",
			wantErr: false,
		},
		{
			name:    "valid key with 0x prefix",
			key:     "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantErr: false,
		},
		{
			name:    "empty key",
			key:     "",
			wantErr: true,
			errMsg:  "SERVER_WALLET_PRIVATE_KEY not set",
		},
		{
			name:    "too short key",
			key:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
			wantErr: true,
			errMsg:  "private key too short",
		},
		{
			name:    "too long key",
			key:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00",
			wantErr: true,
			errMsg:  "private key must be at most 32 bytes",
		},
		{
			name:    "invalid hex",
			key:     "invalid-hex-string-not-valid-hex-chars-here-zzz",
			wantErr: true,
			errMsg:  "invalid private key format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SERVER_WALLET_PRIVATE_KEY", tt.key)
			err := validateServerPrivateKey()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for key %q, got nil", tt.key)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error to contain %q, got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error for key %q, got: %v", tt.key, err)
				}
			}
		})
	}
}

func TestValidateRedisURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid host:port",
			url:     "localhost:6379",
			wantErr: false,
		},
		{
			name:    "valid redis URL",
			url:     "redis://localhost:6379",
			wantErr: false,
		},
		{
			name:    "valid rediss URL",
			url:     "rediss://localhost:6379",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
			errMsg:  "REDIS_URL not set but CACHE_ENABLED=true",
		},
		{
			name:    "invalid host:port format",
			url:     "localhost",
			wantErr: true,
			errMsg:  "REDIS_URL must be in format 'host:port'",
		},
		{
			name:    "invalid host:port format - empty host",
			url:     ":6379",
			wantErr: true,
			errMsg:  "REDIS_URL must be in format 'host:port'",
		},
		{
			name:    "invalid host:port format - empty port",
			url:     "localhost:",
			wantErr: true,
			errMsg:  "REDIS_URL must be in format 'host:port'",
		},
		{
			name:    "invalid host:port format - multiple colons",
			url:     ":::",
			wantErr: true,
			errMsg:  "REDIS_URL must be in format 'host:port'",
		},
		{
			name:    "invalid redis URL",
			url:     "redis://invalid-url-format",
			wantErr: true,
			errMsg:  "invalid REDIS_URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REDIS_URL", tt.url)
			err := validateRedisURL()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for URL %q, got nil", tt.url)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error to contain %q, got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error for URL %q, got: %v", tt.url, err)
				}
			}
		})
	}
}

func TestTimeoutConfigHelpers(t *testing.T) {
	// Defaults
	if getRequestTimeout() != 60*time.Second {
		t.Fatalf("expected default request timeout 60s, got %v", getRequestTimeout())
	}
	if getAITimeout() != 30*time.Second {
		t.Fatalf("expected default AI timeout 30s, got %v", getAITimeout())
	}
	if getVerifierTimeout() != 2*time.Second {
		t.Fatalf("expected default verifier timeout 2s, got %v", getVerifierTimeout())
	}
	if getHealthCheckTimeout() != 2*time.Second {
		t.Fatalf("expected default health check timeout 2s, got %v", getHealthCheckTimeout())
	}

	// Custom values
	t.Setenv("REQUEST_TIMEOUT_SECONDS", "10")
	t.Setenv("AI_REQUEST_TIMEOUT_SECONDS", "5")
	t.Setenv("VERIFIER_TIMEOUT_SECONDS", "1")
	t.Setenv("HEALTH_CHECK_TIMEOUT_SECONDS", "3")

	if getRequestTimeout() != 10*time.Second {
		t.Fatalf("expected request timeout 10s, got %v", getRequestTimeout())
	}
	if getAITimeout() != 5*time.Second {
		t.Fatalf("expected AI timeout 5s, got %v", getAITimeout())
	}
	if getVerifierTimeout() != 1*time.Second {
		t.Fatalf("expected verifier timeout 1s, got %v", getVerifierTimeout())
	}
	if getHealthCheckTimeout() != 3*time.Second {
		t.Fatalf("expected health check timeout 3s, got %v", getHealthCheckTimeout())
	}

	// Non-positive values should fall back to defaults
	t.Setenv("REQUEST_TIMEOUT_SECONDS", "0")
	if getRequestTimeout() != 60*time.Second {
		t.Fatalf("expected request timeout to fall back to 60s on non-positive value, got %v", getRequestTimeout())
	}
}

func TestGetMaxBodySize(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected int64
	}{
		{
			name:     "default 10MB",
			envValue: "",
			expected: 10 * 1024 * 1024,
		},
		{
			name:     "custom 1MB",
			envValue: "1",
			expected: 1 * 1024 * 1024,
		},
		{
			name:     "custom 50MB",
			envValue: "50",
			expected: 50 * 1024 * 1024,
		},
		{
			name:     "custom 100MB",
			envValue: "100",
			expected: 100 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("MAX_REQUEST_BODY_MB", tt.envValue)
			}
			result := getMaxBodySize()
			if result != tt.expected {
				t.Errorf("expected %d bytes, got %d bytes", tt.expected, result)
			}
		})
	}
}
