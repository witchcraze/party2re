package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	nethttp "net/http"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/logging"
)

func TestResolveServerAddr(t *testing.T) {
	origAddr := os.Getenv("ADDR")
	origPartyAddr := os.Getenv("PARTY2_ADDR")
	origPort := os.Getenv("PORT")
	origPartyPort := os.Getenv("PARTY2_PORT")
	defer func() {
		restoreEnv("ADDR", origAddr)
		restoreEnv("PARTY2_ADDR", origPartyAddr)
		restoreEnv("PORT", origPort)
		restoreEnv("PARTY2_PORT", origPartyPort)
	}()

	t.Run("defaults to 8080", func(t *testing.T) {
		_ = os.Unsetenv("ADDR")
		_ = os.Unsetenv("PARTY2_ADDR")
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("PARTY2_PORT")

		addr := resolveServerAddr()
		if addr != ":8080" {
			t.Fatalf("expected :8080, got %s", addr)
		}
	})

	t.Run("uses PORT environment variable", func(t *testing.T) {
		_ = os.Unsetenv("ADDR")
		_ = os.Unsetenv("PARTY2_ADDR")
		_ = os.Unsetenv("PARTY2_PORT")
		_ = os.Setenv("PORT", "9090")

		addr := resolveServerAddr()
		if addr != ":9090" {
			t.Fatalf("expected :9090, got %s", addr)
		}
	})

	t.Run("uses PARTY2_PORT with leading colon", func(t *testing.T) {
		_ = os.Unsetenv("ADDR")
		_ = os.Unsetenv("PARTY2_ADDR")
		_ = os.Setenv("PARTY2_PORT", ":7070")

		addr := resolveServerAddr()
		if addr != ":7070" {
			t.Fatalf("expected :7070, got %s", addr)
		}
	})

	t.Run("uses ADDR environment variable", func(t *testing.T) {
		_ = os.Unsetenv("PARTY2_ADDR")
		_ = os.Setenv("ADDR", "127.0.0.1:3000")

		addr := resolveServerAddr()
		if addr != "127.0.0.1:3000" {
			t.Fatalf("expected 127.0.0.1:3000, got %s", addr)
		}
	})

	t.Run("PARTY2_ADDR overrides all others", func(t *testing.T) {
		_ = os.Setenv("PORT", "8080")
		_ = os.Setenv("ADDR", "0.0.0.0:8080")
		_ = os.Setenv("PARTY2_ADDR", "192.168.1.1:4000")

		addr := resolveServerAddr()
		if addr != "192.168.1.1:4000" {
			t.Fatalf("expected 192.168.1.1:4000, got %s", addr)
		}
	})
}

func TestRunLifecycleGracefulShutdown(t *testing.T) {
	dsn := os.Getenv("PARTY2_DB_DSN")
	if dsn == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	// Use an ephemeral port on localhost
	origAddr := os.Getenv("PARTY2_ADDR")
	defer restoreEnv("PARTY2_ADDR", origAddr)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate ephemeral port: %v", err)
	}
	serverAddr := ln.Addr().String()
	_ = ln.Close()

	_ = os.Setenv("PARTY2_ADDR", serverAddr)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- run(ctx, logging.Nop())
	}()

	// Poll until the server responds to /health
	client := &nethttp.Client{Timeout: 500 * time.Millisecond}
	healthURL := fmt.Sprintf("http://%s/health", serverAddr)
	var healthy bool

	for i := 0; i < 30; i++ {
		time.Sleep(50 * time.Millisecond)
		resp, err := client.Get(healthURL)
		if err == nil && resp.StatusCode == nethttp.StatusOK {
			var body struct {
				Status string `json:"status"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Status == "ok" {
				_ = resp.Body.Close()
				healthy = true
				break
			}
			_ = resp.Body.Close()
		}
	}

	if !healthy {
		cancel()
		t.Fatalf("server at %s did not become healthy in time", healthURL)
	}

	// Trigger graceful shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run() returned error on graceful shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not terminate within timeout after cancellation")
	}
}

func TestRunFailsWhenDatabaseDSNIsMissing(t *testing.T) {
	original := os.Getenv("PARTY2_DB_DSN")
	defer restoreEnv("PARTY2_DB_DSN", original)

	_ = os.Unsetenv("PARTY2_DB_DSN")
	if err := run(context.Background(), logging.Nop()); err == nil {
		t.Fatal("run() expected error when PARTY2_DB_DSN is unset, got nil")
	}
}

func TestRunFailsWhenDatabaseDSNIsInvalid(t *testing.T) {
	original := os.Getenv("PARTY2_DB_DSN")
	defer restoreEnv("PARTY2_DB_DSN", original)

	_ = os.Setenv("PARTY2_DB_DSN", "invalid_user:invalid_pass@tcp(127.0.0.1:9999)/invalid_db")
	if err := run(context.Background(), logging.Nop()); err == nil {
		t.Fatal("run() expected error when PARTY2_DB_DSN is unreachable, got nil")
	}
}

func TestRunFailsWhenPortIsUnavailable(t *testing.T) {
	dsn := os.Getenv("PARTY2_DB_DSN")
	if dsn == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	origAddr := os.Getenv("PARTY2_ADDR")
	defer restoreEnv("PARTY2_ADDR", origAddr)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind port: %v", err)
	}
	defer ln.Close()

	_ = os.Setenv("PARTY2_ADDR", ln.Addr().String())

	if err := run(context.Background(), logging.Nop()); err == nil {
		t.Fatal("run() expected error when port is already in use, got nil")
	}
}

func restoreEnv(key, value string) {
	if value != "" {
		_ = os.Setenv(key, value)
	} else {
		_ = os.Unsetenv(key)
	}
}
