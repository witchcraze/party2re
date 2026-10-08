package tavern_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/tavern"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failingTavernSQLRepo struct {
	*database.TavernRepository
	getErr   error
	resetErr error
}

func (r *failingTavernSQLRepo) GetCharacterStatus(ctx context.Context, id string) (tavern.TavernCharacterStatus, error) {
	if r.getErr != nil {
		return tavern.TavernCharacterStatus{}, r.getErr
	}
	return r.TavernRepository.GetCharacterStatus(ctx, id)
}

func (r *failingTavernSQLRepo) ResetFullness(ctx context.Context, id string) error {
	if r.resetErr != nil {
		return r.resetErr
	}
	return r.TavernRepository.ResetFullness(ctx, id)
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

	// 3. Inject storage failure on GetCharacterStatus and ResetFullness
	sentinelErr := errors.New("simulated database storage failure")
	wrappedRepo.getErr = sentinelErr
	wrappedRepo.resetErr = sentinelErr

	// ResetFullness must propagate error without mutating status
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
	wrappedRepo.resetErr = nil

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

type hookTavernRepo struct {
	tavern.Repository
	beforeReset func()
}

func (h *hookTavernRepo) ResetFullness(ctx context.Context, id string) error {
	if h.beforeReset != nil {
		h.beforeReset()
	}
	return h.Repository.ResetFullness(ctx, id)
}

func TestTavern_RealDatabase_ResetFullness_Concurrency(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	svc, err := tavern.NewService(catalog, realTavernRepo, charRepo, txProvider)
	if err != nil {
		t.Fatal(err)
	}

	defaultItem := catalog.Items()[0]

	// 1. Controlled interleaving: ResetFullness paused before SQL write vs concurrent OrderMeal
	t.Run("controlled_interleaving_order_meal", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "ResetOrderRace", 10000)
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

		seed := tavern.TavernCharacterStatus{
			CharacterID:     char.ID,
			IsFull:          false,
			TotalMealsEaten: 7,
			TotalGoldSpent:  1234,
			LastEatenAt:     nil,
		}
		if err := realTavernRepo.UpsertCharacterStatus(ctx, seed); err != nil {
			t.Fatal(err)
		}

		resetStarted := make(chan struct{})
		allowReset := make(chan struct{})
		pausingRepo := &hookTavernRepo{
			Repository: realTavernRepo,
			beforeReset: func() {
				close(resetStarted)
				<-allowReset
			},
		}
		resetSvc, err := tavern.NewService(catalog, pausingRepo, charRepo, txProvider)
		if err != nil {
			t.Fatal(err)
		}

		var resetErr error
		resetDone := make(chan struct{})
		go func() {
			defer close(resetDone)
			resetErr = resetSvc.ResetFullness(ctx, char.ID)
		}()

		select {
		case <-resetStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ResetFullness to start")
		}

		// Concurrently execute OrderMeal with unwrapped real service
		orderRes, err := svc.OrderMeal(ctx, char.ID, defaultItem.ID)
		if err != nil {
			t.Fatalf("OrderMeal failed: %v", err)
		}
		expectedGold := 10000 - defaultItem.Price
		if orderRes.RemainingGold != expectedGold {
			t.Errorf("expected gold %d, got %d", expectedGold, orderRes.RemainingGold)
		}

		midStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		expectedSpent := int64(1234 + defaultItem.Price)
		if !midStatus.IsFull || midStatus.TotalMealsEaten != 8 || midStatus.TotalGoldSpent != expectedSpent || midStatus.LastEatenAt == nil {
			t.Fatalf("unexpected midStatus after OrderMeal: %+v", midStatus)
		}

		// Resume ResetFullness and wait for it to return nil
		close(allowReset)
		select {
		case <-resetDone:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ResetFullness to finish")
		}
		if resetErr != nil {
			t.Fatalf("ResetFullness returned error: %v", resetErr)
		}

		// Verify real SQL status: history is preserved, IsFull is cleared
		finalStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalStatus.IsFull {
			t.Errorf("expected IsFull=false after ResetFullness")
		}
		if finalStatus.TotalMealsEaten != 8 {
			t.Errorf("expected TotalMealsEaten=8 preserved, got %d", finalStatus.TotalMealsEaten)
		}
		if finalStatus.TotalGoldSpent != expectedSpent {
			t.Errorf("expected TotalGoldSpent=%d preserved, got %d", expectedSpent, finalStatus.TotalGoldSpent)
		}
		if finalStatus.LastEatenAt == nil || !finalStatus.LastEatenAt.Equal(*midStatus.LastEatenAt) {
			t.Errorf("expected LastEatenAt preserved (%v), got: %v", midStatus.LastEatenAt, finalStatus.LastEatenAt)
		}

		reloadedChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedChar.Money != expectedGold {
			t.Errorf("expected money %d, got %d", expectedGold, reloadedChar.Money)
		}
	})

	// 2. Controlled interleaving: ResetFullness paused vs concurrent ClaimDelivery
	t.Run("controlled_interleaving_claim_delivery", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "ResetClaimRace", 10000)
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

		seed := tavern.TavernCharacterStatus{
			CharacterID:     char.ID,
			IsFull:          false,
			TotalMealsEaten: 7,
			TotalGoldSpent:  1234,
			LastEatenAt:     nil,
		}
		if err := realTavernRepo.UpsertCharacterStatus(ctx, seed); err != nil {
			t.Fatal(err)
		}

		if _, err := svc.ReserveDelivery(ctx, char.ID, defaultItem.ID); err != nil {
			t.Fatalf("ReserveDelivery failed: %v", err)
		}

		resetStarted := make(chan struct{})
		allowReset := make(chan struct{})
		pausingRepo := &hookTavernRepo{
			Repository: realTavernRepo,
			beforeReset: func() {
				close(resetStarted)
				<-allowReset
			},
		}
		resetSvc, err := tavern.NewService(catalog, pausingRepo, charRepo, txProvider)
		if err != nil {
			t.Fatal(err)
		}

		var resetErr error
		resetDone := make(chan struct{})
		go func() {
			defer close(resetDone)
			resetErr = resetSvc.ResetFullness(ctx, char.ID)
		}()

		select {
		case <-resetStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ResetFullness to start")
		}

		claimRes, err := svc.ClaimDelivery(ctx, char.ID)
		if err != nil {
			t.Fatalf("ClaimDelivery failed: %v", err)
		}
		expectedGold := 10000 - defaultItem.Price
		if claimRes.RemainingGold != expectedGold {
			t.Errorf("expected gold %d, got %d", expectedGold, claimRes.RemainingGold)
		}

		midStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		expectedSpent := int64(1234 + defaultItem.Price)
		if midStatus.TotalMealsEaten != 8 || midStatus.TotalGoldSpent != expectedSpent || midStatus.LastEatenAt == nil {
			t.Fatalf("unexpected midStatus after ClaimDelivery: %+v", midStatus)
		}

		close(allowReset)
		select {
		case <-resetDone:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ResetFullness to finish")
		}
		if resetErr != nil {
			t.Fatalf("ResetFullness returned error: %v", resetErr)
		}

		finalStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalStatus.IsFull {
			t.Errorf("expected IsFull=false")
		}
		if finalStatus.TotalMealsEaten != 8 || finalStatus.TotalGoldSpent != expectedSpent || finalStatus.LastEatenAt == nil {
			t.Errorf("expected history preserved after delivery claim, got: %+v", finalStatus)
		}
	})

	// 3. Paired race with barrier start
	t.Run("paired_race2", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "ResetRace2", 10000)
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

		err1, err2 := testutil.RunRace2(
			func() error {
				return svc.ResetFullness(ctx, char.ID)
			},
			func() error {
				_, err := svc.OrderMeal(ctx, char.ID, defaultItem.ID)
				if err != nil && errors.Is(err, tavern.ErrAlreadyFull) {
					return nil
				}
				return err
			},
		)

		if err1 != nil {
			t.Errorf("ResetFullness returned error in race: %v", err1)
		}
		if err2 != nil {
			t.Errorf("OrderMeal returned error in race: %v", err2)
		}
	})

	// 4. Legal serial orders produce consistent reset semantics
	t.Run("serial_order_contrast", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "ResetSerial", 10000)
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

		// Case A: Reset then Order
		seed := tavern.TavernCharacterStatus{
			CharacterID:     char.ID,
			IsFull:          true,
			TotalMealsEaten: 7,
			TotalGoldSpent:  1234,
		}
		if err := realTavernRepo.UpsertCharacterStatus(ctx, seed); err != nil {
			t.Fatal(err)
		}
		if err := svc.ResetFullness(ctx, char.ID); err != nil {
			t.Fatalf("ResetFullness failed: %v", err)
		}
		stA, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stA.IsFull || stA.TotalMealsEaten != 7 || stA.TotalGoldSpent != 1234 {
			t.Fatalf("unexpected state after ResetFullness: %+v", stA)
		}
		if _, err := svc.OrderMeal(ctx, char.ID, defaultItem.ID); err != nil {
			t.Fatalf("OrderMeal failed: %v", err)
		}
		stA2, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		expectedSpent1 := int64(1234 + defaultItem.Price)
		if !stA2.IsFull || stA2.TotalMealsEaten != 8 || stA2.TotalGoldSpent != expectedSpent1 {
			t.Fatalf("unexpected state after OrderMeal: %+v", stA2)
		}

		// Case B: Order then Reset
		if err := svc.ResetFullness(ctx, char.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.OrderMeal(ctx, char.ID, defaultItem.ID); err != nil {
			t.Fatalf("OrderMeal second order failed: %v", err)
		}
		stB, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		expectedSpent2 := int64(1234 + 2*defaultItem.Price)
		if !stB.IsFull || stB.TotalMealsEaten != 9 || stB.TotalGoldSpent != expectedSpent2 {
			t.Fatalf("unexpected state after second order: %+v", stB)
		}
		if err := svc.ResetFullness(ctx, char.ID); err != nil {
			t.Fatalf("ResetFullness after order failed: %v", err)
		}
		stB2, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stB2.IsFull || stB2.TotalMealsEaten != 9 || stB2.TotalGoldSpent != expectedSpent2 {
			t.Fatalf("unexpected state after reset following order: %+v", stB2)
		}
	})

	// 5. Concurrency stress test with asset and counter conservation
	t.Run("concurrent_stress_test", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "ResetStress", 10000)
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

		cfg := testutil.ConcurrencyStressConfig{
			Workers:      6,
			OpsPerWorker: 6,
		}

		res := testutil.RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
			if workerID%2 == 0 {
				return svc.ResetFullness(ctx, char.ID)
			}
			_, err := svc.OrderMeal(ctx, char.ID, defaultItem.ID)
			if err != nil && errors.Is(err, tavern.ErrAlreadyFull) {
				return nil
			}
			return err
		})

		if res.Deadlocks > 0 {
			t.Fatalf("Deadlock detected during stress test: %d", res.Deadlocks)
		}

		reloadedChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		finalStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}

		// Asset conservation check:
		expectedSpent := int64(finalStatus.TotalMealsEaten * defaultItem.Price)
		if finalStatus.TotalGoldSpent != expectedSpent {
			t.Errorf("gold spent mismatch: meals=%d * price=%d = %d, got TotalGoldSpent=%d",
				finalStatus.TotalMealsEaten, defaultItem.Price, expectedSpent, finalStatus.TotalGoldSpent)
		}
		if int64(reloadedChar.Money)+finalStatus.TotalGoldSpent != 10000 {
			t.Errorf("total asset conservation violated: money=%d + spent=%d != 10000",
				reloadedChar.Money, finalStatus.TotalGoldSpent)
		}
	})
}
