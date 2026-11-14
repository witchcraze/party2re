package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func characterFactoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := playerIsolatedDB(t)
	source, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	for _, table := range []string{"characters", "character_depots"} {
		var name, ddl string
		if err := source.QueryRow("SHOW CREATE TABLE `"+table+"`").Scan(&name, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func assertFactoryRows(t *testing.T, ctx context.Context, db *sql.DB, want int) {
	t.Helper()
	for _, table := range []string{"players", "characters", "character_depots"} {
		var got int
		if err := ExecutorFromContext(ctx, db).QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}
}

func TestCharacterFactoryRequiresDatabase(t *testing.T) {
	c, err := CreateTestCharacterWithFunds(context.Background(), nil, "NoDB", 1)
	if err == nil || c.ID != "" || c.PlayerID != "" {
		t.Fatalf("nil DB returned character=%+v error=%v", c, err)
	}
}

func TestCharacterFactoryRollsBackSaveFailures(t *testing.T) {
	for _, table := range []string{"characters", "character_depots"} {
		t.Run(table, func(t *testing.T) {
			db := characterFactoryDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			sentinel, err := CreateTestCharacterWithFunds(ctx, db, "Sentinel", 123)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_factory BEFORE INSERT ON `+table+`
				FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected factory failure'`); err != nil {
				t.Fatal(err)
			}
			for _, ambient := range []bool{false, true} {
				create := func(callCtx context.Context) error {
					c, err := CreateTestCharacterWithFunds(callCtx, db, "Failure", 7777)
					if c.ID != "" || c.PlayerID != "" {
						t.Errorf("failed factory returned partial character: %+v", c)
					}
					return err
				}
				if ambient {
					err = RunInTx(ctx, db, create)
				} else {
					err = create(ctx)
				}
				var injected *mysql.MySQLError
				if !errors.As(err, &injected) || injected.Number != 1644 || injected.Message != "injected factory failure" {
					t.Fatalf("ambient=%v: got %v, want original injected SQL error", ambient, err)
				}
				assertFactoryRows(t, ctx, db, 1)
				var owner string
				var money int
				if err := db.QueryRowContext(ctx, `SELECT c.player_id, c.money FROM characters c
					JOIN players p ON p.id = c.player_id
					JOIN character_depots d ON d.character_id = c.id
					WHERE c.id = ?`, sentinel.ID).Scan(&owner, &money); err != nil {
					t.Fatal(err)
				}
				if owner != sentinel.PlayerID || money != sentinel.Money {
					t.Fatal("unrelated sentinel changed")
				}
			}
		})
	}
}

func TestCharacterFactoryPreservesNameRetries(t *testing.T) {
	db := characterFactoryDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, err := CreateTestCharacterWithFunds(ctx, db, "Factory", 123)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateTestCharacterWithFunds(ctx, db, a.Name, 7777)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "Factory" || b.Name == a.Name || b.Money != 7777 || a.PlayerID == b.PlayerID {
		t.Fatalf("unexpected fixtures: %+v, %+v", a, b)
	}
	assertFactoryRows(t, ctx, db, 2)
	// Session variables survive rollback, so they independently count failed attempts.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER exhaust_factory_names BEFORE INSERT ON characters
		FOR EACH ROW BEGIN
		SET @fixture_attempts = COALESCE(@fixture_attempts, 0) + 1;
		SIGNAL SQLSTATE '23000' SET MYSQL_ERRNO = 1062, MESSAGE_TEXT = 'Duplicate entry for key ''uq_characters_name''';
		END`); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTestCharacterWithFunds(ctx, db, "Retry", 1); !errors.Is(err, character.ErrNameAlreadyTaken) {
		t.Fatalf("exhausted retries = %v, want name conflict", err)
	}
	var attempts int
	if err := db.QueryRowContext(ctx, `SELECT @fixture_attempts`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("save attempts = %d, want 3", attempts)
	}
	assertFactoryRows(t, ctx, db, 2)
}

func TestCharacterFactoryPreservesAmbientTransaction(t *testing.T) {
	for _, mode := range []string{"rollback", "commit"} {
		t.Run(mode, func(t *testing.T) {
			commit := mode == "commit"
			db := characterFactoryDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			rollback := errors.New("outer rollback")
			var c corecharacter.Character
			err := RunInTx(ctx, db, func(txCtx context.Context) error {
				var err error
				c, err = CreateTestCharacterWithFunds(txCtx, db, "Ambient", 456)
				if err != nil {
					return err
				}
				assertFactoryRows(t, txCtx, db, 1)
				if !commit {
					return rollback
				}
				return nil
			})
			wantRows := 0
			if commit {
				wantRows = 1
				if err != nil {
					t.Fatal(err)
				}
				var money int
				if err := db.QueryRowContext(ctx, `SELECT money FROM characters WHERE id = ?`, c.ID).Scan(&money); err != nil || money != 456 {
					t.Fatalf("committed character money=%d error=%v", money, err)
				}
			} else if !errors.Is(err, rollback) {
				t.Fatalf("outer rollback = %v", err)
			}
			assertFactoryRows(t, ctx, db, wantRows)
		})
	}
}
