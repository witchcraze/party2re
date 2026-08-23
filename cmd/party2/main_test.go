package main

import (
	"os"
	"testing"
)

func TestRunSucceedsWhenDatabaseIsConfigured(t *testing.T) {
	dsn := os.Getenv("PARTY2_DB_DSN")
	if dsn == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunFailsWhenDatabaseDSNIsMissing(t *testing.T) {
	original := os.Getenv("PARTY2_DB_DSN")
	defer func() {
		if original != "" {
			_ = os.Setenv("PARTY2_DB_DSN", original)
		} else {
			_ = os.Unsetenv("PARTY2_DB_DSN")
		}
	}()

	_ = os.Unsetenv("PARTY2_DB_DSN")
	if err := run(); err == nil {
		t.Fatal("run() expected error when PARTY2_DB_DSN is unset, got nil")
	}
}

func TestRunFailsWhenDatabaseDSNIsInvalid(t *testing.T) {
	original := os.Getenv("PARTY2_DB_DSN")
	defer func() {
		if original != "" {
			_ = os.Setenv("PARTY2_DB_DSN", original)
		} else {
			_ = os.Unsetenv("PARTY2_DB_DSN")
		}
	}()

	_ = os.Setenv("PARTY2_DB_DSN", "invalid_user:invalid_pass@tcp(127.0.0.1:9999)/invalid_db")
	if err := run(); err == nil {
		t.Fatal("run() expected error when PARTY2_DB_DSN is unreachable, got nil")
	}
}
