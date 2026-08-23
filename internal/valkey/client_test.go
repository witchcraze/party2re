package valkey

import (
	"os"
	"testing"
)

func TestGetConfig(t *testing.T) {
	// Backup original env
	orig := os.Getenv("PARTY2_VALKEY_ADDR")
	defer os.Setenv("PARTY2_VALKEY_ADDR", orig)

	t.Run("default config", func(t *testing.T) {
		os.Unsetenv("PARTY2_VALKEY_ADDR")
		cfg := GetConfig()
		if len(cfg.InitAddress) != 1 || cfg.InitAddress[0] != "localhost:6379" {
			t.Errorf("expected default address localhost:6379, got %v", cfg.InitAddress)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		os.Setenv("PARTY2_VALKEY_ADDR", "valkey-svc:1234")
		cfg := GetConfig()
		if len(cfg.InitAddress) != 1 || cfg.InitAddress[0] != "valkey-svc:1234" {
			t.Errorf("expected valkey-svc:1234, got %v", cfg.InitAddress)
		}
	})
}
