package character_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

type faultInjectingCharacterRepo struct {
	*database.CharacterRepository
	db        *sql.DB
	failDepot bool
}

func (r *faultInjectingCharacterRepo) Save(ctx context.Context, value corecharacter.Character) error {
	if !r.failDepot {
		return r.CharacterRepository.Save(ctx, value)
	}

	color := value.Color
	if color == "" {
		color = corecharacter.DefaultColor
	}

	// Execute the REAL character insert using ambient transaction from ctx
	_, err := database.ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO characters
			(id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, sp, job_level, old_job_id, old_sp, job_memory_job_id, job_memory_sp, job_memory_old_job_id, job_memory_old_sp, job_memory_kind, small_medals, help_count, hero_count, pvp_wins, casino_wins, monster_kills, mao_count, orb, tired, over_level, over_depot, over_monster, over_future, over_flea, over_store, color, deposit, crystal, wea_seal, wea_name, arm_name, pending_wake)
		VALUES (
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?
		)
	`, value.ID, value.PlayerID, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP,
		value.Stats.HP, value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility,
		value.Money, value.Level, value.Experience, value.SP, value.JobLevel, value.OldJobID, value.OldSP,
		"", 0, "", 0, string(corecharacter.JobMemoryKindPersistent), value.SmallMedals, value.HelpCount, value.HeroCount, value.PvPWins, value.CasinoWins,
		value.MonsterKills, value.MaoCount,
		value.Orb, value.Tired, value.OverLevel, value.OverDepot, value.OverMonster, value.OverFuture, value.OverFlea, value.OverStore, color,
		value.Deposit, value.Crystal, value.WeaponSeal, value.WeaponCustomName, value.ArmorCustomName, value.PendingWake)
	if err != nil {
		return err
	}

	// Simulate failure of the 2nd insert (initial depot auto-initialization)
	return errors.New("simulated initial depot persistence failure")
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
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID)
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	faultRepo := &faultInjectingCharacterRepo{
		CharacterRepository: charRepo,
		db:                  db,
		failDepot:           true,
	}

	service, err := character.NewService(
		faultRepo,
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	charName := fmt.Sprintf("F_%d", time.Now().UnixNano()%100000)

	// 1. Attempt creation with depot failure injected (2nd INSERT failure)
	_, err = service.CreateWithOptions(ctx, player.ID, charName, character.CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
	if err == nil {
		t.Fatal("expected error due to simulated depot persistence failure, got nil")
	}

	// 2. Verify rollback: Character row must NOT exist in DB
	charCount, err := countCharactersByName(ctx, db, charName)
	if err != nil {
		t.Fatalf("failed to query characters: %v", err)
	}
	if charCount != 0 {
		t.Fatalf("character leaked into database on depot failure! count = %d, want 0", charCount)
	}

	// 3. Retry after failure with healthy repo: must succeed cleanly without duplicate/orphan state
	faultRepo.failDepot = false
	retryChar, err := service.CreateWithOptions(ctx, player.ID, charName, character.CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
	if err != nil {
		t.Fatalf("retry after failure failed: %v", err)
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
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

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
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID)
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
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID)
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
