package database_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/replay"
)

func TestReplayArchiveMigrationPreservesUnknownRows(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	// Connection-local tables shadow shared tables, so migration/pruning tests cannot affect other packages.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "CREATE TEMPORARY TABLE schema_migrations (version VARCHAR(255) PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	apply := func(name string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		script := strings.Replace(string(data), "CREATE TABLE IF NOT EXISTS battle_replays", "CREATE TEMPORARY TABLE battle_replays", 1)
		for _, statement := range strings.Split(script, ";") {
			if strings.TrimSpace(statement) != "" {
				if _, err := db.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	apply("027_battle_replays.sql")
	if _, err := db.ExecContext(ctx, `INSERT INTO battle_replays
		(id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name, outcome, initial_participants_json, turn_logs_json)
		VALUES ('historical', 'adventure', 'recipient-guess', 'Owner', 'other', 'Opponent', 'win', '[{"id":"recipient-guess","attack":123}]', '[{"turn":1,"message":"preserved"}]')`); err != nil {
		t.Fatal(err)
	}
	apply("102_replay_archive_provenance.sql")
	repo, err := database.NewReplayRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.FindByID(ctx, "historical")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Provenance != "" || stored.ArchiveRecipientID != "" || stored.InitialParticipants[0].Attack != 123 || stored.TurnLogs[0].Message != "preserved" {
		t.Fatalf("migration guessed ownership or changed payload: %+v", stored)
	}
	if _, err := repo.FindPersonalRecipient(ctx, "historical"); !errors.Is(err, replay.ErrReplayNotFound) {
		t.Fatal(err)
	}
	rows, err := repo.FindByCharacter(ctx, "recipient-guess", "", 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("historical headers exposed: %+v %v", rows, err)
	}
}
