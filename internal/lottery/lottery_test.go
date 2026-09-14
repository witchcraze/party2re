package lottery_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/lottery"
)

type mockLotteryRepo struct {
	getRaffleTicketsFn                  func(ctx context.Context, charID string) (int, error)
	useRaffleTicketsFn                  func(ctx context.Context, charID string, count int) (int, error)
	getActiveTakarakujiRoundFn          func(ctx context.Context) (lottery.TakarakujiRound, error)
	getActiveTakarakujiRoundForUpdateFn func(ctx context.Context) (lottery.TakarakujiRound, error)
	createTakarakujiRoundFn             func(ctx context.Context, round lottery.TakarakujiRound) (lottery.TakarakujiRound, error)
	countTakarakujiTicketsFn            func(ctx context.Context, roundID int) (int, error)
	hasCharacterPurchasedTakarakujiFn   func(ctx context.Context, roundID int, characterID string) (bool, error)
	purchaseTakarakujiTicketFn          func(ctx context.Context, roundID int, characterID string, goldCost int) (lottery.TakarakujiTicket, corecharacter.Character, error)
	getCharacterTakarakujiTicketFn      func(ctx context.Context, roundID int, characterID string) (lottery.TakarakujiTicket, error)
	listCharacterTakarakujiTicketsFn    func(ctx context.Context, characterID string) ([]lottery.TakarakujiTicket, error)
	listRoundTakarakujiTicketsFn        func(ctx context.Context, roundID int) ([]lottery.TakarakujiTicket, error)
	settleTakarakujiRoundFn             func(ctx context.Context, roundID int, drawnAt time.Time, winningTickets []lottery.TakarakujiTicket) error
}

