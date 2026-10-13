package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobPointsMigrationBackfillsOnlyKnownSP(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	// Connection-local shadows isolate the backfill from other test packages.
	for _, sql := range []string{
		"CREATE TEMPORARY TABLE characters (id CHAR(32), job_id VARCHAR(64), sp INT, old_job_id VARCHAR(64), old_sp INT)",
		"CREATE TEMPORARY TABLE character_job_masteries (character_id CHAR(32), job_id VARCHAR(64), mastered_sp INT)",
		"CREATE TEMPORARY TABLE character_job_points (character_id CHAR(32), job_id VARCHAR(64), earned_sp INT, PRIMARY KEY(character_id,job_id))",
		"CREATE TEMPORARY TABLE schema_migrations (version VARCHAR(255) PRIMARY KEY)",
		"INSERT INTO characters VALUES ('a','job-01',20,'job-02',0)",
		"INSERT INTO character_job_masteries VALUES ('a','job-01',80),('a','job-03',99)",
	} {
		if _, err := db.ExecContext(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", "103_character_job_points.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// DDL is exercised by make db-migrate; run its actual backfill twice here.
	start := strings.Index(string(source), "-- Preserve known")
	if start < 0 {
		t.Fatal("migration backfill missing")
	}
	apply := func() {
		t.Helper()
		for _, sql := range strings.Split(string(source)[start:], ";") {
			if strings.TrimSpace(sql) == "" {
				continue
			}
			if _, err := db.ExecContext(ctx, sql); err != nil {
				t.Fatal(err, sql)
			}
		}
	}
	apply()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_job_points").Scan(&count); err != nil || count != 3 {
		t.Fatal(count, err)
	}
	for id, want := range map[string]int{"job-01": 20, "job-02": 0, "job-03": 99} {
		var got int
		if err := db.QueryRowContext(ctx, "SELECT earned_sp FROM character_job_points WHERE character_id='a' AND job_id=?", id).Scan(&got); err != nil || got != want {
			t.Fatal(id, got, err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE character_job_points SET earned_sp=7 WHERE job_id='job-03'"); err != nil {
		t.Fatal(err)
	}
	apply()
	var got int
	if err := db.QueryRowContext(ctx, "SELECT earned_sp FROM character_job_points WHERE job_id='job-03'").Scan(&got); err != nil || got != 7 {
		t.Fatal(got, err)
	}
}
