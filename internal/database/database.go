package database

import (
	"database/sql"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Default connection pool configuration baseline values.
const (
	DefaultMaxOpenConns    = 25
	DefaultMaxIdleConns    = 25
	DefaultConnMaxLifetime = 5 * time.Minute
	DefaultConnMaxIdleTime = 1 * time.Minute
)

// Config holds database connection and connection pool parameters.
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultConfig returns a Config initialized with sensible baseline pool defaults.
func DefaultConfig(dsn string) Config {
	return Config{
		DSN:             dsn,
		MaxOpenConns:    DefaultMaxOpenConns,
		MaxIdleConns:    DefaultMaxIdleConns,
		ConnMaxLifetime: DefaultConnMaxLifetime,
		ConnMaxIdleTime: DefaultConnMaxIdleTime,
	}
}

// ConfigFromEnvironment loads database configuration from environment variables.
// Supported environment variables:
//   - PARTY2_DB_DSN: Connection DSN (required)
//   - PARTY2_DB_MAX_OPEN_CONNS: Maximum open connections (integer, default: 25)
//   - PARTY2_DB_MAX_IDLE_CONNS: Maximum idle connections in pool (integer, default: 25)
//   - PARTY2_DB_CONN_MAX_LIFETIME: Maximum connection lifetime (duration, e.g. "5m", default: 5m)
//   - PARTY2_DB_CONN_MAX_IDLE_TIME: Maximum connection idle duration (duration, e.g. "1m", default: 1m)
//
// If invalid values or negative numbers are provided, it falls back to safe defaults gracefully.
func ConfigFromEnvironment() (Config, error) {
	dsn := strings.TrimSpace(os.Getenv("PARTY2_DB_DSN"))
	if dsn == "" {
		return Config{}, errors.New("PARTY2_DB_DSN environment variable is required")
	}

	cfg := DefaultConfig(dsn)

	if val := strings.TrimSpace(os.Getenv("PARTY2_DB_MAX_OPEN_CONNS")); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.MaxOpenConns = n
		}
	}

	if val := strings.TrimSpace(os.Getenv("PARTY2_DB_MAX_IDLE_CONNS")); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.MaxIdleConns = n
		}
	}

	if val := strings.TrimSpace(os.Getenv("PARTY2_DB_CONN_MAX_LIFETIME")); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			cfg.ConnMaxLifetime = d
		}
	}

	if val := strings.TrimSpace(os.Getenv("PARTY2_DB_CONN_MAX_IDLE_TIME")); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			cfg.ConnMaxIdleTime = d
		}
	}

	// Ensure MaxIdleConns does not exceed MaxOpenConns if MaxOpenConns is positive.
	if cfg.MaxOpenConns > 0 && cfg.MaxIdleConns > cfg.MaxOpenConns {
		cfg.MaxIdleConns = cfg.MaxOpenConns
	}

	return cfg, nil
}

// Open opens a database connection with default connection pool settings.
func Open(dsn string) (*sql.DB, error) {
	return OpenWithConfig(DefaultConfig(dsn))
}

// OpenWithConfig opens a database connection using the provided configuration.
func OpenWithConfig(cfg Config) (*sql.DB, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errors.New("database DSN is required")
	}

	// Apply safe defaults for zero or negative values
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = DefaultMaxOpenConns
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = DefaultMaxIdleConns
	}
	if cfg.ConnMaxLifetime <= 0 {
		cfg.ConnMaxLifetime = DefaultConnMaxLifetime
	}
	if cfg.ConnMaxIdleTime <= 0 {
		cfg.ConnMaxIdleTime = DefaultConnMaxIdleTime
	}
	if cfg.MaxOpenConns > 0 && cfg.MaxIdleConns > cfg.MaxOpenConns {
		cfg.MaxIdleConns = cfg.MaxOpenConns
	}

	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	return db, nil
}

// OpenFromEnvironment opens a database connection reading configuration from environment variables.
func OpenFromEnvironment() (*sql.DB, error) {
	cfg, err := ConfigFromEnvironment()
	if err != nil {
		return nil, err
	}
	return OpenWithConfig(cfg)
}

func Ping(db *sql.DB) error {
	if db == nil {
		return errors.New("database is nil")
	}
	return db.Ping()
}
