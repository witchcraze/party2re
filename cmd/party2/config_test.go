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
	defer func() {
		restoreEnv("PARTY2_DB_DSN", origDB)
		restoreEnv("PARTY2_ADMIN_API_KEY", origAdmin)
		restoreEnv("PARTY2_CORS_ORIGINS", origCORS)
		restoreEnv("PARTY2_ADDR", origAddr)
		restoreEnv("PORT", origPort)
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
	})
}

func TestConfigStructParallelCreation(t *testing.T) {
	t.Parallel()

	// Verify that Config structs can be constructed in-memory without mutating process env
	cfg := Config{
		DB:     database.DefaultConfig("user:pass@tcp(127.0.0.1:3306)/party2"),
		Valkey: valkey.DefaultConfig("127.0.0.1:6379"),
		Server: struct{ Addr string }{Addr: ":8080"},
		Admin:  struct{ APIKey string }{APIKey: "test-admin-key"},
		CORS:   struct{ AllowedOrigins string }{AllowedOrigins: "http://example.com"},
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
	if cfg.Admin.APIKey != "test-admin-key" {
		t.Errorf("unexpected admin api key: %s", cfg.Admin.APIKey)
	}
	if cfg.CORS.AllowedOrigins != "http://example.com" {
		t.Errorf("unexpected cors allowed origins: %s", cfg.CORS.AllowedOrigins)
	}
}