func (m *mockLotteryRepo) GetRaffleTickets(ctx context.Context, charID string) (int, error) {
	if m.getRaffleTicketsFn != nil {
		return m.getRaffleTicketsFn(ctx, charID)
	}
	return 0, nil
}
func (m *mockLotteryRepo) UseRaffleTickets(ctx context.Context, charID string, count int) (int, error) {
	if m.useRaffleTicketsFn != nil {
		return m.useRaffleTicketsFn(ctx, charID, count)
	}
	return 0, nil
}
func (m *mockLotteryRepo) GetActiveTakarakujiRound(ctx context.Context) (lottery.TakarakujiRound, error) {
	if m.getActiveTakarakujiRoundFn != nil {
		return m.getActiveTakarakujiRoundFn(ctx)
	}
	return lottery.TakarakujiRound{}, lottery.ErrRoundNotFound
}
func (m *mockLotteryRepo) GetActiveTakarakujiRoundForUpdate(ctx context.Context) (lottery.TakarakujiRound, error) {
	if m.getActiveTakarakujiRoundForUpdateFn != nil {
		return m.getActiveTakarakujiRoundForUpdateFn(ctx)
	}
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

type mockTxProvider struct {
	called     bool
	committed  bool
	rolledBack bool
}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.called = true
	err := fn(ctx)
	if err != nil {
		m.rolledBack = true
		return err
	}
	m.committed = true
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

type mockInventoryRepo struct {
	inventories map[string]coreinventory.Inventory
}

func (m *mockInventoryRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if m.inventories == nil {
		m.inventories = make(map[string]coreinventory.Inventory)
	}
	inv, ok := m.inventories[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
		m.inventories[characterID] = inv
	}
	return inv, nil
}

func (m *mockInventoryRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInventoryRepo) Save(ctx context.Context, value coreinventory.Inventory) error {
	if m.inventories == nil {
		m.inventories = make(map[string]coreinventory.Inventory)
	}
	m.inventories[value.CharacterID] = value
	return nil
}

type mockCharacterRepo struct {
	characters map[string]corecharacter.Character
}

func (m *mockCharacterRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if m.characters == nil {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharacterRepo) Update(ctx context.Context, value corecharacter.Character) error {
	if m.characters == nil {
		m.characters = make(map[string]corecharacter.Character)
	}
	m.characters[value.ID] = value
	return nil
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

func TestDrawTakarakuji_Premature(t *testing.T) {
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

	repo := &mockLotteryRepo{
		getActiveTakarakujiRoundFn: func(ctx context.Context) (lottery.TakarakujiRound, error) {
			return activeRound, nil
		},
	}

	svc, err := lottery.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	// Attempt draw 1 hour before scheduled time
	prematureTime := drawTime.Add(-1 * time.Hour)
	_, err = svc.DrawTakarakuji(context.Background(), prematureTime)
	if !errors.Is(err, lottery.ErrNotReadyToDraw) {
		t.Fatalf("expected ErrNotReadyToDraw, got: %v", err)
	}
}

func TestDrawTakarakuji_DepotFull(t *testing.T) {
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

	// 20 tickets all belonging to char-full to guarantee char-full wins
	tickets := make([]lottery.TakarakujiTicket, 20)
	for i := 0; i < 20; i++ {
		tickets[i] = lottery.TakarakujiTicket{
			ID:          "t-" + string(rune('a'+i)),
			RoundID:     1,
			CharacterID: "char-full",
		}
	}

	var settled bool
	var nextRoundCreated bool

	repo := &mockLotteryRepo{
		getActiveTakarakujiRoundFn: func(ctx context.Context) (lottery.TakarakujiRound, error) {
			return activeRound, nil
		},
		listRoundTakarakujiTicketsFn: func(ctx context.Context, roundID int) ([]lottery.TakarakujiTicket, error) {
			return tickets, nil
		},
		settleTakarakujiRoundFn: func(ctx context.Context, roundID int, drawnAt time.Time, winningTickets []lottery.TakarakujiTicket) error {
			settled = true
			return nil
		},
		createTakarakujiRoundFn: func(ctx context.Context, round lottery.TakarakujiRound) (lottery.TakarakujiRound, error) {
			nextRoundCreated = true
			return round, nil
		},
	}

	// Create depot filled to max capacity (5/5)
	fullDepot, err := depot.NewDepotWithCapacity("char-full", 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		inst, err := coreitem.NewInstance(fmt.Sprintf("weapon-%d", i+1), 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := fullDepot.AddItem(inst); err != nil {
			t.Fatal(err)
		}
	}

	depotRepo := &mockDepotRepo{
		depot: map[string]depot.Depot{
			"char-full": fullDepot,
		},
	}

	txProv := &mockTxProvider{}
	svc, err := lottery.NewService(repo,
		lottery.WithDepotRepository(depotRepo),
		lottery.WithTransactionProvider(txProv),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.DrawTakarakuji(context.Background(), drawTime)
	if err == nil {
		t.Fatal("expected error due to full depot, got nil")
	}
	if !errors.Is(err, depot.ErrDepotFull) {
		t.Fatalf("expected error wrapping depot.ErrDepotFull, got: %v", err)
	}

	// Assert transaction rollback and no round settlement / next round creation
	if !txProv.called {
		t.Error("expected transaction provider to be invoked")
	}
	if !txProv.rolledBack {
		t.Error("expected transaction to be rolled back")
	}
	if txProv.committed {
		t.Error("expected transaction NOT to be committed")
	}
	if settled {
		t.Error("round should NOT have been settled when depot delivery failed")
	}
	if nextRoundCreated {
		t.Error("next round should NOT have been created when depot delivery failed")
	}
}

func TestEvaluateRaffleRoll(t *testing.T) {
	t.Run("Standard Raffle - Day of Week Grand Prizes", func(t *testing.T) {
		expectedGrandPrizes := []struct {
			wday   int
			itemID string
			name   string
		}{
			{wday: 0, itemID: "item-027", name: "賢者の悟り"},
			{wday: 1, itemID: "item-035", name: "ドラゴンの心"},
			{wday: 2, itemID: "item-036", name: "闇のロザリオ"},
			{wday: 3, itemID: "item-088", name: "魔銃"},
			{wday: 4, itemID: "item-037", name: "ギザールの野菜"},
			{wday: 5, itemID: "item-038", name: "クポの実"},
			{wday: 6, itemID: "item-039", name: "ギャンブルハート"},
		}

		for _, tc := range expectedGrandPrizes {
			p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, 0, tc.wday)
			if p.Tier != lottery.PrizeTierGrand {
				t.Errorf("wday %d: expected Grand Prize, got %s", tc.wday, p.Tier)
			}
			if p.ItemDefinitionID != tc.itemID {
				t.Errorf("wday %d: expected item %s, got %s", tc.wday, tc.itemID, p.ItemDefinitionID)
			}
			if p.Name != tc.name {
				t.Errorf("wday %d: expected name %s, got %s", tc.wday, tc.name, p.Name)
			}
			if p.ColorName != "金" {
				t.Errorf("expected gold color, got %s", p.ColorName)
			}
		}
	})

	t.Run("Standard Raffle - All Tiers", func(t *testing.T) {
		// 1st Prize: item-030 (rolls 1, 2, 3)
		for _, r := range []int{1, 2, 3} {
			p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, r, 0)
			if p.Tier != lottery.PrizeTier1st || p.ItemDefinitionID != "item-030" || p.ColorName != "赤" {
				t.Errorf("roll %d: expected 1st prize item-030, got %+v", r, p)
			}
		}

		// 2nd Prize: item-033 (rolls 4..7)
		for _, r := range []int{4, 7} {
			p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, r, 0)
			if p.Tier != lottery.PrizeTier2nd || p.ItemDefinitionID != "item-033" || p.ColorName != "紫" {
				t.Errorf("roll %d: expected 2nd prize item-033, got %+v", r, p)
			}
		}

		// 3rd Prize: item-023 (rolls 8..13)
		for _, r := range []int{8, 13} {
			p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, r, 0)
			if p.Tier != lottery.PrizeTier3rd || p.ItemDefinitionID != "item-023" || p.ColorName != "黄" {
				t.Errorf("roll %d: expected 3rd prize item-023, got %+v", r, p)
			}
		}

		// 4th Prize - Seeds (rolls 14..44)
		seedTests := []struct {
			rolls  []int
			itemID string
			name   string
		}{
			{rolls: []int{14, 19}, itemID: "item-016", name: "命の木の実"},
			{rolls: []int{20, 24}, itemID: "item-017", name: "不思議な木の実"},
			{rolls: []int{25, 29}, itemID: "item-018", name: "力の種"},
			{rolls: []int{30, 34}, itemID: "item-019", name: "守りの種"},
			{rolls: []int{35, 39}, itemID: "item-020", name: "素早さの種"},
			{rolls: []int{40, 44}, itemID: "item-021", name: "スキルの種"},
		}
		for _, st := range seedTests {
			for _, r := range st.rolls {
				p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, r, 0)
				if p.Tier != lottery.PrizeTier4th || p.ItemDefinitionID != st.itemID || p.Name != st.name || p.ColorName != "桃" {
					t.Errorf("roll %d: expected 4th prize %s (%s), got %+v", r, st.name, st.itemID, p)
				}
			}
		}

		// 5th Prize: item-012 (rolls 45..54)
		for _, r := range []int{45, 54} {
			p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, r, 0)
			if p.Tier != lottery.PrizeTier5th || p.ItemDefinitionID != "item-012" || p.ColorName != "青" {
				t.Errorf("roll %d: expected 5th prize item-012, got %+v", r, p)
			}
		}

		// 6th Prize: item-125 (rolls 55..74)
		for _, r := range []int{55, 74} {
			p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, r, 0)
			if p.Tier != lottery.PrizeTier6th || p.ItemDefinitionID != "item-125" || p.ColorName != "緑" {
				t.Errorf("roll %d: expected 6th prize item-125, got %+v", r, p)
			}
		}

		// Miss: rolls 75..999
		for _, r := range []int{75, 500, 999} {
			p := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, r, 0)
			if p.Tier != lottery.PrizeTierMiss || p.ItemDefinitionID != "" || p.ColorName != "白" {
				t.Errorf("roll %d: expected Miss, got %+v", r, p)
			}
		}
	})

	t.Run("Special Raffle - Orbs & Materials", func(t *testing.T) {
		// Grand Prize: rolls 0..2 (materials)
		p0 := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, 0, 0, 0)
		if p0.Tier != lottery.PrizeTierGrand || p0.ItemDefinitionID != "item-090" || p0.ColorName != "ゴールド" {
			t.Errorf("expected material item-090, got %+v", p0)
		}

		p142 := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, 2, 0, 11)
		if p142.Tier != lottery.PrizeTierGrand || p142.ItemDefinitionID != "item-142" || p142.Name != "蝶の翅" {
			t.Errorf("expected material item-142, got %+v", p142)
		}

		// Orbs
		orbTests := []struct {
			rolls     []int
			tier      string
			itemID    string
			name      string
			colorName string
		}{
			{rolls: []int{3, 14}, tier: lottery.PrizeTier1st, itemID: "item-060", name: "シルバーオーブ", colorName: "シルバー"},
			{rolls: []int{15, 29}, tier: lottery.PrizeTier2nd, itemID: "item-061", name: "レッドオーブ", colorName: "レッド"},
			{rolls: []int{30, 39}, tier: lottery.PrizeTier3rd, itemID: "item-062", name: "ブルーオーブ", colorName: "ブルー"},
			{rolls: []int{40, 49}, tier: lottery.PrizeTier4th, itemID: "item-063", name: "グリーンオーブ", colorName: "グリーン"},
			{rolls: []int{50, 59}, tier: lottery.PrizeTier5th, itemID: "item-064", name: "イエローオーブ", colorName: "イエロー"},
			{rolls: []int{60, 69}, tier: lottery.PrizeTier6th, itemID: "item-065", name: "パープルオーブ", colorName: "パープル"},
		}

		for _, ot := range orbTests {
			for _, r := range ot.rolls {
				p := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, r, 0)
				if p.Tier != ot.tier || p.ItemDefinitionID != ot.itemID || p.Name != ot.name || p.ColorName != ot.colorName {
					t.Errorf("roll %d: expected %s (%s), got %+v", r, ot.name, ot.itemID, p)
				}
			}
		}

		// Miss: rolls 70..99
		for _, r := range []int{70, 90, 99} {
			p := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, r, 0)
			if p.Tier != lottery.PrizeTierMiss || p.ItemDefinitionID != "" || p.ColorName != "ホワイト" {
				t.Errorf("roll %d: expected Miss, got %+v", r, p)
			}
		}
	})
}

