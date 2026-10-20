package main

import (
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/valkey"
)

func TestConfigFromEnv(t *testing.T) {
	origDB := os.Getenv("PARTY2_DB_DSN")
	origAdmin := os.Getenv("PARTY2_ADMIN_API_KEY")
	origCORS := os.Getenv("PARTY2_CORS_ORIGINS")
	origAddr := os.Getenv("PARTY2_ADDR")
	origPort := os.Getenv("PORT")
	origTrusted := os.Getenv("PARTY2_TRUSTED_PROXIES")
	origMaxPlayers := os.Getenv("PARTY2_MAX_PLAYERS")
	defer func() {
		restoreEnv("PARTY2_DB_DSN", origDB)
		restoreEnv("PARTY2_ADMIN_API_KEY", origAdmin)
		restoreEnv("PARTY2_CORS_ORIGINS", origCORS)
		restoreEnv("PARTY2_ADDR", origAddr)
		restoreEnv("PORT", origPort)
		restoreEnv("PARTY2_TRUSTED_PROXIES", origTrusted)
		restoreEnv("PARTY2_MAX_PLAYERS", origMaxPlayers)
	}()

	t.Run("missing DB DSN returns error", func(t *testing.T) {
		_ = os.Unsetenv("PARTY2_DB_DSN")
		_, err := ConfigFromEnv()
		if err == nil {
			t.Fatal("expected error when PARTY2_DB_DSN is unset, got nil")
		}
	})

	t.Run("valid configuration loaded successfully", func(t *testing.T) {
		_ = os.Setenv("PARTY2_DB_DSN", "test:test@tcp(127.0.0.1:3306)/test_db")
		_ = os.Setenv("PARTY2_ADMIN_API_KEY", "super-admin-key")
		_ = os.Setenv("PARTY2_CORS_ORIGINS", "http://localhost:3000,http://app.party2.test")
		_ = os.Setenv("PARTY2_ADDR", ":8888")
		_ = os.Setenv("PARTY2_TRUSTED_PROXIES", "127.0.0.1/32,10.0.0.0/8")

		cfg, err := ConfigFromEnv()
		if err != nil {
			t.Fatalf("unexpected error loading config: %v", err)
		}

		if cfg.DB.DSN != "test:test@tcp(127.0.0.1:3306)/test_db" {
			t.Errorf("expected DSN test:test@tcp(127.0.0.1:3306)/test_db, got %s", cfg.DB.DSN)
		}
		if cfg.Admin.APIKey != "super-admin-key" {
			t.Errorf("expected admin key super-admin-key, got %s", cfg.Admin.APIKey)
		}
		if cfg.CORS.AllowedOrigins != "http://localhost:3000,http://app.party2.test" {
			t.Errorf("expected CORS origins http://localhost:3000,http://app.party2.test, got %s", cfg.CORS.AllowedOrigins)
		}
		if cfg.Server.Addr != ":8888" {
			t.Errorf("expected server addr :8888, got %s", cfg.Server.Addr)
		}
		if cfg.Server.TrustedProxies != "127.0.0.1/32,10.0.0.0/8" {
			t.Errorf("expected trusted proxies 127.0.0.1/32,10.0.0.0/8, got %s", cfg.Server.TrustedProxies)
		}
		if cfg.Player.MaxPlayers != 999 {
			t.Errorf("expected default max players 999, got %d", cfg.Player.MaxPlayers)
		}
	})

	t.Run("PARTY2_MAX_PLAYERS parsing", func(t *testing.T) {
		_ = os.Setenv("PARTY2_DB_DSN", "test:test@tcp(127.0.0.1:3306)/test_db")

		testCases := []struct {
			name        string
			envVal      string
			setEnv      bool
			expected    int
			expectError bool
		}{
			{
				name:        "unset defaults to 999",
				setEnv:      false,
				expected:    999,
				expectError: false,
			},
			{
				name:        "empty defaults to 999",
				envVal:      "   ",
				setEnv:      true,
				expected:    999,
				expectError: false,
			},
			{
				name:        "custom positive value 500",
				envVal:      "500",
				setEnv:      true,
				expected:    500,
				expectError: false,
			},
			{
				name:        "zero interpreted as unlimited",
				envVal:      "0",
				setEnv:      true,
				expected:    0,
				expectError: false,
			},
			{
				name:        "negative interpreted as unlimited",
				envVal:      "-1",
				setEnv:      true,
				expected:    -1,
				expectError: false,
			},
			{
				name:        "invalid non-integer returns error",
				envVal:      "abc",
				setEnv:      true,
				expectError: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.setEnv {
					_ = os.Setenv("PARTY2_MAX_PLAYERS", tc.envVal)
				} else {
					_ = os.Unsetenv("PARTY2_MAX_PLAYERS")
				}

				cfg, err := ConfigFromEnv()
				if tc.expectError {
					if err == nil {
						t.Fatal("expected error, got nil")
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.Player.MaxPlayers != tc.expected {
					t.Errorf("expected max players %d, got %d", tc.expected, cfg.Player.MaxPlayers)
				}
			})
		}
	})

	t.Run("invalid trusted proxies returns error", func(t *testing.T) {
		_ = os.Setenv("PARTY2_DB_DSN", "test:test@tcp(127.0.0.1:3306)/test_db")
		_ = os.Setenv("PARTY2_TRUSTED_PROXIES", "invalid-cidr/999")

		_, err := ConfigFromEnv()
		if err == nil {
			t.Fatal("expected error for invalid PARTY2_TRUSTED_PROXIES, got nil")
		}
	})
}

func TestConfigStructParallelCreation(t *testing.T) {
	t.Parallel()

	// Verify that Config structs can be constructed in-memory without mutating process env
	cfg := Config{
		DB:     database.DefaultConfig("user:pass@tcp(127.0.0.1:3306)/party2"),
		Valkey: valkey.DefaultConfig("127.0.0.1:6379"),
		Server: struct {
			Addr           string
			TrustedProxies string
		}{Addr: ":8080", TrustedProxies: "127.0.0.1/32"},
		Admin:  struct{ APIKey string }{APIKey: "test-admin-key"},
		CORS:   struct{ AllowedOrigins string }{AllowedOrigins: "http://example.com"},
		Player: struct{ MaxPlayers int }{MaxPlayers: 999},
	}

	if cfg.DB.DSN != "user:pass@tcp(127.0.0.1:3306)/party2" {
		t.Errorf("unexpected DSN: %s", cfg.DB.DSN)
	}
	if cfg.Valkey.Address != "127.0.0.1:6379" {
		t.Errorf("unexpected valkey addr: %s", cfg.Valkey.Address)
	}
	if cfg.Valkey.DialTimeout != 5*time.Second {
		t.Errorf("unexpected valkey dial timeout: %v", cfg.Valkey.DialTimeout)
	}
	if cfg.Server.TrustedProxies != "127.0.0.1/32" {
		t.Errorf("unexpected trusted proxies: %s", cfg.Server.TrustedProxies)
	}
	if cfg.Admin.APIKey != "test-admin-key" {
		t.Errorf("unexpected admin api key: %s", cfg.Admin.APIKey)
	}
	if cfg.CORS.AllowedOrigins != "http://example.com" {
		t.Errorf("unexpected cors allowed origins: %s", cfg.CORS.AllowedOrigins)
	}
	if cfg.Player.MaxPlayers != 999 {
		t.Errorf("unexpected player max players: %d", cfg.Player.MaxPlayers)
	}
}
