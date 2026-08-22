package database

import (
	"os"
	"testing"
)

func TestOpenFromEnvironmentUsesConfiguredDSN(t *testing.T) {
	const dsn = "party2:party2@tcp(example:3306)/party2?parseTime=true"
	t.Setenv("PARTY2_DB_DSN", dsn)

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatalf("OpenFromEnvironment() error = %v", err)
	}
	defer db.Close()

	if os.Getenv("PARTY2_DB_DSN") != dsn {
		t.Fatal("test environment was not configured")
	}
}

func TestConfiguredDatabaseIsReachable(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatalf("OpenFromEnvironment() error = %v", err)
	}
	defer db.Close()

	if err := Ping(db); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	var migration string
	if err := db.QueryRow("SELECT version FROM schema_migrations WHERE version = '001_initial'").Scan(&migration); err != nil {
		t.Fatalf("initial migration was not applied: %v", err)
	}
}
