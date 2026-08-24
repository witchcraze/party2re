package lottery_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/lottery"
)

type mockLotteryRepo struct {
	getRaffleTicketsFn func(ctx context.Context, charID string) (int, error)
	buyRaffleTicketsFn func(ctx context.Context, charID string, count int, goldCost int) (int, corecharacter.Character, error)
	useRaffleTicketsFn func(ctx context.Context, charID string, count int, rewardGold int) (int, corecharacter.Character, error)
	purchaseTicketFn   func(ctx context.Context, ticket lottery.LotteryTicket, goldCost int) (lottery.LotteryTicket, corecharacter.Character, error)
	getTicketFn        func(ctx context.Context, ticketID string) (lottery.LotteryTicket, error)
	listTicketsFn      func(ctx context.Context, charID string, roundID int) ([]lottery.LotteryTicket, error)
	saveDrawingFn      func(ctx context.Context, drawing lottery.LotteryDrawing) error
	getDrawingFn       func(ctx context.Context, roundID int) (lottery.LotteryDrawing, error)
	claimTicketFn      func(ctx context.Context, ticketID string, tier string, prizeGold int) (lottery.LotteryTicket, corecharacter.Character, error)
}

func (m *mockLotteryRepo) GetRaffleTickets(ctx context.Context, charID string) (int, error) {
	if m.getRaffleTicketsFn != nil {
		return m.getRaffleTicketsFn(ctx, charID)
	}
	return 0, nil
}
func (m *mockLotteryRepo) BuyRaffleTickets(ctx context.Context, charID string, count int, goldCost int) (int, corecharacter.Character, error) {
	if m.buyRaffleTicketsFn != nil {
		return m.buyRaffleTicketsFn(ctx, charID, count, goldCost)
	}
	return 0, corecharacter.Character{}, nil
}
func (m *mockLotteryRepo) UseRaffleTickets(ctx context.Context, charID string, count int, rewardGold int) (int, corecharacter.Character, error) {
	if m.useRaffleTicketsFn != nil {
		return m.useRaffleTicketsFn(ctx, charID, count, rewardGold)
	}
	return 0, corecharacter.Character{}, nil
}
func (m *mockLotteryRepo) PurchaseLotteryTicket(ctx context.Context, ticket lottery.LotteryTicket, goldCost int) (lottery.LotteryTicket, corecharacter.Character, error) {
	if m.purchaseTicketFn != nil {
		return m.purchaseTicketFn(ctx, ticket, goldCost)
	}
	return ticket, corecharacter.Character{}, nil
}
func (m *mockLotteryRepo) GetLotteryTicket(ctx context.Context, ticketID string) (lottery.LotteryTicket, error) {
	if m.getTicketFn != nil {
		return m.getTicketFn(ctx, ticketID)
	}
	return lottery.LotteryTicket{}, nil
}
func (m *mockLotteryRepo) ListLotteryTickets(ctx context.Context, charID string, roundID int) ([]lottery.LotteryTicket, error) {
	if m.listTicketsFn != nil {
		return m.listTicketsFn(ctx, charID, roundID)
	}
	return nil, nil
}
func (m *mockLotteryRepo) SaveDrawing(ctx context.Context, drawing lottery.LotteryDrawing) error {
	if m.saveDrawingFn != nil {
		return m.saveDrawingFn(ctx, drawing)
	}
	return nil
}
func (m *mockLotteryRepo) GetDrawing(ctx context.Context, roundID int) (lottery.LotteryDrawing, error) {
	if m.getDrawingFn != nil {
		return m.getDrawingFn(ctx, roundID)
	}
	return lottery.LotteryDrawing{}, nil
}
func (m *mockLotteryRepo) ClaimLotteryTicket(ctx context.Context, ticketID string, tier string, prizeGold int) (lottery.LotteryTicket, corecharacter.Character, error) {
	if m.claimTicketFn != nil {
		return m.claimTicketFn(ctx, ticketID, tier, prizeGold)
	}
	return lottery.LotteryTicket{}, corecharacter.Character{}, nil
}

