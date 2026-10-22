package character_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

func characterIsolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated character creation tests")
	}

	ctx := context.Background()
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Errorf("failed to close source DB: %v", err)
		}
	})

	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Errorf("failed to close admin DB: %v", err)
		}
	})

	name := "party2_char_test_" + id.New()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("failed to drop isolated database %s: %v", name, err)
		}
	})

	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close isolated DB: %v", err)
		}
	})

	for _, table := range []string{"players", "characters", "character_depots", "depot_items"} {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}

	return db
}

func countCharactersByName(ctx context.Context, db *sql.DB, name string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE name = ?", name).Scan(&count)
	return count, err
}

func countDepotsByCharacterID(ctx context.Context, db *sql.DB, charID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_depots WHERE character_id = ?", charID).Scan(&count)
	return count, err
}

func TestCharacterCreationRollbackOnDepotFailure_Integration(t *testing.T) {
	db := characterIsolatedDB(t)
	ctx := context.Background()

	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID); err != nil {
			t.Errorf("failed to cleanup character_depots: %v", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID); err != nil {
			t.Errorf("failed to cleanup characters: %v", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID); err != nil {
			t.Errorf("failed to cleanup players: %v", err)
		}
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	service, err := character.NewService(
		charRepo,
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	charName := fmt.Sprintf("F_%05d", time.Now().UnixNano()%100000)

	// Inject a real database-level failure on character_depots INSERT statement.
	// This ensures the real CharacterRepository.Save executes characters INSERT successfully,
	// but the second INSERT (character_depots) fails at the real MariaDB SQL level.
	_, err = db.ExecContext(ctx, "CREATE TRIGGER trg_fail_depot BEFORE INSERT ON character_depots FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'simulated depot insert failure'")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Attempt creation: characters INSERT succeeds, but character_depots INSERT fails at MariaDB level
	_, err = service.CreateWithOptions(ctx, player.ID, charName, character.CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
	if err == nil {
		t.Fatal("expected error due to real database depot persistence failure, got nil")
	}

	// 2. Verify rollback: Neither character row nor depot row must exist in DB
	charCount, err := countCharactersByName(ctx, db, charName)
	if err != nil {
		t.Fatalf("failed to query characters: %v", err)
	}
	if charCount != 0 {
		t.Fatalf("character leaked into database on depot failure! count = %d, want 0", charCount)
	}

	var totalDepots int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_depots").Scan(&totalDepots); err != nil {
		t.Fatalf("failed to query total depots: %v", err)
	}
	if totalDepots != 0 {
		t.Fatalf("depot leaked into database on depot failure! count = %d, want 0", totalDepots)
	}

	// 3. Remove DB fault injection and retry: must succeed cleanly without duplicate/orphan state
	if _, err := db.ExecContext(ctx, "DROP TRIGGER trg_fail_depot"); err != nil {
		t.Fatal(err)
	}

	retryChar, err := service.CreateWithOptions(ctx, player.ID, charName, character.CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
	if err != nil {
		t.Fatalf("retry after resolving depot failure failed: %v", err)
	}

	if retryChar.PlayerID != player.ID || retryChar.Name != charName {
		t.Fatalf("unexpected character returned: %+v", retryChar)
	}

	charCount, err = countCharactersByName(ctx, db, charName)
	if err != nil || charCount != 1 {
		t.Fatalf("expected exactly 1 character after retry, got count=%d, err=%v", charCount, err)
	}

	depotCount, err := countDepotsByCharacterID(ctx, db, retryChar.ID)
	if err != nil || depotCount != 1 {
		t.Fatalf("expected exactly 1 depot after retry, got count=%d, err=%v", depotCount, err)
	}
}

func TestCharacterCreationRollbackOnFirstInsertFailure_Integration(t *testing.T) {
	db := characterIsolatedDB(t)
	ctx := context.Background()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	service, err := character.NewService(
		charRepo,
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	charName := fmt.Sprintf("B_%05d", time.Now().UnixNano()%100000)
	// Non-existent player ID violates foreign key constraint on characters.player_id
	nonExistentPlayerID := "p_nonexistent_99999"

	_, err = service.CreateWithOptions(ctx, nonExistentPlayerID, charName, character.CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
	if err == nil {
		t.Fatal("expected foreign key error on character creation with non-existent player ID, got nil")
	}

	charCount, err := countCharactersByName(ctx, db, charName)
	if err != nil {
		t.Fatalf("failed to query characters: %v", err)
	}
	if charCount != 0 {
		t.Fatalf("character created despite first INSERT foreign key failure! count = %d, want 0", charCount)
	}
}

func TestCharacterCreationRollbackOnAmbientTransaction_Integration(t *testing.T) {
	db := characterIsolatedDB(t)
	ctx := context.Background()

	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID); err != nil {
			t.Errorf("failed to cleanup character_depots: %v", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID); err != nil {
			t.Errorf("failed to cleanup characters: %v", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID); err != nil {
			t.Errorf("failed to cleanup players: %v", err)
		}
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	service, err := character.NewService(
		charRepo,
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	charName := fmt.Sprintf("A_%05d", time.Now().UnixNano()%100000)
	var createdCharID string

	// Caller opens ambient transaction and rolls back
	errCallerRollback := errors.New("intentional caller rollback")
	err = database.RunInTx(ctx, db, func(txCtx context.Context) error {
		created, err := service.CreateWithOptions(txCtx, player.ID, charName, character.CreationOptions{
			JobID:  "job-01",
			Gender: "m",
		})
		if err != nil {
			return err
		}
		createdCharID = created.ID
		return errCallerRollback
	})

	if !errors.Is(err, errCallerRollback) {
		t.Fatalf("expected caller rollback error, got: %v", err)
	}

	// Verify both character and depot were rolled back with ambient transaction
	charCount, err := countCharactersByName(ctx, db, charName)
	if err != nil {
		t.Fatalf("failed to query characters: %v", err)
	}
	if charCount != 0 {
		t.Fatalf("character committed despite ambient transaction rollback! count = %d, want 0", charCount)
	}

	if createdCharID != "" {
		depotCount, err := countDepotsByCharacterID(ctx, db, createdCharID)
		if err != nil {
			t.Fatalf("failed to query depots: %v", err)
		}
		if depotCount != 0 {
			t.Fatalf("depot committed despite ambient transaction rollback! count = %d, want 0", depotCount)
		}
	}
}

func TestCharacterCreationNormalSuccess_Integration(t *testing.T) {
	db := characterIsolatedDB(t)
	ctx := context.Background()

	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID); err != nil {
			t.Errorf("failed to cleanup character_depots: %v", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID); err != nil {
			t.Errorf("failed to cleanup characters: %v", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID); err != nil {
			t.Errorf("failed to cleanup players: %v", err)
		}
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	service, err := character.NewService(
		charRepo,
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	charName := fmt.Sprintf("N_%05d", time.Now().UnixNano()%100000)

	created, err := service.CreateWithOptions(ctx, player.ID, charName, character.CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions() failed: %v", err)
	}

	// 1. Verify character properties
	if created.PlayerID != player.ID {
		t.Errorf("created.PlayerID = %q, want %q", created.PlayerID, player.ID)
	}
	if created.Name != charName {
		t.Errorf("created.Name = %q, want %q", created.Name, charName)
	}
	if created.JobID != "job-01" {
		t.Errorf("created.JobID = %q, want job-01", created.JobID)
	}
	if created.Gender != "m" {
		t.Errorf("created.Gender = %q, want m", created.Gender)
	}
	if created.Money != corecharacter.InitialMoney {
		t.Errorf("created.Money = %d, want %d (200G)", created.Money, corecharacter.InitialMoney)
	}
	if created.Stats.MaxHP <= 0 || created.Stats.HP <= 0 {
		t.Errorf("created initial HP must be positive: %+v", created.Stats)
	}

	// 2. Verify DB persistence
	persistedChar, err := charRepo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if persistedChar.PlayerID != player.ID || persistedChar.Name != charName {
		t.Fatalf("persisted character mismatch: %+v", persistedChar)
	}

	persistedDepot, err := depotRepo.FindByCharacterID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	expectedCap := depot.CalculateCapacity(persistedChar.JobLevel, 0, persistedChar.OverDepot)
	if persistedDepot.Capacity != expectedCap {
		t.Errorf("persistedDepot.Capacity = %d, want %d", persistedDepot.Capacity, expectedCap)
	}
	if persistedDepot.CharacterID != created.ID {
		t.Errorf("persistedDepot.CharacterID = %q, want %q", persistedDepot.CharacterID, created.ID)
	}
}
