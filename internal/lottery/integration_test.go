package lottery_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/collection"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/lottery"
)

func TestTakarakujiDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lotteryRepo, err := database.NewLotteryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	itemCatalog, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	colRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	colSvc, err := collection.NewService(colRepo, 286, 150)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := lottery.NewService(lotteryRepo,
		lottery.WithCharacterRepository(charRepo),
		lottery.WithDepotRepository(depotRepo),
		lottery.WithItemDefinitionProvider(itemCatalog),
		lottery.WithCollectionRecorder(colSvc),
		lottery.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Acquire advisory lock to prevent cross-package test interference during parallel test runs
	lockConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lockConn.Close()

	var lockAcquired int
	if err := lockConn.QueryRowContext(ctx, "SELECT GET_LOCK('takarakuji_test_mutex', 60)").Scan(&lockAcquired); err != nil || lockAcquired != 1 {
		t.Fatalf("failed acquiring takarakuji test mutex: %v", err)
	}
	defer func() {
		_, _ = lockConn.ExecContext(ctx, "SELECT RELEASE_LOCK('takarakuji_test_mutex')")
	}()

	// Clean tables for isolated round testing
	if _, err := db.ExecContext(ctx, "DELETE FROM takarakuji_tickets"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM takarakuji_rounds"); err != nil {
		t.Fatal(err)
	}

	// 1. Create main test character with 100,000 gold
	char1, err := database.CreateTestCharacter(ctx, db, "TakarakujiPlayer1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 100000, char1.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Get initial status (auto-initializes round if none)
	status, err := svc.GetTakarakujiStatus(ctx)
	if err != nil {
		t.Fatalf("GetTakarakujiStatus failed: %v", err)
	}
	if status.TicketPrice != 30000 || status.MaxTickets != 20 {
		t.Fatalf("unexpected status: %+v", status)
	}

	// 3. Buy ticket for char1
	res1, err := svc.BuyTakarakujiTicket(ctx, char1.ID)
	if err != nil {
		t.Fatalf("BuyTakarakujiTicket failed: %v", err)
	}
	if res1.RemainingGold != 70000 {
		t.Errorf("char1 remaining gold = %d; want 70000", res1.RemainingGold)
	}
	if res1.Ticket.RoundID != status.RoundID {
		t.Errorf("ticket round = %d; want %d", res1.Ticket.RoundID, status.RoundID)
	}

	// 4. Duplicate purchase attempt by char1 -> must fail with ErrAlreadyPurchased
	_, err = svc.BuyTakarakujiTicket(ctx, char1.ID)
	if !errors.Is(err, lottery.ErrAlreadyPurchased) {
		t.Fatalf("expected ErrAlreadyPurchased, got: %v", err)
	}

	// 5. Buy tickets 2..19 with unique characters (19 tickets total)
	for i := 2; i <= 19; i++ {
		charName := fmt.Sprintf("TakarakujiBuyer%d_%d", i, time.Now().UnixNano())
		c, err := database.CreateTestCharacter(ctx, db, charName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 50000, c.ID); err != nil {
			t.Fatal(err)
		}
		_, err = svc.BuyTakarakujiTicket(ctx, c.ID)
		if err != nil {
			t.Fatalf("failed buying ticket for buyer %d: %v", i, err)
		}
	}

	// 5b. Concurrently compete for the final (20th) ticket with 10 buyers
	const concurrentBuyers = 10
	raceBuyers := make([]string, concurrentBuyers)
	for i := 0; i < concurrentBuyers; i++ {
		charName := fmt.Sprintf("TakarakujiRaceBuyer%d_%d", i, time.Now().UnixNano())
		c, err := database.CreateTestCharacter(ctx, db, charName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 50000, c.ID); err != nil {
			t.Fatal(err)
		}
		raceBuyers[i] = c.ID
	}

	var wg sync.WaitGroup
	errCh := make(chan error, concurrentBuyers)
	startBarrier := make(chan struct{})

	for i := 0; i < concurrentBuyers; i++ {
		wg.Add(1)
		buyerID := raceBuyers[i]
		go func() {
			defer wg.Done()
			<-startBarrier
			_, pErr := svc.BuyTakarakujiTicket(ctx, buyerID)
			errCh <- pErr
		}()
	}

	close(startBarrier)
	wg.Wait()
	close(errCh)

	var successCount, soldOutCount int
	for pErr := range errCh {
		if pErr == nil {
			successCount++
		} else if errors.Is(pErr, lottery.ErrSoldOut) {
			soldOutCount++
		} else {
			t.Errorf("unexpected error in concurrent buy: %v", pErr)
		}
	}

	if successCount != 1 {
		t.Errorf("concurrent successCount = %d, want 1", successCount)
	}
	if soldOutCount != concurrentBuyers-1 {
		t.Errorf("concurrent soldOutCount = %d, want %d", soldOutCount, concurrentBuyers-1)
	}

	// 6. 21st ticket attempt with a new character -> must fail with ErrSoldOut
	char21, err := database.CreateTestCharacter(ctx, db, "TakarakujiLateBuyer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 50000, char21.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.BuyTakarakujiTicket(ctx, char21.ID)
	if !errors.Is(err, lottery.ErrSoldOut) {
		t.Fatalf("expected ErrSoldOut, got: %v", err)
	}

	// 7. Verify status reports sold out
	statusSold, err := svc.GetTakarakujiStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if statusSold.SoldCount != 20 || statusSold.RemainingTickets != 0 || !statusSold.IsSoldOut {
		t.Errorf("expected sold out status, got: %+v", statusSold)
	}

	// 8. Execute draw: premature attempt must return ErrNotReadyToDraw
	_, err = svc.DrawTakarakuji(ctx, time.Now().UTC())
	if !errors.Is(err, lottery.ErrNotReadyToDraw) {
		t.Fatalf("expected ErrNotReadyToDraw when drawing before draw_date, got: %v", err)
	}

	// 8b. Execute draw at scheduled draw date
	drawResult, err := svc.DrawTakarakuji(ctx, status.DrawDate.Add(time.Second))
	if err != nil {
		t.Fatalf("DrawTakarakuji failed: %v", err)
	}
	if drawResult.RoundID != status.RoundID {
		t.Errorf("drawResult.RoundID = %d; want %d", drawResult.RoundID, status.RoundID)
	}
	if drawResult.NextRound.RoundID == 0 {
		t.Errorf("expected new round created, got: %+v", drawResult.NextRound)
	}

	// 9. Check winners and verify prize delivery to Depot and proper collection category recording
	for _, w := range drawResult.Winners {
		if !w.IsDummy {
			dp, err := depotRepo.FindByCharacterID(ctx, w.CharacterID)
			if err != nil {
				t.Errorf("failed getting depot for winner %s: %v", w.CharacterID, err)
			}
			found := false
			for _, it := range dp.Items {
				if it.DefinitionID == w.ItemID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("winner %s depot does not contain won prize %s", w.CharacterID, w.ItemID)
			}

			// Verify collection recording category
			colEntries, err := colRepo.GetItemCollection(ctx, w.CharacterID, "")
			if err != nil {
				t.Errorf("failed getting collection for winner %s: %v", w.CharacterID, err)
			}
			foundCol := false
			for _, ce := range colEntries {
				if ce.ItemID == w.ItemID {
					foundCol = true
					if ce.Category == "takarakuji" {
						t.Errorf("winner %s prize %s recorded with invalid category takarakuji", w.CharacterID, w.ItemID)
					}
					def, err := itemCatalog.FindByID(w.ItemID)
					if err == nil {
						if ce.Category != def.Category() {
							t.Errorf("winner %s prize %s recorded with wrong category %s; want %s", w.CharacterID, w.ItemID, ce.Category, def.Category())
						}
					}
				}
			}
			if !foundCol {
				t.Errorf("winner %s depot prize %s was not found in collection", w.CharacterID, w.ItemID)
			}
		}
	}
}

func TestRaffleDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lotteryRepo, err := database.NewLotteryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	itemCatalog, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)

	svc, err := lottery.NewService(lotteryRepo,
		lottery.WithCharacterRepository(charRepo),
		lottery.WithInventoryRepository(invRepo),
		lottery.WithDepotRepository(depotRepo),
		lottery.WithItemDefinitionProvider(itemCatalog),
		lottery.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("Boundary 299/300/301 Tickets on Real DB", func(t *testing.T) {
		// 299 tickets -> Standard Raffle (3 tickets consumed, 296 left)
		char299, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("Raffle299_%d", time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lotteryRepo.AddRaffleTickets(ctx, char299.ID, 299); err != nil {
			t.Fatal(err)
		}

		res299, remaining299, _, err := svc.PlayRaffle(ctx, char299.ID)
		if err != nil {
			t.Fatalf("PlayRaffle 299 failed: %v", err)
		}
		if res299.RaffleType != lottery.RaffleStandard {
			t.Errorf("expected RaffleStandard for 299 tickets, got %s", res299.RaffleType)
		}
		if res299.TicketsUsed != 3 {
			t.Errorf("expected 3 tickets used, got %d", res299.TicketsUsed)
		}
		if remaining299 != 296 {
			t.Errorf("expected 296 remaining tickets, got %d", remaining299)
		}
		dbTickets299, err := lotteryRepo.GetRaffleTickets(ctx, char299.ID)
		if err != nil || dbTickets299 != 296 {
			t.Errorf("DB tickets = %d; want 296 (err=%v)", dbTickets299, err)
		}

		// 300 tickets -> Special Raffle (300 tickets consumed, 0 left)
		char300, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("Raffle300_%d", time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lotteryRepo.AddRaffleTickets(ctx, char300.ID, 300); err != nil {
			t.Fatal(err)
		}

		res300, remaining300, _, err := svc.PlayRaffle(ctx, char300.ID)
		if err != nil {
			t.Fatalf("PlayRaffle 300 failed: %v", err)
		}
		if res300.RaffleType != lottery.RaffleSpecial {
			t.Errorf("expected RaffleSpecial for 300 tickets, got %s", res300.RaffleType)
		}
		if res300.TicketsUsed != 300 {
			t.Errorf("expected 300 tickets used, got %d", res300.TicketsUsed)
		}
		if remaining300 != 0 {
			t.Errorf("expected 0 remaining tickets, got %d", remaining300)
		}
		dbTickets300, err := lotteryRepo.GetRaffleTickets(ctx, char300.ID)
		if err != nil || dbTickets300 != 0 {
			t.Errorf("DB tickets = %d; want 0 (err=%v)", dbTickets300, err)
		}

		// 301 tickets -> Special Raffle (300 tickets consumed, 1 left)
		char301, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("Raffle301_%d", time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lotteryRepo.AddRaffleTickets(ctx, char301.ID, 301); err != nil {
			t.Fatal(err)
		}

		res301, remaining301, _, err := svc.PlayRaffle(ctx, char301.ID)
		if err != nil {
			t.Fatalf("PlayRaffle 301 failed: %v", err)
		}
		if res301.RaffleType != lottery.RaffleSpecial {
			t.Errorf("expected RaffleSpecial for 301 tickets, got %s", res301.RaffleType)
		}
		if res301.TicketsUsed != 300 {
			t.Errorf("expected 300 tickets used, got %d", res301.TicketsUsed)
		}
		if remaining301 != 1 {
			t.Errorf("expected 1 remaining tickets, got %d", remaining301)
		}
		dbTickets301, err := lotteryRepo.GetRaffleTickets(ctx, char301.ID)
		if err != nil || dbTickets301 != 1 {
			t.Errorf("DB tickets = %d; want 1 (err=%v)", dbTickets301, err)
		}
	})

	t.Run("Concurrent Draws with 300 Tickets - Strictly 1 Special Draw and Zero Stale Mode", func(t *testing.T) {
		char, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("RaffleRace300_%d", time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lotteryRepo.AddRaffleTickets(ctx, char.ID, 300); err != nil {
			t.Fatal(err)
		}

		const workers = 10
		var wg sync.WaitGroup
		startBarrier := make(chan struct{})
		results := make(chan error, workers)
		types := make(chan lottery.RaffleType, workers)

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startBarrier
				res, _, _, pErr := svc.PlayRaffle(ctx, char.ID)
				if pErr == nil {
					types <- res.RaffleType
				}
				results <- pErr
			}()
		}

		close(startBarrier)
		wg.Wait()
		close(results)
		close(types)

		successCount := 0
		insufficientCount := 0
		for pErr := range results {
			if pErr == nil {
				successCount++
			} else if errors.Is(pErr, lottery.ErrInsufficientTickets) {
				insufficientCount++
			} else {
				t.Errorf("unexpected error in concurrent draw: %v", pErr)
			}
		}

		if successCount != 1 {
			t.Fatalf("expected exactly 1 successful draw, got %d", successCount)
		}
		if insufficientCount != workers-1 {
			t.Fatalf("expected %d insufficient ticket errors, got %d", workers-1, insufficientCount)
		}

		for rType := range types {
			if rType != lottery.RaffleSpecial {
				t.Errorf("expected winning draw to be RaffleSpecial, got %s", rType)
			}
		}

		remaining, err := lotteryRepo.GetRaffleTickets(ctx, char.ID)
		if err != nil || remaining != 0 {
			t.Errorf("DB remaining tickets = %d; want 0 (err=%v)", remaining, err)
		}
	})

	t.Run("Concurrent Draws with 303 Tickets - Exactly 1 Special and 1 Standard Draw", func(t *testing.T) {
		char, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("RaffleRace303_%d", time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lotteryRepo.AddRaffleTickets(ctx, char.ID, 303); err != nil {
			t.Fatal(err)
		}

		const workers = 10
		var wg sync.WaitGroup
		startBarrier := make(chan struct{})
		results := make(chan error, workers)
		types := make(chan lottery.RaffleType, workers)

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startBarrier
				res, _, _, pErr := svc.PlayRaffle(ctx, char.ID)
				if pErr == nil {
					types <- res.RaffleType
				}
				results <- pErr
			}()
		}

		close(startBarrier)
		wg.Wait()
		close(results)
		close(types)

		successCount := 0
		insufficientCount := 0
		for pErr := range results {
			if pErr == nil {
				successCount++
			} else if errors.Is(pErr, lottery.ErrInsufficientTickets) {
				insufficientCount++
			} else {
				t.Errorf("unexpected error in concurrent draw: %v", pErr)
			}
		}

		if successCount != 2 {
			t.Fatalf("expected exactly 2 successful draws, got %d", successCount)
		}
		if insufficientCount != workers-2 {
			t.Fatalf("expected %d insufficient ticket errors, got %d", workers-2, insufficientCount)
		}

		var specialCount, standardCount int
		for rType := range types {
			if rType == lottery.RaffleSpecial {
				specialCount++
			} else if rType == lottery.RaffleStandard {
				standardCount++
			}
		}

		if specialCount != 1 || standardCount != 1 {
			t.Errorf("expected 1 Special and 1 Standard, got %d Special and %d Standard", specialCount, standardCount)
		}

		remaining, err := lotteryRepo.GetRaffleTickets(ctx, char.ID)
		if err != nil || remaining != 0 {
			t.Errorf("DB remaining tickets = %d; want 0 (err=%v)", remaining, err)
		}
	})
}
