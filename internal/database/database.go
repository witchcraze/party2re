package database

import (
	"database/sql"
	"errors"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const defaultDSN = "party2:party2@tcp(localhost:3306)/party2?parseTime=true"

func OpenFromEnvironment() (*sql.DB, error) {
	dsn := os.Getenv("PARTY2_DB_DSN")
	if dsn == "" {
		dsn = defaultDSN
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	return db, nil
}

func Ping(db *sql.DB) error {
	if db == nil {
		return errors.New("database is nil")
	}
	return db.Ping()
}
