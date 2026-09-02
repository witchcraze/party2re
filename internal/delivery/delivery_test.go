package delivery

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type mockTxProvider struct{}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{chars: make(map[string]corecharacter.Character)}
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

func (m *mockCharRepo) Update(ctx context.Context, value corecharacter.Character) error {
	m.chars[value.ID] = value
	return nil
}

type mockInvRepo struct {
	invs map[string]coreinventory.Inventory
}

func newMockInvRepo() *mockInvRepo {
	return &mockInvRepo{invs: make(map[string]coreinventory.Inventory)}
}

func (m *mockInvRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.invs[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
		m.invs[characterID] = inv
	}
	return inv, nil
}

func (m *mockInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInvRepo) Save(ctx context.Context, value coreinventory.Inventory) error {
	m.invs[value.CharacterID] = value
	return nil
}

type mockDeliveryRepo struct {
	quests     map[string]Quest
	deliveries map[string]CharacterDelivery
	parcels    map[string]Parcel
}

func newMockDeliveryRepo() *mockDeliveryRepo {
	return &mockDeliveryRepo{
		quests:     make(map[string]Quest),
		deliveries: make(map[string]CharacterDelivery),
		parcels:    make(map[string]Parcel),
	}
}

func (m *mockDeliveryRepo) GetAvailableQuests(ctx context.Context, now time.Time) ([]Quest, error) {
	var list []Quest
	for _, q := range m.quests {
		if q.ExpiresAt.After(now) {
			list = append(list, q)
		}
	}
	return list, nil
}

func (m *mockDeliveryRepo) GetQuestByID(ctx context.Context, id string) (*Quest, error) {
	q, ok := m.quests[id]
	if !ok {
		return nil, ErrQuestNotFound
	}
	return &q, nil
}

func (m *mockDeliveryRepo) SaveQuest(ctx context.Context, q *Quest) error {
	m.quests[q.ID] = *q
	return nil
}

func (m *mockDeliveryRepo) SaveQuests(ctx context.Context, quests []Quest) error {
	for _, q := range quests {
		m.quests[q.ID] = q
	}
	return nil
}

func (m *mockDeliveryRepo) GetCharacterDeliveries(ctx context.Context, characterID string) ([]CharacterDelivery, error) {
	var list []CharacterDelivery
	for _, d := range m.deliveries {
		if d.CharacterID == characterID {
			item := d
			if q, ok := m.quests[d.QuestID]; ok {
				item.Quest = &q
			}
			list = append(list, item)
		}
	}
	return list, nil
}

func (m *mockDeliveryRepo) GetActiveCharacterDeliveries(ctx context.Context, characterID string) ([]CharacterDelivery, error) {
	var list []CharacterDelivery
	for _, d := range m.deliveries {
		if d.CharacterID == characterID && d.Status == StatusInProgress {
			item := d
			if q, ok := m.quests[d.QuestID]; ok {
				item.Quest = &q
			}
			list = append(list, item)
		}
	}
	return list, nil
}

func (m *mockDeliveryRepo) GetCharacterDeliveryByID(ctx context.Context, id string) (*CharacterDelivery, error) {
	d, ok := m.deliveries[id]
	if !ok {
		return nil, ErrDeliveryNotFound
	}
	return &d, nil
}

func (m *mockDeliveryRepo) SaveCharacterDelivery(ctx context.Context, d *CharacterDelivery) error {
	m.deliveries[d.ID] = *d
	return nil
}

func (m *mockDeliveryRepo) UpdateCharacterDelivery(ctx context.Context, d *CharacterDelivery) error {
	m.deliveries[d.ID] = *d
	return nil
}

func (m *mockDeliveryRepo) SaveParcel(ctx context.Context, p *Parcel) error {
	m.parcels[p.ID] = *p
	return nil
}

func (m *mockDeliveryRepo) GetParcelByID(ctx context.Context, id string) (*Parcel, error) {
	p, ok := m.parcels[id]
	if !ok {
		return nil, ErrParcelNotFound
	}
	return &p, nil
}

