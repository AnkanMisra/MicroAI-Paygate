package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateConfig_MissingRequiredEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "")
	t.Setenv("VERIFIER_URL", "")
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("RECEIPT_STORE", "memory")

	err := validateConfig()
	if err == nil {
		t.Fatalf("expected error when required env vars are missing, got nil")
	}

	expectedVars := []string{"OPENROUTER_API_KEY", "SERVER_WALLET_PRIVATE_KEY", "VERIFIER_URL"}
	errStr := err.Error()
	for _, v := range expectedVars {
		if !strings.Contains(errStr, v) {
			t.Errorf("expected error to mention missing var %s, got: %v", v, err)
		}
	}
}

// TestNormalizeRecipientAddress covers the three branches of the new
// startup-time address normalization: empty (no-op), valid hex with
// non-canonical case (rewrites to EIP-55), and invalid hex (returns error).
// The non-canonical case is the exact bug that produced the production
// `bad address checksum` wallet rejection.
func TestNormalizeRecipientAddress(t *testing.T) {
	t.Run("empty leaves env unset and returns nil", func(t *testing.T) {
		t.Setenv("RECIPIENT_ADDRESS", "")
		if err := normalizeRecipientAddress(); err != nil {
			t.Fatalf("expected nil error for unset address, got: %v", err)
		}
		if got := os.Getenv("RECIPIENT_ADDRESS"); got != "" {
			t.Errorf("expected env to remain unset, got %q", got)
		}
	})

	t.Run("non-canonical case is rewritten to EIP-55", func(t *testing.T) {
		// Lowercase form of the canonical hardhat account 0.
		t.Setenv("RECIPIENT_ADDRESS", "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
		if err := normalizeRecipientAddress(); err != nil {
			t.Fatalf("expected nil error for valid lowercased address, got: %v", err)
		}
		want := "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
		if got := os.Getenv("RECIPIENT_ADDRESS"); got != want {
			t.Errorf("expected normalized %s, got %s", want, got)
		}
	})

	t.Run("invalid hex returns error", func(t *testing.T) {
		t.Setenv("RECIPIENT_ADDRESS", "0xZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ")
		if err := normalizeRecipientAddress(); err == nil {
			t.Fatal("expected error for invalid hex, got nil")
		}
	})
}

func TestValidateConfig_WithRequiredEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("VERIFIER_URL", "http://127.0.0.1:3002")
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("RECEIPT_STORE", "memory")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")

	err := validateConfig()
	if err != nil {
		t.Fatalf("expected no error when required env vars are set, got: %v", err)
	}
}

func TestValidateConfig_CacheEnabledRequiresRedis(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("VERIFIER_URL", "http://127.0.0.1:3002")
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("RECEIPT_STORE", "memory")
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
	t.Setenv("VERIFIER_URL", "http://127.0.0.1:3002")
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("RECEIPT_STORE", "memory")
	t.Setenv("REDIS_URL", "localhost:6379")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")

	err := validateConfig()
	if err != nil {
		t.Fatalf("expected no error when all required vars are set, got: %v", err)
	}
}

func TestValidateConfig_DefaultReceiptStoreRequiresRedis(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("VERIFIER_URL", "http://127.0.0.1:3002")
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("RECEIPT_STORE", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")

	err := validateConfig()
	if err == nil {
		t.Fatal("expected error when default redis receipt store has no REDIS_URL")
	}
	if !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("expected error to mention REDIS_URL, got: %v", err)
	}
}

func TestValidateConfig_MemoryReceiptStoreDoesNotRequireRedis(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("VERIFIER_URL", "http://127.0.0.1:3002")
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("RECEIPT_STORE", "memory")
	t.Setenv("REDIS_URL", "")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")

	err := validateConfig()
	if err != nil {
		t.Fatalf("expected memory receipt store to allow missing Redis, got: %v", err)
	}
}