func TestPlayRaffle(t *testing.T) {
	ctx := context.Background()

	t.Run("Standard Raffle - Empty Hand Delivers to Inventory", func(t *testing.T) {
		repo := &mockLotteryRepo{
			getRaffleTicketsFn: func(ctx context.Context, charID string) (int, error) {
				return 10, nil
			},
			useRaffleTicketsFn: func(ctx context.Context, charID string, count int) (int, error) {
				if count != 3 {
					t.Errorf("expected count 3, got %d", count)
				}
				return 7, nil
			},
		}

		charRepo := &mockCharacterRepo{
			characters: map[string]corecharacter.Character{
				"char-1": {ID: "char-1", JobLevel: 5},
			},
		}

		invRepo := &mockInventoryRepo{}
		depotRepo := &mockDepotRepo{depot: make(map[string]depot.Depot)}

		svc, err := lottery.NewService(repo,
			lottery.WithCharacterRepository(charRepo),
			lottery.WithInventoryRepository(invRepo),
			lottery.WithDepotRepository(depotRepo),
		)
		if err != nil {
			t.Fatal(err)
		}

		res, remaining, _, err := svc.PlayRaffle(ctx, "char-1", lottery.RaffleStandard)
		if err != nil {
			t.Fatalf("PlayRaffle failed: %v", err)
		}

		if remaining != 7 {
			t.Errorf("remaining tickets = %d; want 7", remaining)
		}
		if res.TicketsUsed != 3 {
			t.Errorf("tickets used = %d; want 3", res.TicketsUsed)
		}

		// Check destination: if won an item, hand was empty so should be in inventory
		if res.Prize.ItemDefinitionID != "" {
			if res.TransferredToDepot {
				t.Errorf("expected item delivered to inventory, but transferred_to_depot is true")
			}
			inv, err := invRepo.FindByCharacterID(ctx, "char-1")
			if err != nil {
				t.Fatal(err)
			}
			if len(inv.Items) != 1 || inv.Items[0].DefinitionID != res.Prize.ItemDefinitionID {
				t.Errorf("expected %s in inventory, got %+v", res.Prize.ItemDefinitionID, inv.Items)
			}
		}
	})

	t.Run("Standard Raffle - Occupied Hand Delivers to Depot", func(t *testing.T) {
		repo := &mockLotteryRepo{
			getRaffleTicketsFn: func(ctx context.Context, charID string) (int, error) {
				return 10, nil
			},
			useRaffleTicketsFn: func(ctx context.Context, charID string, count int) (int, error) {
				return 7, nil
			},
		}

		charRepo := &mockCharacterRepo{
			characters: map[string]corecharacter.Character{
				"char-1": {ID: "char-1", JobLevel: 5},
			},
		}

		// Existing consumable in hand
		existingItem, _ := coreitem.NewInstance("item-001", 1)
		inv := coreinventory.Inventory{
			CharacterID: "char-1",
			Items:       []coreitem.Instance{existingItem},
		}
		invRepo := &mockInventoryRepo{
			inventories: map[string]coreinventory.Inventory{"char-1": inv},
		}
		depotRepo := &mockDepotRepo{depot: make(map[string]depot.Depot)}

		svc, err := lottery.NewService(repo,
			lottery.WithCharacterRepository(charRepo),
			lottery.WithInventoryRepository(invRepo),
			lottery.WithDepotRepository(depotRepo),
		)
		if err != nil {
			t.Fatal(err)
		}

		// Keep rolling until we hit an item win (<= 75/1000 chance) to verify depot delivery
		hitItem := false
		for i := 0; i < 200; i++ {
			res, _, _, err := svc.PlayRaffle(ctx, "char-1", lottery.RaffleStandard)
			if err != nil {
				t.Fatalf("PlayRaffle failed: %v", err)
			}
			if res.Prize.ItemDefinitionID != "" {
				hitItem = true
				if !res.TransferredToDepot {
					t.Errorf("expected item delivered to depot when hand occupied, got transferred_to_depot=false")
				}
				dp, err := depotRepo.FindByCharacterID(ctx, "char-1")
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, it := range dp.Items {
					if it.DefinitionID == res.Prize.ItemDefinitionID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected won item %s in depot, got %+v", res.Prize.ItemDefinitionID, dp.Items)
				}
				break
			}
		}
		if !hitItem {
			t.Log("Note: did not hit item win in 200 trials (random test)")
		}
	})

	t.Run("Standard Raffle - Insufficient Tickets", func(t *testing.T) {
		repo := &mockLotteryRepo{
			getRaffleTicketsFn: func(ctx context.Context, charID string) (int, error) {
				return 2, nil
			},
			useRaffleTicketsFn: func(ctx context.Context, charID string, count int) (int, error) {
				return 0, lottery.ErrInsufficientTickets
			},
		}
		svc, _ := lottery.NewService(repo)
		_, _, _, err := svc.PlayRaffle(ctx, "char-1", lottery.RaffleStandard)
		if !errors.Is(err, lottery.ErrInsufficientTickets) {
			t.Errorf("expected ErrInsufficientTickets, got %v", err)
		}
	})

	t.Run("Special Raffle - Insufficient Tickets for 300 requirement", func(t *testing.T) {
		repo := &mockLotteryRepo{
			getRaffleTicketsFn: func(ctx context.Context, charID string) (int, error) {
				return 100, nil
			},
			useRaffleTicketsFn: func(ctx context.Context, charID string, count int) (int, error) {
				if count != 300 {
					t.Errorf("expected count 300, got %d", count)
				}
				return 0, lottery.ErrInsufficientTickets
			},
		}
		svc, _ := lottery.NewService(repo)
		_, _, _, err := svc.PlayRaffle(ctx, "char-1", lottery.RaffleSpecial)
		if !errors.Is(err, lottery.ErrInsufficientTickets) {
			t.Errorf("expected ErrInsufficientTickets, got %v", err)
		}
	})
}
