package database

import (
	"os"
	"testing"
	"time"
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

	stats := db.Stats()
	if stats.MaxOpenConnections != DefaultMaxOpenConns {
		t.Errorf("expected MaxOpenConnections=%d, got %d", DefaultMaxOpenConns, stats.MaxOpenConnections)
	}
}

func TestConfigFromEnvironment_Defaults(t *testing.T) {
	const dsn = "party2:party2@tcp(localhost:3306)/party2"
	t.Setenv("PARTY2_DB_DSN", dsn)
	t.Setenv("PARTY2_DB_MAX_OPEN_CONNS", "")
	t.Setenv("PARTY2_DB_MAX_IDLE_CONNS", "")
	t.Setenv("PARTY2_DB_CONN_MAX_LIFETIME", "")
	t.Setenv("PARTY2_DB_CONN_MAX_IDLE_TIME", "")

	cfg, err := ConfigFromEnvironment()
	if err != nil {
		t.Fatalf("ConfigFromEnvironment() error = %v", err)
	}

	if cfg.DSN != dsn {
		t.Errorf("cfg.DSN = %q, want %q", cfg.DSN, dsn)
	}
	if cfg.MaxOpenConns != DefaultMaxOpenConns {
		t.Errorf("cfg.MaxOpenConns = %d, want %d", cfg.MaxOpenConns, DefaultMaxOpenConns)
	}
	if cfg.MaxIdleConns != DefaultMaxIdleConns {
		t.Errorf("cfg.MaxIdleConns = %d, want %d", cfg.MaxIdleConns, DefaultMaxIdleConns)
	}
	if cfg.ConnMaxLifetime != DefaultConnMaxLifetime {
		t.Errorf("cfg.ConnMaxLifetime = %v, want %v", cfg.ConnMaxLifetime, DefaultConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime != DefaultConnMaxIdleTime {
		t.Errorf("cfg.ConnMaxIdleTime = %v, want %v", cfg.ConnMaxIdleTime, DefaultConnMaxIdleTime)
	}
}

func TestConfigFromEnvironment_CustomValid(t *testing.T) {
	const dsn = "party2:party2@tcp(localhost:3306)/party2"
	t.Setenv("PARTY2_DB_DSN", dsn)
	t.Setenv("PARTY2_DB_MAX_OPEN_CONNS", "50")
	t.Setenv("PARTY2_DB_MAX_IDLE_CONNS", "30")
	t.Setenv("PARTY2_DB_CONN_MAX_LIFETIME", "10m")
	t.Setenv("PARTY2_DB_CONN_MAX_IDLE_TIME", "2m")

	cfg, err := ConfigFromEnvironment()
	if err != nil {
		t.Fatalf("ConfigFromEnvironment() error = %v", err)
	}

	if cfg.MaxOpenConns != 50 {
		t.Errorf("cfg.MaxOpenConns = %d, want 50", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 30 {
		t.Errorf("cfg.MaxIdleConns = %d, want 30", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 10*time.Minute {
		t.Errorf("cfg.ConnMaxLifetime = %v, want 10m", cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime != 2*time.Minute {
		t.Errorf("cfg.ConnMaxIdleTime = %v, want 2m", cfg.ConnMaxIdleTime)
	}
}

func TestConfigFromEnvironment_InvalidFallback(t *testing.T) {
	const dsn = "party2:party2@tcp(localhost:3306)/party2"
	t.Setenv("PARTY2_DB_DSN", dsn)
	t.Setenv("PARTY2_DB_MAX_OPEN_CONNS", "-5")
	t.Setenv("PARTY2_DB_MAX_IDLE_CONNS", "invalid_int")
	t.Setenv("PARTY2_DB_CONN_MAX_LIFETIME", "invalid_duration")
	t.Setenv("PARTY2_DB_CONN_MAX_IDLE_TIME", "-10s")

	cfg, err := ConfigFromEnvironment()
	if err != nil {
		t.Fatalf("ConfigFromEnvironment() error = %v", err)
	}

	if cfg.MaxOpenConns != DefaultMaxOpenConns {
		t.Errorf("cfg.MaxOpenConns = %d, want default %d", cfg.MaxOpenConns, DefaultMaxOpenConns)
	}
	if cfg.MaxIdleConns != DefaultMaxIdleConns {
		t.Errorf("cfg.MaxIdleConns = %d, want default %d", cfg.MaxIdleConns, DefaultMaxIdleConns)
	}
	if cfg.ConnMaxLifetime != DefaultConnMaxLifetime {
		t.Errorf("cfg.ConnMaxLifetime = %v, want default %v", cfg.ConnMaxLifetime, DefaultConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime != DefaultConnMaxIdleTime {
		t.Errorf("cfg.ConnMaxIdleTime = %v, want default %v", cfg.ConnMaxIdleTime, DefaultConnMaxIdleTime)
	}
}

func TestConfigFromEnvironment_IdleCappedAtOpen(t *testing.T) {
	const dsn = "party2:party2@tcp(localhost:3306)/party2"
	t.Setenv("PARTY2_DB_DSN", dsn)
	t.Setenv("PARTY2_DB_MAX_OPEN_CONNS", "10")
	t.Setenv("PARTY2_DB_MAX_IDLE_CONNS", "20")

	cfg, err := ConfigFromEnvironment()
	if err != nil {
		t.Fatalf("ConfigFromEnvironment() error = %v", err)
	}

	if cfg.MaxOpenConns != 10 {
		t.Errorf("cfg.MaxOpenConns = %d, want 10", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 10 {
		t.Errorf("cfg.MaxIdleConns = %d, want capped 10", cfg.MaxIdleConns)
	}
}

func TestConfigFromEnvironment_MissingDSN(t *testing.T) {
	t.Setenv("PARTY2_DB_DSN", "")

	_, err := ConfigFromEnvironment()
	if err == nil {
		t.Fatal("expected error when PARTY2_DB_DSN is empty, got nil")
	}
}

func TestOpenWithConfig_CustomSettings(t *testing.T) {
	cfg := Config{
		DSN:             "party2:party2@tcp(example:3306)/party2",
		MaxOpenConns:    42,
		MaxIdleConns:    18,
		ConnMaxLifetime: 7 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	db, err := OpenWithConfig(cfg)
	if err != nil {
		t.Fatalf("OpenWithConfig() error = %v", err)
	}
	defer db.Close()

	stats := db.Stats()
	if stats.MaxOpenConnections != 42 {
		t.Errorf("stats.MaxOpenConnections = %d, want 42", stats.MaxOpenConnections)
	}
}

func TestOpenWithConfig_MissingDSN(t *testing.T) {
	_, err := OpenWithConfig(Config{})
	if err == nil {
		t.Fatal("expected error with empty DSN, got nil")
	}
}

func TestOpen_DefaultSettings(t *testing.T) {
	db, err := Open("party2:party2@tcp(example:3306)/party2")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	stats := db.Stats()
	if stats.MaxOpenConnections != DefaultMaxOpenConns {
		t.Errorf("stats.MaxOpenConnections = %d, want %d", stats.MaxOpenConnections, DefaultMaxOpenConns)
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
