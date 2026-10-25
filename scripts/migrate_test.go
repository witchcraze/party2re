package scripts_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/database"
)

type migrateFakeFixture struct {
	root       string
	binDir     string
	sqlLog     string
	env        []string
	migrations map[string]string
}

func setupMigrateFakeFixture(t *testing.T, migrations map[string]string) *migrateFakeFixture {
	t.Helper()
	root := t.TempDir()

	migrateScript, err := os.ReadFile("migrate.sh")
	if err != nil {
		t.Fatal(err)
	}
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "migrate.sh"), migrateScript, 0o755); err != nil {
		t.Fatal(err)
	}

	migrationsDir := filepath.Join(root, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range migrations {
		if err := os.WriteFile(filepath.Join(migrationsDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sqlLog := filepath.Join(root, "sql.log")
	stateDir := filepath.Join(root, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fakeDocker := `#!/usr/bin/env bash
set -euo pipefail

if [ "${1:-}" = "inspect" ]; then
    echo "${FAKE_HEALTH:-healthy}"
    exit "${FAKE_INSPECT_EXIT:-0}"
fi

if [ "${1:-}" = "compose" ] && [ "${2:-}" = "up" ]; then
    exit "${FAKE_UP_EXIT:-0}"
fi

if [ "${1:-}" = "compose" ] && [ "${2:-}" = "ps" ]; then
    echo "cid-mariadb"
    exit 0
fi

if [ "${1:-}" = "compose" ] && [ "${2:-}" = "exec" ]; then
    shift 2
    # Consume flags and service/binary args to inspect -e query or stdin
    has_e=0
    query=""
    while [ "$#" -gt 0 ]; do
        case "$1" in
            -e)
                has_e=1
                query="${2:-}"
                shift 2
                ;;
            *)
                shift
                ;;
        esac
    done

    if [ "$has_e" -eq 1 ]; then
        case "$query" in
            "SELECT 1")
                exit "${FAKE_PING_EXIT:-0}"
                ;;
            *"CREATE TABLE IF NOT EXISTS schema_migrations"*)
                echo "CREATE_TABLE" >> "$SQL_LOG"
                if [ "${FAKE_CREATE_EXIT:-0}" -ne 0 ]; then
                    echo "ERROR 1044 (42000): Access denied for user" >&2
                    exit "$FAKE_CREATE_EXIT"
                fi
                exit 0
                ;;
            *"SELECT COUNT(1) FROM schema_migrations"*)
                echo "COUNT:$query" >> "$SQL_LOG"
                if [ "${FAKE_COUNT_EXIT:-0}" -ne 0 ]; then
                    echo "ERROR 1142 (42000): SELECT command denied" >&2
                    exit "$FAKE_COUNT_EXIT"
                fi
                if [ -n "${FAKE_COUNT_OUT+x}" ]; then
                    printf '%s' "$FAKE_COUNT_OUT"
                    exit 0
                fi
                # Extract version from query
                version=$(printf '%s' "$query" | sed -n "s/.*version = '\([^']*\)'.*/\1/p")
                if [ -f "$STATE_DIR/$version" ]; then
                    echo "1"
                else
                    echo "0"
                fi
                exit 0
                ;;
            *"INSERT"*"INTO schema_migrations"*)
                echo "INSERT:$query" >> "$SQL_LOG"
                if [ "${FAKE_INSERT_EXIT:-0}" -ne 0 ]; then
                    echo "ERROR 1142 (42000): INSERT command denied" >&2
                    exit "$FAKE_INSERT_EXIT"
                fi
                version=$(printf '%s' "$query" | sed -n "s/.*VALUES ('\([^']*\)').*/\1/p")
                if [ -n "$version" ]; then
                    if [ -f "$STATE_DIR/$version" ] && [[ "$query" != *"IGNORE"* ]] && [[ "$query" != *"ON DUPLICATE KEY"* ]]; then
                        echo "ERROR 1062 (23000): Duplicate entry '$version' for key 'PRIMARY'" >&2
                        exit 1
                    fi
                    touch "$STATE_DIR/$version"
                fi
                exit 0
                ;;
            *)
                echo "QUERY:$query" >> "$SQL_LOG"
                exit 0
                ;;
        esac
    else
        sql_in=$(cat)
        echo "APPLY_SQL:$sql_in" >> "$SQL_LOG"
        if [ "${FAKE_APPLY_EXIT:-0}" -ne 0 ]; then
            echo "ERROR 1064 (42000): You have an error in your SQL syntax" >&2
            exit "$FAKE_APPLY_EXIT"
        fi
        # Simulate self-registering SQL if it contains INSERT ... schema_migrations
        if [[ "$sql_in" == *"schema_migrations"* ]]; then
            version=$(printf '%s' "$sql_in" | sed -n "s/.*VALUES ('\([^']*\)').*/\1/p")
            if [ -n "$version" ]; then
                touch "$STATE_DIR/$version"
            fi
        fi
        exit 0
    fi
fi

exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(fakeDocker), 0o755); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"PATH=" + binDir + ":" + os.Getenv("PATH"),
		"SQL_LOG=" + sqlLog,
		"STATE_DIR=" + stateDir,
	}

	return &migrateFakeFixture{
		root:       root,
		binDir:     binDir,
		sqlLog:     sqlLog,
		env:        env,
		migrations: migrations,
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMigrate_FakeFaultInjection(t *testing.T) {
	defaultMigrations := map[string]string{
		"001_test.sql": "CREATE TABLE test_one (id INT PRIMARY KEY);\nINSERT IGNORE INTO schema_migrations (version) VALUES ('001_test');\n",
	}

	t.Run("stops when schema_migrations table creation fails", func(t *testing.T) {
		fix := setupMigrateFakeFixture(t, defaultMigrations)
		out, code := runCmd(t, fix.root, append(fix.env, "FAKE_CREATE_EXIT=1"), "bash", "./scripts/migrate.sh")
		if code == 0 {
			t.Fatalf("expected non-zero exit when table creation fails, got 0:\n%s", out)
		}
		if strings.Contains(out, "All migrations are up to date") {
			t.Fatalf("unexpected success message on failure:\n%s", out)
		}
		log := readLog(t, fix.sqlLog)
		if strings.Contains(log, "APPLY_SQL:") || strings.Contains(log, "COUNT:") {
			t.Fatalf("migration continued after table creation failure:\n%s", log)
		}
	})

	t.Run("stops when applied-history COUNT query fails", func(t *testing.T) {
		fix := setupMigrateFakeFixture(t, defaultMigrations)
		out, code := runCmd(t, fix.root, append(fix.env, "FAKE_COUNT_EXIT=1"), "bash", "./scripts/migrate.sh")
		if code == 0 {
			t.Fatalf("expected non-zero exit when COUNT query fails, got 0:\n%s", out)
		}
		if strings.Contains(out, "All migrations are up to date") {
			t.Fatalf("unexpected success message on failure:\n%s", out)
		}
		log := readLog(t, fix.sqlLog)
		if strings.Contains(log, "APPLY_SQL:") {
			t.Fatalf("migration SQL was executed despite COUNT failure:\n%s", log)
		}
	})

	for _, badOut := range []struct {
		name string
		val  string
	}{
		{name: "empty COUNT output", val: ""},
		{name: "whitespace COUNT output", val: "   \n\t "},
		{name: "non-numeric COUNT output", val: "ERROR"},
		{name: "unexpected numeric COUNT output", val: "2"},
	} {
		t.Run("fails explicitly on "+badOut.name, func(t *testing.T) {
			fix := setupMigrateFakeFixture(t, defaultMigrations)
			out, code := runCmd(t, fix.root, append(fix.env, "FAKE_COUNT_OUT="+badOut.val), "bash", "./scripts/migrate.sh")
			if code == 0 {
				t.Fatalf("expected non-zero exit on %s (%q), got 0:\n%s", badOut.name, badOut.val, out)
			}
			if strings.Contains(out, "All migrations are up to date") {
				t.Fatalf("unexpected success message on %s:\n%s", badOut.name, out)
			}
			log := readLog(t, fix.sqlLog)
			if strings.Contains(log, "APPLY_SQL:") {
				t.Fatalf("migration SQL was executed despite %s:\n%s", badOut.name, log)
			}
		})
	}

	t.Run("propagates failure when migration SQL execution fails", func(t *testing.T) {
		fix := setupMigrateFakeFixture(t, map[string]string{
			"001_first.sql":  "CREATE TABLE t1 (id INT PRIMARY KEY);\n",
			"002_second.sql": "CREATE TABLE t2 (id INT PRIMARY KEY);\n",
		})
		out, code := runCmd(t, fix.root, append(fix.env, "FAKE_APPLY_EXIT=1"), "bash", "./scripts/migrate.sh")
		if code == 0 {
			t.Fatalf("expected non-zero exit when SQL execution fails, got 0:\n%s", out)
		}
		if strings.Contains(out, "All migrations are up to date") {
			t.Fatalf("unexpected success message on SQL failure:\n%s", out)
		}
		log := readLog(t, fix.sqlLog)
		if strings.Contains(log, "INSERT:") {
			t.Fatalf("history INSERT ran after failed SQL execution:\n%s", log)
		}
		if strings.Contains(log, "002_second") {
			t.Fatalf("subsequent migration ran after failed SQL execution:\n%s", log)
		}
	})

	t.Run("stops when history write fails due to DB/permission error", func(t *testing.T) {
		fix := setupMigrateFakeFixture(t, defaultMigrations)
		out, code := runCmd(t, fix.root, append(fix.env, "FAKE_INSERT_EXIT=1"), "bash", "./scripts/migrate.sh")
		if code == 0 {
			t.Fatalf("expected non-zero exit when history INSERT fails, got 0:\n%s", out)
		}
		if strings.Contains(out, "All migrations are up to date") {
			t.Fatalf("unexpected success message on history INSERT failure:\n%s", out)
		}
	})

	t.Run("healthy contrast applies unapplied migration and skips on second run", func(t *testing.T) {
		fix := setupMigrateFakeFixture(t, defaultMigrations)
		out1, code1 := runCmd(t, fix.root, fix.env, "bash", "./scripts/migrate.sh")
		if code1 != 0 {
			t.Fatalf("expected exit 0 on healthy run, got %d:\n%s", code1, out1)
		}
		if !strings.Contains(out1, "Applying migrations/001_test.sql") {
			t.Fatalf("expected migration application log, got:\n%s", out1)
		}
		if !strings.Contains(out1, "All migrations are up to date") {
			t.Fatalf("expected completion message, got:\n%s", out1)
		}

		// Clear SQL log and run a second time: should skip
		_ = os.Remove(fix.sqlLog)
		out2, code2 := runCmd(t, fix.root, fix.env, "bash", "./scripts/migrate.sh")
		if code2 != 0 {
			t.Fatalf("expected exit 0 on second run, got %d:\n%s", code2, out2)
		}
		if strings.Contains(out2, "Applying migrations/001_test.sql") {
			t.Fatalf("expected already-applied migration to be skipped on second run:\n%s", out2)
		}
		log2 := readLog(t, fix.sqlLog)
		if strings.Contains(log2, "APPLY_SQL:") {
			t.Fatalf("SQL should not be applied on second run:\n%s", log2)
		}
	})
}

func TestMigrate_RealIsolatedDB(t *testing.T) {
	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		if os.Getenv("PARTY2_DB_DSN") == "" {
			t.Skip("PARTY2_TEST_DB_ADMIN_DSN is not configured")
		}
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated migration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.Close()
	})

	dbName := fmt.Sprintf("party2_migrate_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+dbName+"`")
	})
	if _, err := admin.ExecContext(ctx, "GRANT ALL PRIVILEGES ON `"+dbName+"`.* TO 'party2'@'%'"); err != nil {
		t.Fatal(err)
	}

	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = dbName
	isoDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = isoDB.Close()
	})

	// Copy scripts/migrate.sh into root; do not copy compose.yaml into root
	// so docker compose does not treat root/migrations as a changed volume mount for mariadb.
	root := t.TempDir()
	migrateScript, err := os.ReadFile("migrate.sh")
	if err != nil {
		t.Fatal(err)
	}
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "migrate.sh"), migrateScript, 0o755); err != nil {
		t.Fatal(err)
	}

	composePath, err := filepath.Abs(filepath.Join("..", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	migrationsDir := filepath.Join(root, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 001 self-registers via INSERT IGNORE (like most migrations), 002 relies on runner registration (like 040/049)
	mig001 := `CREATE TABLE mig_items (
    id INT PRIMARY KEY,
    name VARCHAR(64) NOT NULL
);
INSERT INTO mig_items (id, name) VALUES (1, 'sword');
INSERT IGNORE INTO schema_migrations (version) VALUES ('001_self_registered');
`
	if err := os.WriteFile(filepath.Join(migrationsDir, "001_self_registered.sql"), []byte(mig001), 0o644); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"COMPOSE_FILE=" + composePath,
		"PARTY2_DB_NAME=" + dbName,
	}

	// 1. Apply single unapplied migration (001_self_registered)
	out1, code1 := runCmd(t, root, env, "bash", "./scripts/migrate.sh")
	if code1 != 0 {
		t.Fatalf("first migration run failed (code %d):\n%s", code1, out1)
	}
	if !strings.Contains(out1, "Applying migrations/001_self_registered.sql") {
		t.Fatalf("expected 001_self_registered.sql to be applied:\n%s", out1)
	}

	var count int
	if err := isoDB.QueryRowContext(ctx, "SELECT COUNT(1) FROM schema_migrations WHERE version = '001_self_registered'").Scan(&count); err != nil {
		t.Fatalf("failed to query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected version 001_self_registered count = 1, got %d", count)
	}

	// 2. Add a second migration (without self-registration) and run: 001 should be skipped, 002 applied
	// Notice that if 001 were re-executed, its non-idempotent CREATE TABLE / INSERT would fail!
	mig002 := `CREATE TABLE mig_extra (
    id INT PRIMARY KEY
);
`
	if err := os.WriteFile(filepath.Join(migrationsDir, "002_runner_registered.sql"), []byte(mig002), 0o644); err != nil {
		t.Fatal(err)
	}

	out2, code2 := runCmd(t, root, env, "bash", "./scripts/migrate.sh")
	if code2 != 0 {
		t.Fatalf("second migration run failed (code %d):\n%s", code2, out2)
	}
	if strings.Contains(out2, "Applying migrations/001_self_registered.sql") {
		t.Fatalf("001_self_registered.sql should have been skipped on second run:\n%s", out2)
	}
	if !strings.Contains(out2, "Applying migrations/002_runner_registered.sql") {
		t.Fatalf("expected 002_runner_registered.sql to be applied:\n%s", out2)
	}

	if err := isoDB.QueryRowContext(ctx, "SELECT COUNT(1) FROM schema_migrations WHERE version = '002_runner_registered'").Scan(&count); err != nil {
		t.Fatalf("failed to query schema_migrations for 002: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected version 002_runner_registered count = 1, got %d", count)
	}

	// 3. Re-run with no changes: both 001 and 002 skipped, exits 0
	out3, code3 := runCmd(t, root, env, "bash", "./scripts/migrate.sh")
	if code3 != 0 {
		t.Fatalf("third migration run failed (code %d):\n%s", code3, out3)
	}
	if strings.Contains(out3, "-> Applying ") {
		t.Fatalf("expected no migrations to be applied on no-op re-run:\n%s", out3)
	}
	if !strings.Contains(out3, "All migrations are up to date") {
		t.Fatalf("expected up-to-date message on third run:\n%s", out3)
	}

	// 4. Add a mid-SQL failing migration (003_partial_failure): first statement creates table, second statement fails
	mig003 := `CREATE TABLE mig_partial (id INT PRIMARY KEY);
INVALID SQL STATEMENT THAT FAILS;
`
	if err := os.WriteFile(filepath.Join(migrationsDir, "003_partial_failure.sql"), []byte(mig003), 0o644); err != nil {
		t.Fatal(err)
	}

	out4, code4 := runCmd(t, root, env, "bash", "./scripts/migrate.sh")
	if code4 == 0 {
		t.Fatalf("expected non-zero exit when migration SQL fails midway, got 0:\n%s", out4)
	}
	if strings.Contains(out4, "All migrations are up to date") {
		t.Fatalf("unexpected success message when migration fails midway:\n%s", out4)
	}
	if err := isoDB.QueryRowContext(ctx, "SELECT COUNT(1) FROM schema_migrations WHERE version = '003_partial_failure'").Scan(&count); err != nil {
		t.Fatalf("failed to query schema_migrations for 003: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed migration 003_partial_failure must not be recorded as applied, got count = %d", count)
	}

	// 5. Revoke INSERT privilege on schema_migrations so SQL's own INSERT and runner's INSERT fail on a new migration
	mig004 := `CREATE TABLE mig_no_insert_priv (id INT PRIMARY KEY);
`
	if err := os.Remove(filepath.Join(migrationsDir, "003_partial_failure.sql")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(migrationsDir, "004_priv_failure.sql"), []byte(mig004), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "REVOKE ALL PRIVILEGES ON `"+dbName+"`.* FROM 'party2'@'%'"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "GRANT SELECT, CREATE ON `"+dbName+"`.* TO 'party2'@'%'"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "FLUSH PRIVILEGES"); err != nil {
		t.Fatal(err)
	}

	out5, code5 := runCmd(t, root, env, "bash", "./scripts/migrate.sh")
	if code5 == 0 {
		t.Fatalf("expected non-zero exit when party2 lacks INSERT privilege on schema_migrations, got 0:\n%s", out5)
	}
	if strings.Contains(out5, "All migrations are up to date") {
		t.Fatalf("unexpected success message when history INSERT privilege is missing:\n%s", out5)
	}
}
