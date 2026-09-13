package lottery_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

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

	svc, err := lottery.NewService(lotteryRepo,
		lottery.WithCharacterRepository(charRepo),
		lottery.WithDepotRepository(depotRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

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

	// 5. Buy remaining 19 tickets with 19 unique characters
	for i := 2; i <= 20; i++ {
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

	// 8. Execute draw
	drawResult, err := svc.DrawTakarakuji(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("DrawTakarakuji failed: %v", err)
	}
	if drawResult.RoundID != status.RoundID {
		t.Errorf("drawResult.RoundID = %d; want %d", drawResult.RoundID, status.RoundID)
	}
	if drawResult.NextRound.RoundID == 0 {
		t.Errorf("expected new round created, got: %+v", drawResult.NextRound)
	}

	// 9. Check winners and verify prize delivery to Depot
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
		}
	}
}
