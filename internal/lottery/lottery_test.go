package lottery_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/lottery"
)

type mockLotteryRepo struct {
	getRaffleTicketsFn                func(ctx context.Context, charID string) (int, error)
	buyRaffleTicketsFn                func(ctx context.Context, charID string, count int, goldCost int) (int, corecharacter.Character, error)
	useRaffleTicketsFn                func(ctx context.Context, charID string, count int, rewardGold int) (int, corecharacter.Character, error)
	getActiveTakarakujiRoundFn        func(ctx context.Context) (lottery.TakarakujiRound, error)
	createTakarakujiRoundFn           func(ctx context.Context, round lottery.TakarakujiRound) (lottery.TakarakujiRound, error)
	countTakarakujiTicketsFn          func(ctx context.Context, roundID int) (int, error)
	hasCharacterPurchasedTakarakujiFn func(ctx context.Context, roundID int, characterID string) (bool, error)
	purchaseTakarakujiTicketFn        func(ctx context.Context, roundID int, characterID string, goldCost int) (lottery.TakarakujiTicket, corecharacter.Character, error)
	getCharacterTakarakujiTicketFn    func(ctx context.Context, roundID int, characterID string) (lottery.TakarakujiTicket, error)
	listCharacterTakarakujiTicketsFn  func(ctx context.Context, characterID string) ([]lottery.TakarakujiTicket, error)
	listRoundTakarakujiTicketsFn      func(ctx context.Context, roundID int) ([]lottery.TakarakujiTicket, error)
	settleTakarakujiRoundFn           func(ctx context.Context, roundID int, drawnAt time.Time, winningTickets []lottery.TakarakujiTicket) error
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
func (m *mockLotteryRepo) GetActiveTakarakujiRound(ctx context.Context) (lottery.TakarakujiRound, error) {
	if m.getActiveTakarakujiRoundFn != nil {
		return m.getActiveTakarakujiRoundFn(ctx)
	}
	return lottery.TakarakujiRound{}, lottery.ErrRoundNotFound
}
func (m *mockLotteryRepo) CreateTakarakujiRound(ctx context.Context, round lottery.TakarakujiRound) (lottery.TakarakujiRound, error) {
	if m.createTakarakujiRoundFn != nil {
		return m.createTakarakujiRoundFn(ctx, round)
	}
	round.RoundID = 1
	return round, nil
}
func (m *mockLotteryRepo) CountTakarakujiTickets(ctx context.Context, roundID int) (int, error) {
	if m.countTakarakujiTicketsFn != nil {
		return m.countTakarakujiTicketsFn(ctx, roundID)
	}
	return 0, nil
}
func (m *mockLotteryRepo) HasCharacterPurchasedTakarakuji(ctx context.Context, roundID int, characterID string) (bool, error) {
	if m.hasCharacterPurchasedTakarakujiFn != nil {
		return m.hasCharacterPurchasedTakarakujiFn(ctx, roundID, characterID)
	}
	return false, nil
}
func (m *mockLotteryRepo) PurchaseTakarakujiTicket(ctx context.Context, roundID int, characterID string, goldCost int) (lottery.TakarakujiTicket, corecharacter.Character, error) {
	if m.purchaseTakarakujiTicketFn != nil {
		return m.purchaseTakarakujiTicketFn(ctx, roundID, characterID, goldCost)
	}
	return lottery.TakarakujiTicket{ID: "ticket-1", RoundID: roundID, CharacterID: characterID}, corecharacter.Character{Money: 100000 - goldCost}, nil
}
func (m *mockLotteryRepo) GetCharacterTakarakujiTicket(ctx context.Context, roundID int, characterID string) (lottery.TakarakujiTicket, error) {
	if m.getCharacterTakarakujiTicketFn != nil {
		return m.getCharacterTakarakujiTicketFn(ctx, roundID, characterID)
	}
	return lottery.TakarakujiTicket{}, errors.New("not found")
}
func (m *mockLotteryRepo) ListCharacterTakarakujiTickets(ctx context.Context, characterID string) ([]lottery.TakarakujiTicket, error) {
	if m.listCharacterTakarakujiTicketsFn != nil {
		return m.listCharacterTakarakujiTicketsFn(ctx, characterID)
	}
	return nil, nil
}
func (m *mockLotteryRepo) ListRoundTakarakujiTickets(ctx context.Context, roundID int) ([]lottery.TakarakujiTicket, error) {
	if m.listRoundTakarakujiTicketsFn != nil {
		return m.listRoundTakarakujiTicketsFn(ctx, roundID)
	}
	return nil, nil
}
func (m *mockLotteryRepo) SettleTakarakujiRound(ctx context.Context, roundID int, drawnAt time.Time, winningTickets []lottery.TakarakujiTicket) error {
	if m.settleTakarakujiRoundFn != nil {
		return m.settleTakarakujiRoundFn(ctx, roundID, drawnAt, winningTickets)
	}
	return nil
}

type mockDepotRepo struct {
	depot map[string]depot.Depot
}

func (m *mockDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depot[characterID]
	if !ok {
		return depot.NewDepotWithCapacity(characterID, 10, 0, 0)
	}
	return d, nil
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockDepotRepo) Save(ctx context.Context, d depot.Depot) error {
	if m.depot == nil {
		m.depot = make(map[string]depot.Depot)
	}
	m.depot[d.CharacterID] = d
	return nil
}

type mockItemDefProvider struct {
	names map[string]string
}

func (m *mockItemDefProvider) FindByID(id string) (coreitem.Definition, error) {
	if name, ok := m.names[id]; ok {
		return coreitem.Definition{ID: id, Name: name}, nil
	}
	return coreitem.Definition{ID: id, Name: id}, nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func TestNextDrawDateJST(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)

	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "1st of month -> 11th",
			input:    time.Date(2026, 9, 1, 12, 0, 0, 0, jst),
			expected: time.Date(2026, 9, 11, 0, 0, 0, 0, jst),
		},
		{
			name:     "10th of month -> 11th",
			input:    time.Date(2026, 9, 10, 23, 59, 0, 0, jst),
			expected: time.Date(2026, 9, 11, 0, 0, 0, 0, jst),
		},
		{
			name:     "11th of month -> 21st",
			input:    time.Date(2026, 9, 11, 0, 0, 0, 0, jst),
			expected: time.Date(2026, 9, 21, 0, 0, 0, 0, jst),
		},
		{
			name:     "20th of month -> 21st",
			input:    time.Date(2026, 9, 20, 15, 30, 0, 0, jst),
			expected: time.Date(2026, 9, 21, 0, 0, 0, 0, jst),
		},
		{
			name:     "21st of month -> 1st of next month",
			input:    time.Date(2026, 9, 21, 1, 0, 0, 0, jst),
			expected: time.Date(2026, 10, 1, 0, 0, 0, 0, jst),
		},
		{
			name:     "30th of month -> 1st of next month",
			input:    time.Date(2026, 9, 30, 23, 59, 0, 0, jst),
			expected: time.Date(2026, 10, 1, 0, 0, 0, 0, jst),
		},
		{
			name:     "December 25th -> January 1st next year",
			input:    time.Date(2026, 12, 25, 10, 0, 0, 0, jst),
			expected: time.Date(2027, 1, 1, 0, 0, 0, 0, jst),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lottery.NextDrawDateJST(tt.input)
			if !got.Equal(tt.expected) {
				t.Errorf("NextDrawDateJST(%v) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetTakarakujiStatus(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	fixedTime := time.Date(2026, 9, 5, 10, 0, 0, 0, jst)

	repo := &mockLotteryRepo{
		getActiveTakarakujiRoundFn: func(ctx context.Context) (lottery.TakarakujiRound, error) {
			return lottery.TakarakujiRound{
				RoundID:      1,
				DrawDate:     time.Date(2026, 9, 11, 0, 0, 0, 0, jst),
				IsDrawn:      false,
				Prize1ItemID: "item-129",
				Prize1Amount: 1,
				Prize2ItemID: "weapon-40",
				Prize2Amount: 2,
				Prize3ItemID: "item-126",
				Prize3Amount: 3,
			}, nil
		},
		countTakarakujiTicketsFn: func(ctx context.Context, roundID int) (int, error) {
			return 8, nil
		},
	}

	itemProvider := &mockItemDefProvider{
		names: map[string]string{
			"item-129":  "神の錬金レシピ",
			"weapon-40": "流銀の剣",
			"item-126":  "超魔力水",
		},
	}

	svc, err := lottery.NewService(repo,
		lottery.WithItemDefinitionProvider(itemProvider),
		lottery.WithClock(fixedClock{now: fixedTime}),
	)
	if err != nil {
		t.Fatal(err)
	}

	status, err := svc.GetTakarakujiStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.RoundID != 1 {
		t.Errorf("RoundID = %d; want 1", status.RoundID)
	}
	if status.TicketPrice != 30000 {
		t.Errorf("TicketPrice = %d; want 30000", status.TicketPrice)
	}
	if status.MaxTickets != 20 {
		t.Errorf("MaxTickets = %d; want 20", status.MaxTickets)
	}
	if status.SoldCount != 8 {
		t.Errorf("SoldCount = %d; want 8", status.SoldCount)
	}
	if status.RemainingTickets != 12 {
		t.Errorf("RemainingTickets = %d; want 12", status.RemainingTickets)
	}
	if status.IsSoldOut {
		t.Error("IsSoldOut should be false")
	}
	if len(status.Prizes) != 3 {
		t.Fatalf("Prizes len = %d; want 3", len(status.Prizes))
	}
	if status.Prizes[0].ItemName != "神の錬金レシピ" || status.Prizes[0].WinnersCount != 1 {
		t.Errorf("Prize 1 = %+v", status.Prizes[0])
	}
	if status.Prizes[1].ItemName != "流銀の剣" || status.Prizes[1].WinnersCount != 2 {
		t.Errorf("Prize 2 = %+v", status.Prizes[1])
	}
	if status.Prizes[2].ItemName != "超魔力水" || status.Prizes[2].WinnersCount != 3 {
		t.Errorf("Prize 3 = %+v", status.Prizes[2])
	}
	// Check talk phrases
	lastPhrase := status.TalkPhrases[len(status.TalkPhrases)-1]
	if !strings.Contains(lastPhrase, "12") {
		t.Errorf("expected last talk phrase to mention 12 remaining, got: %s", lastPhrase)
	}
}

func TestBuyTakarakujiTicket(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	fixedTime := time.Date(2026, 9, 5, 10, 0, 0, 0, jst)

	t.Run("Successful purchase", func(t *testing.T) {
		repo := &mockLotteryRepo{
			getActiveTakarakujiRoundFn: func(ctx context.Context) (lottery.TakarakujiRound, error) {
				return lottery.TakarakujiRound{
					RoundID:  1,
					DrawDate: time.Date(2026, 9, 11, 0, 0, 0, 0, jst),
				}, nil
			},
			purchaseTakarakujiTicketFn: func(ctx context.Context, roundID int, characterID string, goldCost int) (lottery.TakarakujiTicket, corecharacter.Character, error) {
				return lottery.TakarakujiTicket{
					ID:          "ticket-123",
					RoundID:     1,
					CharacterID: characterID,
					PurchasedAt: fixedTime,
				}, corecharacter.Character{ID: characterID, Money: 70000}, nil
			},
		}

		svc, err := lottery.NewService(repo, lottery.WithClock(fixedClock{now: fixedTime}))
		if err != nil {
			t.Fatal(err)
		}

		res, err := svc.BuyTakarakujiTicket(context.Background(), "char-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Ticket.ID != "ticket-123" {
			t.Errorf("ticket id = %s", res.Ticket.ID)
		}
		if res.RemainingGold != 70000 {
			t.Errorf("remaining gold = %d", res.RemainingGold)
		}
		if !strings.Contains(res.NPCMessage, "2026/09/11") && !strings.Contains(res.NPCMessage, "2026/9/11") {
			t.Errorf("NPC message = %s; want to mention 2026/09/11", res.NPCMessage)
		}
	})

	t.Run("Insufficient gold", func(t *testing.T) {
		repo := &mockLotteryRepo{
			getActiveTakarakujiRoundFn: func(ctx context.Context) (lottery.TakarakujiRound, error) {
				return lottery.TakarakujiRound{RoundID: 1, DrawDate: time.Now()}, nil
			},
			purchaseTakarakujiTicketFn: func(ctx context.Context, roundID int, characterID string, goldCost int) (lottery.TakarakujiTicket, corecharacter.Character, error) {
				return lottery.TakarakujiTicket{}, corecharacter.Character{}, lottery.ErrInsufficientGold
			},
		}
		svc, _ := lottery.NewService(repo)
		_, err := svc.BuyTakarakujiTicket(context.Background(), "poor-char")
		if !errors.Is(err, lottery.ErrInsufficientGold) {
			t.Errorf("expected ErrInsufficientGold, got %v", err)
		}
	})

	t.Run("Sold out (20 reached)", func(t *testing.T) {
		repo := &mockLotteryRepo{
			getActiveTakarakujiRoundFn: func(ctx context.Context) (lottery.TakarakujiRound, error) {
				return lottery.TakarakujiRound{RoundID: 1, DrawDate: time.Now()}, nil
			},
			purchaseTakarakujiTicketFn: func(ctx context.Context, roundID int, characterID string, goldCost int) (lottery.TakarakujiTicket, corecharacter.Character, error) {
				return lottery.TakarakujiTicket{}, corecharacter.Character{}, lottery.ErrSoldOut
			},
		}
		svc, _ := lottery.NewService(repo)
		_, err := svc.BuyTakarakujiTicket(context.Background(), "char-late")
		if !errors.Is(err, lottery.ErrSoldOut) {
			t.Errorf("expected ErrSoldOut, got %v", err)
		}
	})

	t.Run("Already purchased (1 per person)", func(t *testing.T) {
		repo := &mockLotteryRepo{
			getActiveTakarakujiRoundFn: func(ctx context.Context) (lottery.TakarakujiRound, error) {
				return lottery.TakarakujiRound{RoundID: 1, DrawDate: time.Now()}, nil
			},
			purchaseTakarakujiTicketFn: func(ctx context.Context, roundID int, characterID string, goldCost int) (lottery.TakarakujiTicket, corecharacter.Character, error) {
				return lottery.TakarakujiTicket{}, corecharacter.Character{}, lottery.ErrAlreadyPurchased
			},
		}
		svc, _ := lottery.NewService(repo)
		_, err := svc.BuyTakarakujiTicket(context.Background(), "char-greedy")
		if !errors.Is(err, lottery.ErrAlreadyPurchased) {
			t.Errorf("expected ErrAlreadyPurchased, got %v", err)
		}
	})
}

func TestDrawTakarakuji(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	drawTime := time.Date(2026, 9, 11, 0, 0, 0, 0, jst)

	activeRound := lottery.TakarakujiRound{
		RoundID:      1,
		DrawDate:     drawTime,
		IsDrawn:      false,
		Prize1ItemID: "item-129",
		Prize1Amount: 1,
		Prize2ItemID: "weapon-40",
		Prize2Amount: 2,
		Prize3ItemID: "item-126",
		Prize3Amount: 3,
	}

	tickets := []lottery.TakarakujiTicket{
		{ID: "t-1", RoundID: 1, CharacterID: "char-1"},
		{ID: "t-2", RoundID: 1, CharacterID: "char-2"},
		{ID: "t-3", RoundID: 1, CharacterID: "char-3"},
	}

	var settledRoundID int
	var settledWinningTickets []lottery.TakarakujiTicket
	var createdRound lottery.TakarakujiRound

	repo := &mockLotteryRepo{
		getActiveTakarakujiRoundFn: func(ctx context.Context) (lottery.TakarakujiRound, error) {
			return activeRound, nil
		},
		listRoundTakarakujiTicketsFn: func(ctx context.Context, roundID int) ([]lottery.TakarakujiTicket, error) {
			return tickets, nil
		},
		settleTakarakujiRoundFn: func(ctx context.Context, roundID int, drawnAt time.Time, winningTickets []lottery.TakarakujiTicket) error {
			settledRoundID = roundID
			settledWinningTickets = winningTickets
			return nil
		},
		createTakarakujiRoundFn: func(ctx context.Context, round lottery.TakarakujiRound) (lottery.TakarakujiRound, error) {
			createdRound = round
			round.RoundID = 2
			return round, nil
		},
	}

	depotRepo := &mockDepotRepo{depot: make(map[string]depot.Depot)}

	svc, err := lottery.NewService(repo,
		lottery.WithDepotRepository(depotRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.DrawTakarakuji(context.Background(), drawTime)
	if err != nil {
		t.Fatalf("DrawTakarakuji failed: %v", err)
	}

	if result.RoundID != 1 {
		t.Errorf("result.RoundID = %d", result.RoundID)
	}
	if settledRoundID != 1 {
		t.Errorf("settledRoundID = %d", settledRoundID)
	}
	_ = settledWinningTickets
	// Total winners drawn: 1 + 2 + 3 = 6 winners (some may be dummies)
	if len(result.Winners) != 6 {
		t.Fatalf("Winners count = %d; want 6", len(result.Winners))
	}

	// Verify next round was created with next draw date (Sept 21)
	expectedNextDate := time.Date(2026, 9, 21, 0, 0, 0, 0, jst)
	if !createdRound.DrawDate.Equal(expectedNextDate) {
		t.Errorf("createdRound.DrawDate = %v; want %v", createdRound.DrawDate, expectedNextDate)
	}

	// If any real character won, check that prize was delivered to depot
	for _, w := range result.Winners {
		if !w.IsDummy {
			dp, err := depotRepo.FindByCharacterID(context.Background(), w.CharacterID)
			if err != nil {
				t.Errorf("failed fetching depot for winner %s: %v", w.CharacterID, err)
			}
			found := false
			for _, item := range dp.Items {
				if item.DefinitionID == w.ItemID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected item %s in winner %s depot", w.ItemID, w.CharacterID)
			}
		}
	}
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

		pMiss := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, 90)
		if pMiss.Tier != lottery.PrizeTierMiss || pMiss.RewardGold != 0 {
			t.Errorf("expected Miss, got %+v", pMiss)
		}
	})
}
