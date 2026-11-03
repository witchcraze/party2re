package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/character"
)

func TestCharacterNameError(t *testing.T) {
	for _, input := range []error{
		nil, errors.New("unrelated error"),
		&mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'id' for key 'PRIMARY'"},
		&mysql.MySQLError{Number: 1048, Message: "Column 'name' cannot be null"},
		&mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'Hero' for key 'uq_characters_name'"},
	} {
		got := characterNameError(input)
		if input != nil && strings.Contains(input.Error(), "'uq_characters_name'") {
			if !errors.Is(got, character.ErrNameAlreadyTaken) {
				t.Fatalf("name conflict mapping=%v", got)
			}
		} else if got != input {
			t.Fatalf("unrelated error changed: %v -> %v", input, got)
		}
	}
}

func TestCharacterNameMigration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", "110_character_name_uniqueness.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// DELIMITER is a CLI directive; execute the same statements through the driver.
	sql := strings.ReplaceAll(strings.ReplaceAll(string(source), "DELIMITER //", ""), "DELIMITER ;", "")
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean_and_repeat", true: "duplicates_retained"}[duplicate], func(t *testing.T) {
			db, err := OpenFromEnvironment()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			ctx := context.Background()
			for _, statement := range []string{
				"CREATE TEMPORARY TABLE characters (id CHAR(32) PRIMARY KEY, name VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL, INDEX idx_characters_name (name))",
				"CREATE TEMPORARY TABLE schema_migrations (version VARCHAR(255) PRIMARY KEY)",
				"INSERT INTO characters VALUES ('one','Hero'),('two','hero'),('three','Héro')",
			} {
				if _, err := db.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			if duplicate {
				if _, err := db.ExecContext(ctx, "INSERT INTO characters VALUES ('duplicate','Hero')"); err != nil {
					t.Fatal(err)
				}
			}
			for repeat := 0; repeat < 2; repeat++ {
				var migrationErr error
				for _, statement := range strings.Split(sql, "//") {
					if strings.TrimSpace(statement) == "" {
						continue
					}
					if _, migrationErr = db.ExecContext(ctx, statement); migrationErr != nil {
						break
					}
				}
				if duplicate {
					if migrationErr == nil || !strings.Contains(migrationErr.Error(), "resolve the listed character IDs") {
						t.Fatalf("missing actionable duplicate preflight: %v", migrationErr)
					}
				} else if migrationErr != nil {
					t.Fatal(migrationErr)
				}
			}
			var count int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters").Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 3
			if duplicate {
				want = 4
			}
			if count != want {
				t.Fatalf("migration modified existing characters: count=%d, want %d", count, want)
			}
			var applied int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil || applied != map[bool]int{false: 1, true: 0}[duplicate] {
				t.Fatalf("migration history=%d err=%v", applied, err)
			}
			_, err = db.ExecContext(ctx, "INSERT INTO characters VALUES ('retry','Hero')")
			if duplicate && err != nil {
				t.Fatalf("preflight unexpectedly changed the original nonunique schema: %v", err)
			} else if !duplicate && !errors.Is(characterNameError(err), character.ErrNameAlreadyTaken) {
				t.Fatalf("unique constraint did not reject exact duplicate: %v", err)
			}
		})
	}
}
