package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockScanner struct {
	scanFn func(dest ...any) error
}

func (m *mockScanner) Scan(dest ...any) error {
	return m.scanFn(dest...)
}

func TestScanCharacterRow_ErrNoRowsMapsToNotFound(t *testing.T) {
	scanner := &mockScanner{
		scanFn: func(dest ...any) error {
			return sql.ErrNoRows
		},
	}

	_, err := scanCharacterRow(scanner)
	if !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("expected corecharacter.ErrNotFound, got %v", err)
	}
}

func TestScanCharacterRow_CustomError(t *testing.T) {
	customErr := errors.New("db connection lost")
	scanner := &mockScanner{
		scanFn: func(dest ...any) error {
			return customErr
		},
	}

	_, err := scanCharacterRow(scanner)
	if !errors.Is(err, customErr) {
		t.Fatalf("expected customErr, got %v", err)
	}
}

func TestScanCharacterRow_SuccessfulScan(t *testing.T) {
	scanner := &mockScanner{
		scanFn: func(dest ...any) error {
			if len(dest) != 24 {
				t.Fatalf("expected 24 scan destinations, got %d", len(dest))
			}
			*dest[0].(*string) = "char-1"
			*dest[1].(*string) = "player-1"
			*dest[2].(*string) = "Alice"
			*dest[3].(*string) = "hero"
			*dest[4].(*string) = "FEMALE"
			*dest[5].(*int) = 150
			*dest[6].(*int) = 50
			*dest[7].(*int) = 140
			*dest[8].(*int) = 45
			*dest[9].(*int) = 30
			*dest[10].(*int) = 25
			*dest[11].(*int) = 20
			*dest[12].(*int) = 5000
			*dest[13].(*int) = 15
			*dest[14].(*int) = 3200
			*dest[15].(*int) = 2
			*dest[16].(*int) = 5
			*dest[17].(*int) = 8
			*dest[18].(*bool) = true
			*dest[19].(*int) = 1
			*dest[20].(*int) = 2
			*dest[21].(*int) = 3
			*dest[22].(*int) = 4
			*dest[23].(*int) = 5
			return nil
		},
	}

	char, err := scanCharacterRow(scanner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if char.ID != "char-1" || char.PlayerID != "player-1" || char.Name != "Alice" {
		t.Errorf("unexpected identity: %+v", char)
	}
	if char.Stats.MaxHP != 150 || char.Stats.MaxMP != 50 || char.Stats.HP != 140 || char.Stats.MP != 45 {
		t.Errorf("unexpected stats: %+v", char.Stats)
	}
	if char.Money != 5000 || char.Level != 15 || char.Experience != 3200 {
		t.Errorf("unexpected progression: Level %d, Exp %d, Money %d", char.Level, char.Experience, char.Money)
	}
	if char.RebirthCount != 2 || char.SmallMedals != 5 || char.HelpCount != 8 {
		t.Errorf("unexpected medals/help/rebirth: Rebirth %d, Medals %d, Help %d", char.RebirthCount, char.SmallMedals, char.HelpCount)
	}
	if !char.OverLevel || char.OverDepot != 1 || char.OverMonster != 2 || char.OverFuture != 3 || char.OverFlea != 4 || char.OverStore != 5 {
		t.Errorf("unexpected limit break fields: OverLevel %v, OverDepot %d, OverMonster %d, OverFuture %d, OverFlea %d, OverStore %d",
			char.OverLevel, char.OverDepot, char.OverMonster, char.OverFuture, char.OverFlea, char.OverStore)
	}
}

func TestUpdateCharacterAtomically_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	player, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := corecharacter.New("Atomic Update Test")
	if err != nil {
		t.Fatal(err)
	}
	char.PlayerID = player.ID
	char.SmallMedals = 3
	char.HelpCount = 4

	if err := repo.Save(ctx, char); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 1. Update same values (0 rows affected in MySQL) -> should succeed
	if err := updateCharacterAtomically(ctx, db, char); err != nil {
		t.Fatalf("expected updateCharacterAtomically with unchanged values to succeed, got %v", err)
	}

	// 2. Update nonexistent character -> should return ErrNotFound
	charNonExistent := char
	charNonExistent.ID = "nonexistent_char_id"
	if err := updateCharacterAtomically(ctx, db, charNonExistent); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for nonexistent character, got %v", err)
	}
}
