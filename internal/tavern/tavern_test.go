package tavern_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/tavern"
)

type mockTxProvider struct{}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	m.chars[char.ID] = char
	return nil
}

type mockTavernRepo struct {
	statuses   map[string]tavern.TavernCharacterStatus
	deliveries map[string]tavern.DeliveryReservation
}

func newMockTavernRepo() *mockTavernRepo {
	return &mockTavernRepo{
		statuses:   make(map[string]tavern.TavernCharacterStatus),
		deliveries: make(map[string]tavern.DeliveryReservation),
	}
}

func (m *mockTavernRepo) GetCharacterStatus(ctx context.Context, characterID string) (tavern.TavernCharacterStatus, error) {
	s, ok := m.statuses[characterID]
	if !ok {
		return tavern.TavernCharacterStatus{CharacterID: characterID, IsFull: false}, nil
	}
	return s, nil
}

func (m *mockTavernRepo) UpsertCharacterStatus(ctx context.Context, status tavern.TavernCharacterStatus) error {
	m.statuses[status.CharacterID] = status
	return nil
}

func (m *mockTavernRepo) GetDelivery(ctx context.Context, characterID string) (tavern.DeliveryReservation, error) {
	d, ok := m.deliveries[characterID]
	if !ok {
		return tavern.DeliveryReservation{}, tavern.ErrNoActiveDelivery
	}
	return d, nil
}

func (m *mockTavernRepo) SaveDelivery(ctx context.Context, delivery tavern.DeliveryReservation) error {
	m.deliveries[delivery.CharacterID] = delivery
	return nil
}

func (m *mockTavernRepo) DeleteDelivery(ctx context.Context, characterID string) error {
	delete(m.deliveries, characterID)
	return nil
}

type mockLotteryRepo struct {
	tickets map[string]int
}

func (m *mockLotteryRepo) AddRaffleTickets(ctx context.Context, characterID string, count int) (int, error) {
	m.tickets[characterID] += count
	return m.tickets[characterID], nil
}

func (m *mockLotteryRepo) GetRaffleTickets(ctx context.Context, characterID string) (int, error) {
	return m.tickets[characterID], nil
}

func setupTestService(t *testing.T) (*tavern.Service, *mockCharRepo, *mockTavernRepo, *mockLotteryRepo) {
	catalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	tavernRepo := newMockTavernRepo()
	lotteryRepo := &mockLotteryRepo{tickets: make(map[string]int)}
	txProvider := &mockTxProvider{}

	svc, err := tavern.NewService(
		catalog,
		tavernRepo,
		charRepo,
		txProvider,
		tavern.WithLotteryRepository(lotteryRepo),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	return svc, charRepo, tavernRepo, lotteryRepo
}

func TestCatalog_LoadAndLookup(t *testing.T) {
	cat, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatalf("LoadDefaultCatalog failed: %v", err)
	}

	items := cat.Items()
	if len(items) != 14 {
		t.Errorf("expected 14 menu items, got %d", len(items))
	}

	curry, found := cat.GetItem("tavern_curry")
	if !found {
		t.Fatal("expected to find tavern_curry")
	}
	if curry.Price != 400 || curry.HPHeal != 250 {
		t.Errorf("unexpected curry stats: %+v", curry)
	}

	_, found = cat.GetItem("non_existent")
	if found {
		t.Error("expected non_existent item to not be found")
	}
}

func TestTavern_GetStatus(t *testing.T) {
	svc, charRepo, tavernRepo, _ := setupTestService(t)
	ctx := context.Background()

	charID := "char-1"
	charRepo.chars[charID] = corecharacter.Character{
		ID:    charID,
		Name:  "Hero",
		Money: 1000,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 100, MP: 20, MaxMP: 50},
	}

	status, err := svc.GetStatus(ctx, charID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.CharacterName != "Hero" || status.Gold != 1000 || status.IsFull {
		t.Errorf("unexpected status: %+v", status)
	}

	// Set full status
	now := time.Now().UTC()
	tavernRepo.statuses[charID] = tavern.TavernCharacterStatus{
		CharacterID: charID,
		IsFull:      true,
		LastEatenAt: &now,
	}

	statusFull, err := svc.GetStatus(ctx, charID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !statusFull.IsFull {
		t.Errorf("expected character to be full")
	}
}

func TestTavern_OrderMeal_Success(t *testing.T) {
	svc, charRepo, _, lotteryRepo := setupTestService(t)
	ctx := context.Background()

	charID := "char-1"
	charRepo.chars[charID] = corecharacter.Character{
		ID:    charID,
		Name:  "Hero",
		Money: 1000,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 100, MP: 10, MaxMP: 50},
	}

	// Order curry (Price: 400, HPHeal: 250, MPHeal: 0, Tickets: 5)
	res, err := svc.OrderMeal(ctx, charID, "tavern_curry")
	if err != nil {
		t.Fatalf("OrderMeal failed: %v", err)
	}

	if res.HPHealed != 50 { // capped at MaxHP - HP = 100 - 50
		t.Errorf("expected HPHealed 50, got %d", res.HPHealed)
	}
	if res.CurrentHP != 100 {
		t.Errorf("expected CurrentHP 100, got %d", res.CurrentHP)
	}
	if res.RemainingGold != 600 {
		t.Errorf("expected RemainingGold 600, got %d", res.RemainingGold)
	}
	if res.TicketsAwarded != 5 || res.TotalTickets != 5 {
		t.Errorf("expected 5 tickets, got %d total", res.TotalTickets)
	}
	if lotteryRepo.tickets[charID] != 5 {
		t.Errorf("expected lottery repo tickets 5, got %d", lotteryRepo.tickets[charID])
	}

	// Ordering again should fail because character is full
	_, err = svc.OrderMeal(ctx, charID, "tavern_water")
	if !errors.Is(err, tavern.ErrAlreadyFull) {
		t.Errorf("expected ErrAlreadyFull, got %v", err)
	}
}