func TestValidateConfig_InvalidReceiptStoreMode(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("VERIFIER_URL", "http://127.0.0.1:3002")
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("RECEIPT_STORE", "postgres")
	t.Setenv("REDIS_URL", "localhost:6379")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")

	err := validateConfig()
	if err == nil {
		t.Fatal("expected invalid RECEIPT_STORE mode to fail validation")
	}
	if !strings.Contains(err.Error(), "RECEIPT_STORE") {
		t.Fatalf("expected error to mention RECEIPT_STORE, got: %v", err)
	}
}

func TestGetReceiptStoreMode(t *testing.T) {
	t.Setenv("RECEIPT_STORE", "")
	if got := getReceiptStoreMode(); got != "redis" {
		t.Fatalf("expected default receipt store mode redis, got %q", got)
	}

	t.Setenv("RECEIPT_STORE", " memory ")
	if got := getReceiptStoreMode(); got != "memory" {
		t.Fatalf("expected trimmed receipt store mode memory, got %q", got)
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
			errMsg:  "REDIS_URL not set",
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

func TestGetAllowedOrigins(t *testing.T) {
	tests := []struct {
		name string
		env  *string
		want []string
	}{
		{
			name: "unset env returns localhost default",
			env:  nil,
			want: []string{"http://localhost:3001"},
		},
		{
			name: "blank env returns localhost default",
			env:  stringPtr("   "),
			want: []string{"http://localhost:3001"},
		},
		{
			name: "multi-origin env returns trimmed origins in order",
			env:  stringPtr(" https://app.example.com,https://admin.example.com , http://localhost:3001 "),
			want: []string{"https://app.example.com", "https://admin.example.com", "http://localhost:3001"},
		},
		{
			name: "empty comma entries are ignored",
			env:  stringPtr("https://app.example.com,, ,https://admin.example.com"),
			want: []string{"https://app.example.com", "https://admin.example.com"},
		},
		{
			name: "all empty comma entries fall back to localhost",
			env:  stringPtr(" , ,, "),
			want: []string{"http://localhost:3001"},
		},
		{
			name: "invalid origin entries are ignored",
			env:  stringPtr("ftp://app.example.com,https://app.example.com/path,https://admin.example.com"),
			want: []string{"https://admin.example.com"},
		},
		{
			name: "all invalid origin entries fall back to localhost",
			env:  stringPtr("*,javascript:alert(1),https://app.example.com/?debug=true"),
			want: []string{"http://localhost:3001"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env == nil {
				previous, hadPrevious := os.LookupEnv("ALLOWED_ORIGINS")
				if err := os.Unsetenv("ALLOWED_ORIGINS"); err != nil {
					t.Fatalf("failed to unset ALLOWED_ORIGINS: %v", err)
				}
				t.Cleanup(func() {
					if hadPrevious {
						_ = os.Setenv("ALLOWED_ORIGINS", previous)
					} else {
						_ = os.Unsetenv("ALLOWED_ORIGINS")
					}
				})
			} else {
				t.Setenv("ALLOWED_ORIGINS", *tt.env)
			}

			got := getAllowedOrigins()

			if len(got) != len(tt.want) {
				t.Fatalf("expected %d origins, got %d: %v", len(tt.want), len(got), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("origin %d mismatch: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGetReceiptTTL(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("RECEIPT_TTL", "")
		if got := getReceiptTTL(); got != 24*time.Hour {
			t.Fatalf("expected default receipt TTL 24h, got %v", got)
		}
	})

	t.Run("custom positive", func(t *testing.T) {
		t.Setenv("RECEIPT_TTL", "120")
		if got := getReceiptTTL(); got != 2*time.Minute {
			t.Fatalf("expected custom receipt TTL 2m, got %v", got)
		}
	})

	t.Run("zero falls back to default", func(t *testing.T) {
		t.Setenv("RECEIPT_TTL", "0")
		if got := getReceiptTTL(); got != 24*time.Hour {
			t.Fatalf("expected zero receipt TTL to fall back to 24h, got %v", got)
		}
	})

	t.Run("negative falls back to default", func(t *testing.T) {
		t.Setenv("RECEIPT_TTL", "-10")
		if got := getReceiptTTL(); got != 24*time.Hour {
			t.Fatalf("expected negative receipt TTL to fall back to 24h, got %v", got)
		}
	})
}

func stringPtr(value string) *string {
	return &value
}