func (m *mockDeliveryRepo) GetParcelByIDForUpdate(ctx context.Context, id string) (*Parcel, error) {
	return m.GetParcelByID(ctx, id)
}

func (m *mockDeliveryRepo) GetIncomingParcels(ctx context.Context, recipientCharacterID string) ([]Parcel, error) {
	var list []Parcel
	for _, p := range m.parcels {
		if p.RecipientCharacterID == recipientCharacterID && p.Status == ParcelStatusPending {
			list = append(list, p)
		}
	}
	return list, nil
}

func (m *mockDeliveryRepo) GetIncomingParcelsByCursor(ctx context.Context, recipientCharacterID string, limit int, beforeTime time.Time, beforeID string) ([]Parcel, error) {
	var list []Parcel
	for _, p := range m.parcels {
		if p.RecipientCharacterID == recipientCharacterID && p.Status == ParcelStatusPending {
			if beforeTime.IsZero() && beforeID == "" {
				list = append(list, p)
			} else if !beforeTime.IsZero() && beforeID != "" {
				if p.CreatedAt.Before(beforeTime) || (p.CreatedAt.Equal(beforeTime) && p.ID < beforeID) {
					list = append(list, p)
				}
			} else if !beforeTime.IsZero() {
				if p.CreatedAt.Before(beforeTime) {
					list = append(list, p)
				}
			} else {
				if p.ID < beforeID {
					list = append(list, p)
				}
			}
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockDeliveryRepo) GetSentParcels(ctx context.Context, senderCharacterID string) ([]Parcel, error) {
	var list []Parcel
	for _, p := range m.parcels {
		if p.SenderCharacterID == senderCharacterID {
			list = append(list, p)
		}
	}
	return list, nil
}

func (m *mockDeliveryRepo) GetSentParcelsByCursor(ctx context.Context, senderCharacterID string, limit int, beforeTime time.Time, beforeID string) ([]Parcel, error) {
	var list []Parcel
	for _, p := range m.parcels {
		if p.SenderCharacterID == senderCharacterID {
			if beforeTime.IsZero() && beforeID == "" {
				list = append(list, p)
			} else if !beforeTime.IsZero() && beforeID != "" {
				if p.CreatedAt.Before(beforeTime) || (p.CreatedAt.Equal(beforeTime) && p.ID < beforeID) {
					list = append(list, p)
				}
			} else if !beforeTime.IsZero() {
				if p.CreatedAt.Before(beforeTime) {
					list = append(list, p)
				}
			} else {
				if p.ID < beforeID {
					list = append(list, p)
				}
			}
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockDeliveryRepo) UpdateParcel(ctx context.Context, p *Parcel) error {
	existing, ok := m.parcels[p.ID]
	if !ok || existing.Status != ParcelStatusPending {
		return ErrParcelAlreadyClaimed
	}
	m.parcels[p.ID] = *p
	return nil
}

type mockItemDefs struct {
	defs map[string]coreitem.Definition
}

func (m *mockItemDefs) FindByID(id string) (coreitem.Definition, error) {
	d, ok := m.defs[id]
	if !ok {
		return coreitem.Definition{}, errors.New("not found")
	}
	return d, nil
}

func setupDeliveryTest(t *testing.T) (*Service, *mockDeliveryRepo, *mockCharRepo, *mockInvRepo) {
	dRepo := newMockDeliveryRepo()
	cRepo := newMockCharRepo()
	iRepo := newMockInvRepo()
	itemDefs := &mockItemDefs{
		defs: map[string]coreitem.Definition{
			"item-001":  {ID: "item-001", Name: "薬草"},
			"item-007":  {ID: "item-007", Name: "毒消し草"},
			"weapon-01": {ID: "weapon-01", Name: "ヒノキの棒"},
		},
	}

	svc, err := NewService(
		dRepo,
		cRepo,
		iRepo,
		WithTransactionProvider(&mockTxProvider{}),
		WithItemDefinitionProvider(itemDefs),
	)
	if err != nil {
		t.Fatalf("failed to create delivery service: %v", err)
	}

	return svc, dRepo, cRepo, iRepo
}

func TestGetAvailableQuestsAutoGeneration(t *testing.T) {
	svc, repo, _, _ := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	quests, err := svc.GetAvailableQuests(ctx, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quests) < 5 {
		t.Fatalf("expected at least 5 quests, got %d", len(quests))
	}
	if len(repo.quests) < 5 {
		t.Fatalf("expected quests to be saved to repo, got %d", len(repo.quests))
	}
}

func TestAcceptAndCompleteDeliveryQuest(t *testing.T) {
	svc, _, cRepo, iRepo := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	// 1. Setup Character
	charID := "char-001"
	cRepo.chars[charID] = corecharacter.Character{
		ID:         charID,
		Name:       "勇者アレン",
		Money:      500,
		Experience: 1000,
		Level:      5,
	}

	// 2. Setup Inventory with target items
	inv, _ := coreinventory.New(charID)
	_ = inv.Add(coreitem.Instance{
		ID:           "inst-001",
		DefinitionID: "item-001",
		Quantity:     5,
	})
	_ = iRepo.Save(ctx, inv)

	// 3. Get available quests
	quests, err := svc.GetAvailableQuests(ctx, now)
	if err != nil {
		t.Fatalf("failed to get quests: %v", err)
	}
	targetQuest := quests[0]

	// Adjust inventory if quest needs different item
	inv, _ = iRepo.FindByCharacterID(ctx, charID)
	_ = inv.Add(coreitem.Instance{
		ID:           "inst-target",
		DefinitionID: targetQuest.TargetItemID,
		Quantity:     targetQuest.RequiredQuantity,
	})
	_ = iRepo.Save(ctx, inv)

	// 4. Accept Quest
	delivery, err := svc.AcceptQuest(ctx, charID, targetQuest.ID, now)
	if err != nil {
		t.Fatalf("failed to accept quest: %v", err)
	}
	if delivery.Status != StatusInProgress {
		t.Fatalf("expected in_progress status, got %s", delivery.Status)
	}

	// Duplicate accept should fail
	_, err = svc.AcceptQuest(ctx, charID, targetQuest.ID, now)
	if !errors.Is(err, ErrAlreadyAccepted) {
		t.Fatalf("expected ErrAlreadyAccepted, got %v", err)
	}

	// 5. Complete Delivery
	result, err := svc.CompleteDelivery(ctx, charID, delivery.ID, now)
	if err != nil {
		t.Fatalf("failed to complete delivery: %v", err)
	}

	if result.RewardedGold != targetQuest.RewardGold {
		t.Errorf("expected reward gold %d, got %d", targetQuest.RewardGold, result.RewardedGold)
	}
	if result.RewardedExp != targetQuest.RewardExp {
		t.Errorf("expected reward exp %d, got %d", targetQuest.RewardExp, result.RewardedExp)
	}

	// Verify updated character stats
	updatedChar, _ := cRepo.FindByID(ctx, charID)
	if updatedChar.Money != 500+targetQuest.RewardGold {
		t.Errorf("expected char money %d, got %d", 500+targetQuest.RewardGold, updatedChar.Money)
	}

	// Attempting to complete already completed delivery should fail
	_, err = svc.CompleteDelivery(ctx, charID, delivery.ID, now)
	if !errors.Is(err, ErrDeliveryNotActive) {
		t.Fatalf("expected ErrDeliveryNotActive, got %v", err)
	}
}

func TestAcceptQuestLimits(t *testing.T) {
	svc, _, cRepo, _ := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	charID := "char-max-test"
	cRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "テスト"}

	quests, _ := svc.GetAvailableQuests(ctx, now)

	// Accept up to MaxActiveDeliveries (3)
	for i := 0; i < 3; i++ {
		_, err := svc.AcceptQuest(ctx, charID, quests[i].ID, now)
		if err != nil {
			t.Fatalf("failed to accept quest %d: %v", i, err)
		}
	}

	// 4th accept should fail with ErrMaxActiveDeliveries
	_, err := svc.AcceptQuest(ctx, charID, quests[3].ID, now)
	if !errors.Is(err, ErrMaxActiveDeliveries) {
		t.Fatalf("expected ErrMaxActiveDeliveries, got %v", err)
	}
}

func TestCancelDelivery(t *testing.T) {
	svc, _, cRepo, _ := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	charID := "char-cancel-test"
	cRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "テスト"}

	quests, _ := svc.GetAvailableQuests(ctx, now)
	del, err := svc.AcceptQuest(ctx, charID, quests[0].ID, now)
	if err != nil {
		t.Fatalf("failed to accept: %v", err)
	}

	// IDOR check: other character cannot cancel
	err = svc.CancelDelivery(ctx, "char-other", del.ID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// Cancel successfully
	err = svc.CancelDelivery(ctx, charID, del.ID)
	if err != nil {
		t.Fatalf("failed to cancel delivery: %v", err)
	}

	// Cancelling again fails
	err = svc.CancelDelivery(ctx, charID, del.ID)
	if !errors.Is(err, ErrDeliveryNotActive) {
		t.Fatalf("expected ErrDeliveryNotActive, got %v", err)
	}
}

func TestSendAndClaimParcel(t *testing.T) {
	svc, _, cRepo, iRepo := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	senderID := "sender-001"
	recipientID := "recipient-002"

	cRepo.chars[senderID] = corecharacter.Character{
		ID:    senderID,
		Name:  "送付者",
		Money: 1000,
	}
	cRepo.chars[recipientID] = corecharacter.Character{
		ID:    recipientID,
		Name:  "受取人",
		Money: 100,
	}

	senderInv, _ := coreinventory.New(senderID)
	_ = senderInv.Add(coreitem.Instance{
		ID:           "item-inst-123",
		DefinitionID: "item-001",
		Quantity:     3,
	})
	_ = iRepo.Save(ctx, senderInv)

	// 1. Self parcel rejected
	_, err := svc.SendParcel(ctx, senderID, SendParcelRequest{
		RecipientCharacterID: senderID,
		GoldAmount:           100,
	}, now)
	if !errors.Is(err, ErrSelfParcelNotAllowed) {
		t.Fatalf("expected ErrSelfParcelNotAllowed, got %v", err)
	}

	// 2. Insufficient gold (including fee) rejected
	_, err = svc.SendParcel(ctx, senderID, SendParcelRequest{
		RecipientCharacterID: recipientID,
		GoldAmount:           1000, // 1000 + 50 > 1000
	}, now)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// 3. Successful parcel sending
	parcel, err := svc.SendParcel(ctx, senderID, SendParcelRequest{
		RecipientCharacterID: recipientID,
		ItemInstanceID:       "item-inst-123",
		ItemQuantity:         2,
		GoldAmount:           200,
		Message:              "いつもありがとう！薬草とお小遣いです。",
	}, now)
	if err != nil {
		t.Fatalf("failed to send parcel: %v", err)
	}

	// Verify sender balance (1000 - 200 gold - 50 fee = 750)
	sender, _ := cRepo.FindByID(ctx, senderID)
	if sender.Money != 750 {
		t.Errorf("expected sender money 750, got %d", sender.Money)
	}

	// Verify incoming parcels
	incoming, err := svc.GetIncomingParcels(ctx, recipientID)
	if err != nil || len(incoming) != 1 {
		t.Fatalf("expected 1 incoming parcel, got %v, err: %v", incoming, err)
	}

	// 4. IDOR claim protection
	cRepo.chars["someone-else"] = corecharacter.Character{ID: "someone-else", Name: "他人"}
	_, err = svc.ClaimParcel(ctx, "someone-else", parcel.ID, now)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// 5. Successful claim
	claimRes, err := svc.ClaimParcel(ctx, recipientID, parcel.ID, now)
	if err != nil {
		t.Fatalf("failed to claim parcel: %v", err)
	}
	if claimRes.GoldAmount != 200 {
		t.Errorf("expected 200 gold, got %d", claimRes.GoldAmount)
	}

	// Recipient should have 100 + 200 = 300 gold
	recipient, _ := cRepo.FindByID(ctx, recipientID)
	if recipient.Money != 300 {
		t.Errorf("expected recipient money 300, got %d", recipient.Money)
	}

	// Recipient inventory should have 2 herbs
	recInv, _ := iRepo.FindByCharacterID(ctx, recipientID)
	if recInv.Quantity("item-001") != 2 {
		t.Errorf("expected 2 herbs in recipient inventory, got %d", recInv.Quantity("item-001"))
	}

	// 6. Claiming again rejected
	_, err = svc.ClaimParcel(ctx, recipientID, parcel.ID, now)
	if !errors.Is(err, ErrParcelAlreadyClaimed) {
		t.Fatalf("expected ErrParcelAlreadyClaimed, got %v", err)
	}
}

func TestCancelParcel(t *testing.T) {
	svc, _, cRepo, _ := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	senderID := "sender-002"
	recipientID := "recipient-003"

	cRepo.chars[senderID] = corecharacter.Character{
		ID:    senderID,
		Name:  "差出人",
		Money: 1000,
	}
	cRepo.chars[recipientID] = corecharacter.Character{
		ID:   recipientID,
		Name: "受取人",
	}

	parcel, err := svc.SendParcel(ctx, senderID, SendParcelRequest{
		RecipientCharacterID: recipientID,
		GoldAmount:           300,
	}, now)
	if err != nil {
		t.Fatalf("failed to send: %v", err)
	}

	// Sender money: 1000 - 300 - 50 = 650
	sender, _ := cRepo.FindByID(ctx, senderID)
	if sender.Money != 650 {
		t.Errorf("expected 650, got %d", sender.Money)
	}

	// Cancel parcel returns 300 gold to sender
	err = svc.CancelParcel(ctx, senderID, parcel.ID)
	if err != nil {
		t.Fatalf("failed to cancel parcel: %v", err)
	}

	sender, _ = cRepo.FindByID(ctx, senderID)
	if sender.Money != 950 { // 650 + 300 = 950 (courier fee 50 consumed)
		t.Errorf("expected 950 after cancel refund, got %d", sender.Money)
	}

	// Claiming cancelled parcel fails
	_, err = svc.ClaimParcel(ctx, recipientID, parcel.ID, now)
	if !errors.Is(err, ErrParcelAlreadyClaimed) {
		t.Fatalf("expected ErrParcelAlreadyClaimed, got %v", err)
	}
}

func TestGetIncomingParcelsByCursor(t *testing.T) {
	svc, dRepo, cRepo, _ := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	cRepo.chars["rec-1"] = corecharacter.Character{ID: "rec-1", Name: "Recipient"}
	for i := 1; i <= 5; i++ {
		pID := fmt.Sprintf("parcel-%d", i)
		dRepo.parcels[pID] = Parcel{
			ID:                   pID,
			SenderCharacterID:    "sender-1",
			SenderCharacterName:  "Sender",
			RecipientCharacterID: "rec-1",
			Status:               ParcelStatusPending,
			GoldAmount:           100 * i,
			CreatedAt:            now.Add(time.Duration(i) * time.Minute),
		}
	}

	page1, err := svc.GetIncomingParcelsByCursor(ctx, "rec-1", 2, "")
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1.Items) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("unexpected page 1: %+v", page1)
	}
}

func TestGetSentParcelsByCursor(t *testing.T) {
	svc, dRepo, cRepo, _ := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	cRepo.chars["sender-1"] = corecharacter.Character{ID: "sender-1", Name: "Sender"}
	for i := 1; i <= 4; i++ {
		pID := fmt.Sprintf("sent-%d", i)
		dRepo.parcels[pID] = Parcel{
			ID:                   pID,
			SenderCharacterID:    "sender-1",
			SenderCharacterName:  "Sender",
			RecipientCharacterID: "rec-1",
			Status:               ParcelStatusClaimed,
			GoldAmount:           100 * i,
			CreatedAt:            now.Add(time.Duration(i) * time.Minute),
		}
	}

	page1, err := svc.GetSentParcelsByCursor(ctx, "sender-1", 2, "")
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1.Items) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("unexpected page 1: %+v", page1)
	}
}
