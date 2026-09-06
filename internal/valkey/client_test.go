package valkey

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	t.Run("empty address defaults to localhost:6379", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultConfig("")
		if cfg.Address != DefaultAddress {
			t.Errorf("expected %s, got %s", DefaultAddress, cfg.Address)
		}
		if cfg.Password != "" {
			t.Errorf("expected empty password, got %s", cfg.Password)
		}
		if cfg.DB != 0 {
			t.Errorf("expected DB 0, got %d", cfg.DB)
		}
		if cfg.DialTimeout != DefaultDialTimeout {
			t.Errorf("expected %v, got %v", DefaultDialTimeout, cfg.DialTimeout)
		}
	})

	t.Run("custom address is preserved", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultConfig("valkey.internal:6380")
		if cfg.Address != "valkey.internal:6380" {
			t.Errorf("expected valkey.internal:6380, got %s", cfg.Address)
		}
	})
}

func TestConfig_ToClientOption(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Address:     "cache.prod:6379",
		Password:    "secret",
		DB:          2,
		DialTimeout: 10 * time.Second,
	}

	opts := cfg.ToClientOption()
	if len(opts.InitAddress) != 1 || opts.InitAddress[0] != "cache.prod:6379" {
		t.Errorf("expected InitAddress cache.prod:6379, got %v", opts.InitAddress)
	}
	if opts.Password != "secret" {
		t.Errorf("expected password secret, got %s", opts.Password)
	}
	if opts.SelectDB != 2 {
		t.Errorf("expected SelectDB 2, got %d", opts.SelectDB)
	}
	if opts.Dialer.Timeout != 10*time.Second {
		t.Errorf("expected Dialer.Timeout 10s, got %v", opts.Dialer.Timeout)
	}
}

func TestConfigFromEnvironment(t *testing.T) {
	// Not parallel because it mutates process environment variables
	origAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	origPass := os.Getenv("PARTY2_VALKEY_PASSWORD")
	origDB := os.Getenv("PARTY2_VALKEY_DB")
	origTimeout := os.Getenv("PARTY2_VALKEY_DIAL_TIMEOUT")

	defer func() {
		restoreEnv("PARTY2_VALKEY_ADDR", origAddr)
		restoreEnv("PARTY2_VALKEY_PASSWORD", origPass)
		restoreEnv("PARTY2_VALKEY_DB", origDB)
		restoreEnv("PARTY2_VALKEY_DIAL_TIMEOUT", origTimeout)
	}()

	t.Run("defaults when unconfigured", func(t *testing.T) {
		_ = os.Unsetenv("PARTY2_VALKEY_ADDR")
		_ = os.Unsetenv("VALKEY_ADDR")
		_ = os.Unsetenv("PARTY2_VALKEY_PASSWORD")
		_ = os.Unsetenv("PARTY2_VALKEY_DB")
		_ = os.Unsetenv("PARTY2_VALKEY_DIAL_TIMEOUT")

		cfg := ConfigFromEnvironment()
		if cfg.Address != DefaultAddress {
			t.Errorf("expected default address %s, got %s", DefaultAddress, cfg.Address)
		}
		if cfg.Password != "" {
			t.Errorf("expected empty password, got %s", cfg.Password)
		}
		if cfg.DB != 0 {
			t.Errorf("expected DB 0, got %d", cfg.DB)
		}
		if cfg.DialTimeout != DefaultDialTimeout {
			t.Errorf("expected timeout %v, got %v", DefaultDialTimeout, cfg.DialTimeout)
		}
	})

	t.Run("reads custom env values", func(t *testing.T) {
		_ = os.Setenv("PARTY2_VALKEY_ADDR", "valkey-cluster:6380")
		_ = os.Setenv("PARTY2_VALKEY_PASSWORD", "supersecret")
		_ = os.Setenv("PARTY2_VALKEY_DB", "3")
		_ = os.Setenv("PARTY2_VALKEY_DIAL_TIMEOUT", "2s")

		cfg := ConfigFromEnvironment()
		if cfg.Address != "valkey-cluster:6380" {
			t.Errorf("expected address valkey-cluster:6380, got %s", cfg.Address)
		}
		if cfg.Password != "supersecret" {
			t.Errorf("expected password supersecret, got %s", cfg.Password)
		}
		if cfg.DB != 3 {
			t.Errorf("expected DB 3, got %d", cfg.DB)
		}
		if cfg.DialTimeout != 2*time.Second {
			t.Errorf("expected timeout 2s, got %v", cfg.DialTimeout)
		}
	})

	t.Run("falls back gracefully on invalid DB and timeout", func(t *testing.T) {
		_ = os.Setenv("PARTY2_VALKEY_ADDR", "localhost:6379")
		_ = os.Setenv("PARTY2_VALKEY_DB", "invalid-int")
		_ = os.Setenv("PARTY2_VALKEY_DIAL_TIMEOUT", "invalid-duration")

		cfg := ConfigFromEnvironment()
		if cfg.DB != 0 {
			t.Errorf("expected fallback DB 0, got %d", cfg.DB)
		}
		if cfg.DialTimeout != DefaultDialTimeout {
			t.Errorf("expected fallback timeout %v, got %v", DefaultDialTimeout, cfg.DialTimeout)
		}
	})
}

func TestNewClientWithConfig_Unreachable(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Address:     "127.0.0.1:1", // guaranteed unreachable port
		DialTimeout: 50 * time.Millisecond,
	}

	client, err := NewClientWithConfig(cfg)
	if err == nil {
		if client != nil {
			client.Close()
		}
		t.Fatal("expected error connecting to unreachable address, got nil")
	}
}

func TestNewClientWithConfig_Reachable(t *testing.T) {
	t.Parallel()

	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	cfg := DefaultConfig(addr)
	cfg.DialTimeout = 500 * time.Millisecond

	client, err := NewClientWithConfig(cfg)
	if err != nil {
		t.Skipf("Valkey not reachable at %s, skipping live connection test: %v", addr, err)
	}
	defer client.Close()
}

func restoreEnv(key, val string) {
	if val != "" {
		_ = os.Setenv(key, val)
	} else {
		_ = os.Unsetenv(key)
	}
}
