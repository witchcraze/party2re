package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/lottery"
)

func TestLotteryRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo, err := database.NewLotteryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create character with 10,000 gold
	char, err := database.CreateTestCharacter(ctx, db, "LotteryTester")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Buy 10 raffle tickets (10 * 100 = 1000 gold)
	tickets, updatedChar, err := repo.BuyRaffleTickets(ctx, char.ID, 10, 1000)
	if err != nil {
		t.Fatalf("BuyRaffleTickets failed: %v", err)
	}
	if tickets != 10 || updatedChar.Money != 9000 {
		t.Errorf("tickets=%d, money=%d", tickets, updatedChar.Money)
	}

	// 3. Use 3 raffle tickets with 500 gold reward
	remaining, updatedChar, err := repo.UseRaffleTickets(ctx, char.ID, 3, 500)
	if err != nil {
		t.Fatalf("UseRaffleTickets failed: %v", err)
	}
	if remaining != 7 || updatedChar.Money != 9500 {
		t.Errorf("remaining=%d, money=%d", remaining, updatedChar.Money)
	}

	// 4. Purchase numbered lottery ticket (300 gold)
	tkt := lottery.LotteryTicket{
		CharacterID:  char.ID,
		RoundID:      1,
		TicketNumber: "7429",
	}
	purchased, updatedChar, err := repo.PurchaseLotteryTicket(ctx, tkt, 300)
	if err != nil {
		t.Fatalf("PurchaseLotteryTicket failed: %v", err)
	}
	if purchased.ID == "" || updatedChar.Money != 9200 {
		t.Errorf("purchased ticket ID=%s, money=%d", purchased.ID, updatedChar.Money)
	}

	// 5. Save drawing for round 1
	drawing := lottery.LotteryDrawing{
		RoundID:       1,
		WinningNumber: "7429",
		DrawnAt:       time.Now().UTC(),
		IsSettled:     true,
	}
	if err := repo.SaveDrawing(ctx, drawing); err != nil {
		t.Fatalf("SaveDrawing failed: %v", err)
	}

	// 6. Claim ticket for 1st Prize (100,000 gold)
	claimed, updatedChar, err := repo.ClaimLotteryTicket(ctx, purchased.ID, lottery.PrizeTier1st, 100000)
	if err != nil {
		t.Fatalf("ClaimLotteryTicket failed: %v", err)
	}
	if !claimed.Claimed || updatedChar.Money != 109200 {
		t.Errorf("claimed=%v, final money=%d", claimed.Claimed, updatedChar.Money)
	}

	// 7. Double claim returns error
	if _, _, err := repo.ClaimLotteryTicket(ctx, purchased.ID, lottery.PrizeTier1st, 100000); err != lottery.ErrTicketAlreadyClaimed {
		t.Errorf("double claim err = %v, want ErrTicketAlreadyClaimed", err)
	}
}
