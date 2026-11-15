package database

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOpenTestDBUnconfigured(t *testing.T) {
	t.Setenv("PARTY2_DB_DSN", "")
	var skipped bool
	if !t.Run("unconfigured", func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		OpenTestDB(t)
		t.Error("returned from an unconfigured fixture")
	}) || !skipped {
		t.Fatal("unconfigured fixture must skip")
	}
}

func TestOpenTestDBInvalidConfiguration(t *testing.T) {
	if os.Getenv("PARTY2_TEST_DB_FAILURE_CHILD") == "1" {
		OpenTestDB(t)
		return
	}
	t.Setenv("PARTY2_TEST_DB_FAILURE_CHILD", "1")
	t.Setenv("PARTY2_DB_DSN", "invalid-dsn")
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestOpenTestDBInvalidConfiguration$", "-test.v")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "open test database:") || strings.Contains(string(out), "--- SKIP:") {
		t.Fatalf("configured opening failure must fail, not skip: err=%v\n%s", err, out)
	}
}

func TestOpenTestDBLifecycle(t *testing.T) {
	observer := OpenTestDB(t)
	var want string
	if err := observer.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&want); err != nil {
		t.Fatal(err)
	}
	for _, earlyReturn := range []bool{false, true} {
		var db *sql.DB
		cleanupRan := false
		t.Run("scope", func(t *testing.T) {
			db = OpenTestDB(t)
			t.Cleanup(func() {
				var got string
				if err := db.QueryRowContext(context.Background(), "SELECT DATABASE()").Scan(&got); err != nil {
					t.Errorf("connection unavailable during fixture cleanup: %v", err)
				} else if got != want {
					t.Errorf("selected database = %q, want %q", got, want)
				}
				cleanupRan = true
			})
			if earlyReturn {
				return
			}
			if err := db.PingContext(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
		if !cleanupRan {
			t.Error("fixture cleanup did not run")
		}
		if err := db.PingContext(t.Context()); err == nil || !strings.Contains(err.Error(), "database is closed") {
			t.Fatalf("connection not closed after fixture cleanup: %v", err)
		}
	}
}