func TestEvaluateRaffleRoll(t *testing.T) {
	t.Run("Standard Raffle Tiers", func(t *testing.T) {
		p0 := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, 0)
		if p0.Tier != lottery.PrizeTierGrand || p0.RewardGold != 5000 {
			t.Errorf("expected Grand Prize, got %+v", p0)
		}

		p1 := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, 3)
		if p1.Tier != lottery.PrizeTier1st || p1.RewardGold != 2500 {
			t.Errorf("expected 1st Prize, got %+v", p1)
		}

		pMiss := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, 500)
		if pMiss.Tier != lottery.PrizeTierMiss || pMiss.RewardGold != 0 {
			t.Errorf("expected Miss, got %+v", pMiss)
		}
	})

	t.Run("Special Raffle Tiers", func(t *testing.T) {
		p0 := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, 2)
		if p0.Tier != lottery.PrizeTierGrand || p0.RewardGold != 100000 {
			t.Errorf("expected Gold Orb, got %+v", p0)
		}

		p1 := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, 10)
		if p1.Tier != lottery.PrizeTier1st || p1.RewardGold != 20000 {
			t.Errorf("expected Silver Orb, got %+v", p1)
		}

		pMiss := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, 85)
		if pMiss.Tier != lottery.PrizeTierMiss || pMiss.RewardGold != 0 {
			t.Errorf("expected White Orb Miss, got %+v", pMiss)
		}
	})
}

func TestEvaluateLotteryTicket(t *testing.T) {
	tests := []struct {
		name          string
		ticket        string
		winning       string
		wantTier      string
		wantPrizeGold int
	}{
		{
			name:          "1st Prize Exact Match",
			ticket:        "7429",
			winning:       "7429",
			wantTier:      lottery.PrizeTier1st,
			wantPrizeGold: 100000,
		},
		{
			name:          "2nd Prize Last 3 Digits",
			ticket:        "1429",
			winning:       "7429",
			wantTier:      lottery.PrizeTier2nd,
			wantPrizeGold: 10000,
		},
		{
			name:          "3rd Prize Last 2 Digits",
			ticket:        "8829",
			winning:       "7429",
			wantTier:      lottery.PrizeTier3rd,
			wantPrizeGold: 1000,
		},
		{
			name:          "4th Prize Last 1 Digit",
			ticket:        "0009",
			winning:       "7429",
			wantTier:      lottery.PrizeTier4th,
			wantPrizeGold: 300,
		},
		{
			name:          "Miss",
			ticket:        "0000",
			winning:       "7429",
			wantTier:      lottery.PrizeTierMiss,
			wantPrizeGold: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, prize := lottery.EvaluateLotteryTicket(tt.ticket, tt.winning)
			if tier != tt.wantTier || prize != tt.wantPrizeGold {
				t.Errorf("EvaluateLotteryTicket(%s, %s) = (%s, %d), want (%s, %d)", tt.ticket, tt.winning, tier, prize, tt.wantTier, tt.wantPrizeGold)
			}
		})
	}
}

func TestLotteryService_RaffleFlow(t *testing.T) {
	ctx := context.Background()
	tickets := 10
	money := 1000

	repo := &mockLotteryRepo{
		getRaffleTicketsFn: func(_ context.Context, charID string) (int, error) {
			return tickets, nil
		},
		useRaffleTicketsFn: func(_ context.Context, charID string, count int, rewardGold int) (int, corecharacter.Character, error) {
			tickets -= count
			money += rewardGold
			return tickets, corecharacter.Character{ID: charID, Money: money}, nil
		},
	}
	svc, _ := lottery.NewService(repo)

	// Play standard raffle (costs 3 tickets)
	res, remaining, char, err := svc.PlayRaffle(ctx, "char1", lottery.RaffleStandard)
	if err != nil {
		t.Fatalf("PlayRaffle failed: %v", err)
	}
	if res.TicketsUsed != 3 || remaining != 7 {
		t.Errorf("tickets used=%d, remaining=%d", res.TicketsUsed, remaining)
	}
	if char.Money != 1000+res.Prize.RewardGold {
		t.Errorf("char money=%d, expected %d", char.Money, 1000+res.Prize.RewardGold)
	}
}
