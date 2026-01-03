package main

import "testing"

func TestValidateConfig_MissingRequiredEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	err := validateConfig()
	if err == nil {
		t.Fatalf("expected error when OPENROUTER_API_KEY is missing, got nil")
	}
}

func TestValidateConfig_WithRequiredEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	err := validateConfig()
	if err != nil {
		t.Fatalf("expected no error when OPENROUTER_API_KEY is set, got: %v", err)
	}
}