func TestTavern_OrderMeal_InsufficientFunds(t *testing.T) {
	svc, charRepo, _, _ := setupTestService(t)
	ctx := context.Background()

	charID := "char-1"
	charRepo.chars[charID] = corecharacter.Character{
		ID:    charID,
		Name:  "Poor Hero",
		Money: 10,
		Stats: corecharacter.Stats{HP: 50, MaxHP: 100},
	}

	_, err := svc.OrderMeal(ctx, charID, "tavern_curry")
	if !errors.Is(err, tavern.ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestTavern_DeliveryFlow(t *testing.T) {
	svc, charRepo, tavernRepo, _ := setupTestService(t)
	ctx := context.Background()

	charID := "char-1"
	charRepo.chars[charID] = corecharacter.Character{
		ID:    charID,
		Name:  "Hero",
		Money: 2000,
		Stats: corecharacter.Stats{HP: 20, MaxHP: 100, MP: 5, MaxMP: 50},
	}

	// 1. Reserve delivery for full course (Price: 3000 -> insufficient)
	_, err := svc.ReserveDelivery(ctx, charID, "tavern_full_course")
	if !errors.Is(err, tavern.ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}

	// 2. Reserve delivery for omelet rice (Price: 750, HPHeal: 500, MPHeal: 100, Tickets: 7)
	deliv, err := svc.ReserveDelivery(ctx, charID, "tavern_omelet_rice")
	if err != nil {
		t.Fatalf("ReserveDelivery failed: %v", err)
	}
	if deliv.ItemID != "tavern_omelet_rice" || deliv.Price != 750 {
		t.Errorf("unexpected delivery: %+v", deliv)
	}

	// 3. Get delivery
	delivFetched, err := svc.GetDelivery(ctx, charID)
	if err != nil {
		t.Fatalf("GetDelivery failed: %v", err)
	}
	if delivFetched.ItemID != deliv.ItemID {
		t.Errorf("mismatch delivery items")
	}

	// 4. Claim delivery after adventure
	res, err := svc.ClaimDelivery(ctx, charID)
	if err != nil {
		t.Fatalf("ClaimDelivery failed: %v", err)
	}
	if res.HPHealed != 80 || res.CurrentHP != 100 {
		t.Errorf("expected HP 100 (healed 80), got HP %d healed %d", res.CurrentHP, res.HPHealed)
	}
	if res.MPHealed != 45 || res.CurrentMP != 50 {
		t.Errorf("expected MP 50 (healed 45), got MP %d healed %d", res.CurrentMP, res.MPHealed)
	}
	if res.RemainingGold != 1250 {
		t.Errorf("expected RemainingGold 1250, got %d", res.RemainingGold)
	}

	// 5. Delivery should now be removed
	_, err = svc.GetDelivery(ctx, charID)
	if !errors.Is(err, tavern.ErrNoActiveDelivery) {
		t.Errorf("expected ErrNoActiveDelivery, got %v", err)
	}

	// 6. Character should now be full
	status, err := tavernRepo.GetCharacterStatus(ctx, charID)
	if err != nil || !status.IsFull {
		t.Errorf("expected character to be full after delivery claim")
	}

	// 7. Reset fullness
	if err := svc.ResetFullness(ctx, charID); err != nil {
		t.Fatalf("ResetFullness failed: %v", err)
	}
	statusReset, _ := tavernRepo.GetCharacterStatus(ctx, charID)
	if statusReset.IsFull {
		t.Errorf("expected character to not be full after reset")
	}
}

func TestTavern_CancelDelivery(t *testing.T) {
	svc, charRepo, _, _ := setupTestService(t)
	ctx := context.Background()

	charID := "char-1"
	charRepo.chars[charID] = corecharacter.Character{
		ID:    charID,
		Name:  "Hero",
		Money: 500,
	}

	// Cancel with no active delivery
	err := svc.CancelDelivery(ctx, charID)
	if !errors.Is(err, tavern.ErrNoActiveDelivery) {
		t.Errorf("expected ErrNoActiveDelivery, got %v", err)
	}

	// Reserve and then cancel
	_, err = svc.ReserveDelivery(ctx, charID, "tavern_water")
	if err != nil {
		t.Fatalf("ReserveDelivery failed: %v", err)
	}

	err = svc.CancelDelivery(ctx, charID)
	if err != nil {
		t.Fatalf("CancelDelivery failed: %v", err)
	}

	_, err = svc.GetDelivery(ctx, charID)
	if !errors.Is(err, tavern.ErrNoActiveDelivery) {
		t.Errorf("expected ErrNoActiveDelivery after cancel, got %v", err)
	}
}

func TestTavern_Talk(t *testing.T) {
	svc, charRepo, _, _ := setupTestService(t)
	ctx := context.Background()

	charID := "char-1"
	charRepo.chars[charID] = corecharacter.Character{
		ID:    charID,
		Name:  "Loto",
		Money: 100,
	}

	talkRes, err := svc.Talk(ctx, charID)
	if err != nil {
		t.Fatalf("Talk failed: %v", err)
	}
	if talkRes.NPCName != "@エレナ" || talkRes.LocationName != "冒険者の酒場" || talkRes.Message == "" {
		t.Errorf("unexpected TalkResult: %+v", talkRes)
	}
}
