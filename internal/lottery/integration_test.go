package lottery_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/lottery"
)

func TestLotteryServiceDatabaseIntegration(t *testing.T) {
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

	svc, err := lottery.NewService(lotteryRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character with 10,000 gold
	char, err := database.CreateTestCharacter(ctx, db, "FullLotteryPlayer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Buy 6 raffle tickets (costs 600 gold)
	tickets, updatedChar, err := svc.BuyRaffleTickets(ctx, char.ID, 6)
	if err != nil {
		t.Fatalf("BuyRaffleTickets failed: %v", err)
	}
	if tickets != 6 || updatedChar.Money != 9400 {
		t.Errorf("tickets=%d, money=%d", tickets, updatedChar.Money)
	}

	// 3. Play raffle
	res, remaining, raffleChar, err := svc.PlayRaffle(ctx, char.ID, lottery.RaffleStandard)
	if err != nil {
		t.Fatalf("PlayRaffle failed: %v", err)
	}
	if remaining != 3 || res.TicketsUsed != 3 {
		t.Errorf("remaining=%d, used=%d", remaining, res.TicketsUsed)
	}
	if raffleChar.Money != 9400+res.Prize.RewardGold {
		t.Errorf("money after raffle = %d, want %d", raffleChar.Money, 9400+res.Prize.RewardGold)
	}

	// 4. Purchase lottery ticket for round 10 (costs 300 gold)
	ticket, ticketChar, err := svc.PurchaseLotteryTicket(ctx, char.ID, 10, "5555")
	if err != nil {
		t.Fatalf("PurchaseLotteryTicket failed: %v", err)
	}
	if ticket.TicketNumber != "5555" || ticketChar.Money != raffleChar.Money-300 {
		t.Errorf("ticket=%+v, money=%d", ticket, ticketChar.Money)
	}

	// 5. Settle drawing with winning number 5555
	drawing, err := svc.SettleDrawing(ctx, 10, "5555")
	if err != nil {
		t.Fatalf("SettleDrawing failed: %v", err)
	}
	if !drawing.IsSettled {
		t.Error("drawing should be settled")
	}

	// 6. Claim winning ticket (100,000 gold jackpot)
	claimed, claimedChar, err := svc.ClaimLotteryTicket(ctx, char.ID, ticket.ID)
	if err != nil {
		t.Fatalf("ClaimLotteryTicket failed: %v", err)
	}
	if claimed.PrizeTier != lottery.PrizeTier1st || claimed.PrizeGold != 100000 {
		t.Errorf("claimed prize tier=%s, gold=%d", claimed.PrizeTier, claimed.PrizeGold)
	}
	if claimedChar.Money != ticketChar.Money+100000 {
		t.Errorf("final money=%d, expected %d", claimedChar.Money, ticketChar.Money+100000)
	}
}
