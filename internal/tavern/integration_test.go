package tavern_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/tavern"
)

type failingTavernSQLRepo struct {
	*database.TavernRepository
	getErr error
}

func (r *failingTavernSQLRepo) GetCharacterStatus(ctx context.Context, id string) (tavern.TavernCharacterStatus, error) {
	if r.getErr != nil {
		return tavern.TavernCharacterStatus{}, r.getErr
	}
	return r.TavernRepository.GetCharacterStatus(ctx, id)
}

func TestTavern_RealDatabase_ErrorPropagation(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	realTavernRepo, err := database.NewTavernRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	catalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}

	wrappedRepo := &failingTavernSQLRepo{TavernRepository: realTavernRepo}
	svc, err := tavern.NewService(catalog, wrappedRepo, charRepo, txProvider)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "TavernSQLTester")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM tavern_deliveries WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM tavern_character_status WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM character_depots WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID)
	})

	// 1. Missing-status defaults for a character with NO status record yet in real SQL
	status, err := svc.GetStatus(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetStatus on missing status failed: %v", err)
	}
	if status.IsFull {
		t.Errorf("expected IsFull=false for character with no status record")
	}

	// ResetFullness on missing status: should succeed and create record with IsFull=false
	if err := svc.ResetFullness(ctx, char.ID); err != nil {
		t.Fatalf("ResetFullness on missing status failed: %v", err)
	}
	dbStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterStatus failed: %v", err)
	}
	if dbStatus.IsFull || dbStatus.TotalMealsEaten != 0 || dbStatus.TotalGoldSpent != 0 {
		t.Errorf("unexpected initial status: %+v", dbStatus)
	}

	// 2. Seed non-trivial history and fullness
	now := time.Now().UTC().Truncate(time.Second)
	seed := tavern.TavernCharacterStatus{
		CharacterID:     char.ID,
		IsFull:          true,
		TotalMealsEaten: 7,
		TotalGoldSpent:  1234,
		LastEatenAt:     &now,
	}
	if err := realTavernRepo.UpsertCharacterStatus(ctx, seed); err != nil {
		t.Fatal(err)
	}

	// 3. Inject storage failure on GetCharacterStatus
	sentinelErr := errors.New("simulated database read failure")
	wrappedRepo.getErr = sentinelErr

	// ResetFullness must propagate error without UPSERT
	if err := svc.ResetFullness(ctx, char.ID); !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinelErr from ResetFullness, got %v", err)
	}
	// Verify in real SQL: counters and fullness are untouched
	stAfterResetErr, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stAfterResetErr.IsFull || stAfterResetErr.TotalMealsEaten != 7 || stAfterResetErr.TotalGoldSpent != 1234 {
		t.Fatalf("counters mutated on error: %+v", stAfterResetErr)
	}

	// GetStatus must propagate error
	if _, err := svc.GetStatus(ctx, char.ID); !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinelErr from GetStatus, got %v", err)
	}

	// OrderMeal must propagate error and roll back
	char.Money = 5000
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OrderMeal(ctx, char.ID, "tavern_curry"); !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinelErr from OrderMeal, got %v", err)
	}
	// Verify money was NOT deducted
	reloadedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedChar.Money != 5000 {
		t.Errorf("expected money untouched (5000), got %d", reloadedChar.Money)
	}

	// ClaimDelivery: reserve first, then claim with failure injection
	wrappedRepo.getErr = nil
	if _, err := svc.ReserveDelivery(ctx, char.ID, "tavern_omelet_rice"); err != nil {
		t.Fatalf("ReserveDelivery failed: %v", err)
	}
	wrappedRepo.getErr = sentinelErr

	if _, err := svc.ClaimDelivery(ctx, char.ID); !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinelErr from ClaimDelivery, got %v", err)
	}
	// Verify money untouched and delivery reservation remains
	reloadedChar, err = charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedChar.Money != 5000 {
		t.Errorf("expected money untouched (5000), got %d", reloadedChar.Money)
	}

	// 4. Healthy contrast: disable failure injection
	wrappedRepo.getErr = nil

	// ResetFullness clears fullness while retaining 7/1234
	if err := svc.ResetFullness(ctx, char.ID); err != nil {
		t.Fatalf("ResetFullness healthy contrast failed: %v", err)
	}
	finalStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalStatus.IsFull {
		t.Errorf("expected IsFull=false after healthy ResetFullness")
	}
	if finalStatus.TotalMealsEaten != 7 || finalStatus.TotalGoldSpent != 1234 {
		t.Errorf("expected counters preserved, got meals=%d gold=%d", finalStatus.TotalMealsEaten, finalStatus.TotalGoldSpent)
	}

	// OrderMeal succeeds, increments meals to 8, accumulates gold to 1634, deducts 400G
	orderRes, err := svc.OrderMeal(ctx, char.ID, "tavern_curry")
	if err != nil {
		t.Fatalf("OrderMeal healthy contrast failed: %v", err)
	}
	if orderRes.RemainingGold != 4600 {
		t.Errorf("expected remaining gold 4600, got %d", orderRes.RemainingGold)
	}
	finalStatus, err = realTavernRepo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !finalStatus.IsFull || finalStatus.TotalMealsEaten != 8 || finalStatus.TotalGoldSpent != 1634 {
		t.Errorf("unexpected status after order: %+v", finalStatus)
	}

	// Claim delivery succeeds, increments meals to 9, accumulates gold to 2384, deducts 750G
	claimRes, err := svc.ClaimDelivery(ctx, char.ID)
	if err != nil {
		t.Fatalf("ClaimDelivery healthy contrast failed: %v", err)
	}
	if claimRes.RemainingGold != 3850 {
		t.Errorf("expected remaining gold 3850, got %d", claimRes.RemainingGold)
	}
	finalStatus, err = realTavernRepo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalStatus.TotalMealsEaten != 9 || finalStatus.TotalGoldSpent != 2384 {
		t.Errorf("unexpected status after claim: %+v", finalStatus)
	}
}
