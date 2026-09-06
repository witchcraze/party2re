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
	err  error
}

func newMockInvRepo() *mockInvRepo {
	return &mockInvRepo{invs: make(map[string]coreinventory.Inventory)}
}

func (m *mockInvRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if m.err != nil {
		return coreinventory.Inventory{}, m.err
	}
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
	quests          map[string]Quest
	deliveries      map[string]CharacterDelivery
	parcels         map[string]Parcel
	updateParcelErr error
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
	if m.updateParcelErr != nil {
		return m.updateParcelErr
	}
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

func TestCancelParcel_ErrorPathsAndItemRefund(t *testing.T) {
	svc, dRepo, cRepo, iRepo := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	senderID := "sender-003"
	recipientID := "recipient-004"
	cRepo.chars[senderID] = corecharacter.Character{ID: senderID, Name: "Sender", Money: 500}
	cRepo.chars[recipientID] = corecharacter.Character{ID: recipientID, Name: "Recipient", Money: 100}

	// 1. Invalid input
	if err := svc.CancelParcel(ctx, "", "parcel-1"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty senderID, got %v", err)
	}
	if err := svc.CancelParcel(ctx, "sender-003", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty parcelID, got %v", err)
	}

	// 2. Parcel not found
	if err := svc.CancelParcel(ctx, senderID, "nonexistent"); !errors.Is(err, ErrParcelNotFound) {
		t.Errorf("expected ErrParcelNotFound, got %v", err)
	}

	// Create a parcel
	pID := "test-parcel-item"
	dRepo.parcels[pID] = Parcel{
		ID:                   pID,
		SenderCharacterID:    senderID,
		SenderCharacterName:  "Sender",
		RecipientCharacterID: recipientID,
		ItemID:               "item-001",
		ItemName:             "薬草",
		ItemQuantity:         2,
		GoldAmount:           150,
		Status:               ParcelStatusPending,
		CreatedAt:            now,
	}

	// 3. Forbidden (another sender tries to cancel)
	if err := svc.CancelParcel(ctx, "other-sender", pID); !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}

	// 4. Sender not found in character repo
	ghostPID := "ghost-parcel"
	dRepo.parcels[ghostPID] = Parcel{
		ID:                   ghostPID,
		SenderCharacterID:    "ghost-sender",
		RecipientCharacterID: recipientID,
		Status:               ParcelStatusPending,
	}
	if err := svc.CancelParcel(ctx, "ghost-sender", ghostPID); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound for missing sender, got %v", err)
	}

	// 5. Inventory repo error
	iRepo.err = errors.New("inv lock failure")
	if err := svc.CancelParcel(ctx, senderID, pID); err == nil || !errors.Is(err, iRepo.err) {
		t.Errorf("expected inv lock failure, got %v", err)
	}
	iRepo.err = nil

	// 6. UpdateParcel failure
	dRepo.updateParcelErr = errors.New("db write failure")
	if err := svc.CancelParcel(ctx, senderID, pID); err == nil || !errors.Is(err, dRepo.updateParcelErr) {
		t.Errorf("expected db write failure, got %v", err)
	}
	dRepo.updateParcelErr = nil

	// Reset sender and parcel state before step 7 because in-memory test mock does not rollback on step 6 error
	cRepo.chars[senderID] = corecharacter.Character{ID: senderID, Name: "Sender", Money: 500}
	cleanInv, _ := coreinventory.New(senderID)
	_ = iRepo.Save(ctx, cleanInv)
	dRepo.parcels[pID] = Parcel{
		ID:                   pID,
		SenderCharacterID:    senderID,
		SenderCharacterName:  "Sender",
		RecipientCharacterID: recipientID,
		ItemID:               "item-001",
		ItemName:             "薬草",
		ItemQuantity:         2,
		GoldAmount:           150,
		Status:               ParcelStatusPending,
		CreatedAt:            now,
	}

	// 7. Successful cancellation with Item & Gold refund
	if err := svc.CancelParcel(ctx, senderID, pID); err != nil {
		t.Fatalf("unexpected cancel error: %v", err)
	}
	// Verify sender money refunded: 500 + 150 = 650
	sender, _ := cRepo.FindByID(ctx, senderID)
	if sender.Money != 650 {
		t.Errorf("expected 650 money after refund, got %d", sender.Money)
	}
	// Verify sender inventory received 2 herbs
	inv, _ := iRepo.FindByCharacterID(ctx, senderID)
	if inv.Quantity("item-001") != 2 {
		t.Errorf("expected 2 herbs refunded to sender, got %d", inv.Quantity("item-001"))
	}
	// Verify parcel status is cancelled
	if dRepo.parcels[pID].Status != ParcelStatusCancelled {
		t.Errorf("expected ParcelStatusCancelled, got %v", dRepo.parcels[pID].Status)
	}

	// 8. Cannot cancel already cancelled parcel
	if err := svc.CancelParcel(ctx, senderID, pID); !errors.Is(err, ErrParcelAlreadyClaimed) {
		t.Errorf("expected ErrParcelAlreadyClaimed on already cancelled parcel, got %v", err)
	}

	// 9. Cannot cancel claimed parcel
	claimedPID := "claimed-parcel"
	dRepo.parcels[claimedPID] = Parcel{
		ID:                   claimedPID,
		SenderCharacterID:    senderID,
		RecipientCharacterID: recipientID,
		Status:               ParcelStatusClaimed,
	}
	if err := svc.CancelParcel(ctx, senderID, claimedPID); !errors.Is(err, ErrParcelAlreadyClaimed) {
		t.Errorf("expected ErrParcelAlreadyClaimed on claimed parcel, got %v", err)
	}

	// 10. Direct execution when txProvider is nil
	itemDefs := &mockItemDefs{defs: map[string]coreitem.Definition{"item-001": {ID: "item-001", Name: "薬草"}}}
	noTxSvc, _ := NewService(dRepo, cRepo, iRepo, WithItemDefinitionProvider(itemDefs))
	noTxPID := "no-tx-parcel"
	dRepo.parcels[noTxPID] = Parcel{
		ID:                   noTxPID,
		SenderCharacterID:    senderID,
		RecipientCharacterID: recipientID,
		GoldAmount:           50,
		Status:               ParcelStatusPending,
	}
	if err := noTxSvc.CancelParcel(ctx, senderID, noTxPID); err != nil {
		t.Fatalf("no tx cancel failed: %v", err)
	}
}

func TestClaimParcel_ComprehensiveErrorPaths(t *testing.T) {
	svc, dRepo, cRepo, iRepo := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	senderID := "sender-claim-01"
	recipientID := "recipient-claim-01"
	cRepo.chars[senderID] = corecharacter.Character{ID: senderID, Name: "Sender", Money: 500}
	cRepo.chars[recipientID] = corecharacter.Character{ID: recipientID, Name: "Recipient", Money: 100}

	// 1. Invalid input
	if _, err := svc.ClaimParcel(ctx, "", "parcel-1", now); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty recipientID, got %v", err)
	}
	if _, err := svc.ClaimParcel(ctx, recipientID, "", now); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty parcelID, got %v", err)
	}

	// 2. Parcel not found
	if _, err := svc.ClaimParcel(ctx, recipientID, "nonexistent", now); !errors.Is(err, ErrParcelNotFound) {
		t.Errorf("expected ErrParcelNotFound, got %v", err)
	}

	// Create parcel with item only
	pItemOnly := "p-item-only"
	dRepo.parcels[pItemOnly] = Parcel{
		ID:                   pItemOnly,
		SenderCharacterID:    senderID,
		RecipientCharacterID: recipientID,
		ItemID:               "item-001",
		ItemName:             "薬草",
		ItemQuantity:         3,
		GoldAmount:           0,
		Status:               ParcelStatusPending,
		CreatedAt:            now,
	}

	// 3. Forbidden (different recipient)
	if _, err := svc.ClaimParcel(ctx, "wrong-recipient", pItemOnly, now); !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}

	// 4. Missing recipient in charRepo
	ghostP := "ghost-rec-parcel"
	dRepo.parcels[ghostP] = Parcel{
		ID:                   ghostP,
		SenderCharacterID:    senderID,
		RecipientCharacterID: "ghost-rec",
		Status:               ParcelStatusPending,
	}
	if _, err := svc.ClaimParcel(ctx, "ghost-rec", ghostP, now); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}

	// 5. Inventory error
	iRepo.err = errors.New("inv query error")
	if _, err := svc.ClaimParcel(ctx, recipientID, pItemOnly, now); err == nil || !errors.Is(err, iRepo.err) {
		t.Errorf("expected inv query error, got %v", err)
	}
	iRepo.err = nil

	// 6. Claim item-only parcel
	claimRes, err := svc.ClaimParcel(ctx, recipientID, pItemOnly, now)
	if err != nil {
		t.Fatalf("unexpected claim error: %v", err)
	}
	if claimRes.GoldAmount != 0 || claimRes.ItemQuantity != 3 || claimRes.ItemID != "item-001" {
		t.Errorf("unexpected claim result: %+v", claimRes)
	}
	rec, _ := cRepo.FindByID(ctx, recipientID)
	if rec.Money != 100 {
		t.Errorf("expected unchanged recipient money 100, got %d", rec.Money)
	}
	recInv, _ := iRepo.FindByCharacterID(ctx, recipientID)
	if recInv.Quantity("item-001") != 3 {
		t.Errorf("expected 3 herbs in recipient inventory, got %d", recInv.Quantity("item-001"))
	}

	// 7. Claim gold-only parcel without transaction provider
	itemDefs := &mockItemDefs{defs: map[string]coreitem.Definition{"item-001": {ID: "item-001", Name: "薬草"}}}
	noTxSvc, _ := NewService(dRepo, cRepo, iRepo, WithItemDefinitionProvider(itemDefs))
	pGoldOnly := "p-gold-only"
	dRepo.parcels[pGoldOnly] = Parcel{
		ID:                   pGoldOnly,
		SenderCharacterID:    senderID,
		RecipientCharacterID: recipientID,
		GoldAmount:           250,
		Status:               ParcelStatusPending,
		CreatedAt:            now,
	}
	claimRes, err = noTxSvc.ClaimParcel(ctx, recipientID, pGoldOnly, now)
	if err != nil {
		t.Fatalf("no tx claim failed: %v", err)
	}
	if claimRes.GoldAmount != 250 {
		t.Errorf("expected 250 gold, got %d", claimRes.GoldAmount)
	}
	rec, _ = cRepo.FindByID(ctx, recipientID)
	if rec.Money != 350 {
		t.Errorf("expected 350 money (100 + 250), got %d", rec.Money)
	}
}

func TestCompleteDelivery_ComprehensiveErrorPaths(t *testing.T) {
	svc, dRepo, cRepo, iRepo := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	charID := "char-quest-test"
	cRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "Tester", Level: 5, Money: 200, Experience: 100}

	// 1. Invalid input
	if _, err := svc.CompleteDelivery(ctx, "", "del-01", now); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty characterID, got %v", err)
	}
	if _, err := svc.CompleteDelivery(ctx, charID, "", now); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty deliveryID, got %v", err)
	}

	// 2. Character not found
	if _, err := svc.CompleteDelivery(ctx, "ghost-char", "del-01", now); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}

	// 3. Delivery not found
	if _, err := svc.CompleteDelivery(ctx, charID, "nonexistent-del", now); !errors.Is(err, ErrDeliveryNotFound) {
		t.Errorf("expected ErrDeliveryNotFound, got %v", err)
	}

	// Create test quest with bonus item reward
	qID := "bonus-quest"
	dRepo.quests[qID] = Quest{
		ID:               qID,
		TargetItemID:     "item-001",
		RequiredQuantity: 2,
		RewardGold:       300,
		RewardExp:        50,
		RewardItemID:     "item-007", // 毒消し草 bonus
		ExpiresAt:        now.Add(1 * time.Hour),
	}

	delID := "active-delivery-1"
	dRepo.deliveries[delID] = CharacterDelivery{
		ID:          delID,
		CharacterID: charID,
		QuestID:     qID,
		Status:      StatusInProgress,
		AcceptedAt:  now,
	}

	// 4. Forbidden (other character tries to complete)
	cRepo.chars["other-char"] = corecharacter.Character{ID: "other-char", Name: "Other"}
	if _, err := svc.CompleteDelivery(ctx, "other-char", delID, now); !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}

	// 5. Quest expired
	expiredQID := "expired-quest"
	dRepo.quests[expiredQID] = Quest{
		ID:               expiredQID,
		TargetItemID:     "item-001",
		RequiredQuantity: 1,
		ExpiresAt:        now.Add(-10 * time.Minute),
	}
	expiredDelID := "del-expired"
	dRepo.deliveries[expiredDelID] = CharacterDelivery{
		ID:          expiredDelID,
		CharacterID: charID,
		QuestID:     expiredQID,
		Status:      StatusInProgress,
	}
	if _, err := svc.CompleteDelivery(ctx, charID, expiredDelID, now); !errors.Is(err, ErrQuestExpired) {
		t.Errorf("expected ErrQuestExpired, got %v", err)
	}

	// 6. Quest missing from repo
	missingQDelID := "del-missing-q"
	dRepo.deliveries[missingQDelID] = CharacterDelivery{
		ID:          missingQDelID,
		CharacterID: charID,
		QuestID:     "deleted-quest",
		Status:      StatusInProgress,
	}
	if _, err := svc.CompleteDelivery(ctx, charID, missingQDelID, now); !errors.Is(err, ErrQuestNotFound) {
		t.Errorf("expected ErrQuestNotFound, got %v", err)
	}

	// 7. Insufficient items (char has 0 herbs, needs 2)
	if _, err := svc.CompleteDelivery(ctx, charID, delID, now); !errors.Is(err, ErrInsufficientItems) {
		t.Errorf("expected ErrInsufficientItems, got %v", err)
	}

	// 8. Inventory query error
	inv, _ := coreinventory.New(charID)
	_ = inv.Add(coreitem.Instance{ID: "herb-inst", DefinitionID: "item-001", Quantity: 5})
	_ = iRepo.Save(ctx, inv)

	iRepo.err = errors.New("inv db error")
	if _, err := svc.CompleteDelivery(ctx, charID, delID, now); err == nil || !errors.Is(err, iRepo.err) {
		t.Errorf("expected inv db error, got %v", err)
	}
	iRepo.err = nil

	// 9. Complete delivery with bonus item reward
	res, err := svc.CompleteDelivery(ctx, charID, delID, now)
	if err != nil {
		t.Fatalf("unexpected CompleteDelivery error: %v", err)
	}
	if res.RewardedGold != 300 || res.RewardedExp != 50 || res.RewardedItemID != "item-007" {
		t.Errorf("unexpected completion result: %+v", res)
	}
	// Verify inventory consumed 2 herbs and added 1 item-007
	inv, _ = iRepo.FindByCharacterID(ctx, charID)
	if inv.Quantity("item-001") != 3 {
		t.Errorf("expected 3 herbs remaining (5 - 2), got %d", inv.Quantity("item-001"))
	}
	if inv.Quantity("item-007") != 1 {
		t.Errorf("expected 1 bonus item-007 in inventory, got %d", inv.Quantity("item-007"))
	}
	// Verify character stats
	char, _ := cRepo.FindByID(ctx, charID)
	if char.Money != 500 { // 200 + 300
		t.Errorf("expected 500 money, got %d", char.Money)
	}

	// 10. Completed delivery cannot be completed again
	if _, err := svc.CompleteDelivery(ctx, charID, delID, now); !errors.Is(err, ErrDeliveryNotActive) {
		t.Errorf("expected ErrDeliveryNotActive, got %v", err)
	}

	// 11. Complete delivery without transaction provider
	itemDefs := &mockItemDefs{
		defs: map[string]coreitem.Definition{
			"item-001": {ID: "item-001", Name: "薬草"},
		},
	}
	noTxSvc, _ := NewService(dRepo, cRepo, iRepo, WithItemDefinitionProvider(itemDefs))
	noTxDelID := "del-no-tx"
	noTxQID := "quest-no-tx"
	dRepo.quests[noTxQID] = Quest{
		ID:               noTxQID,
		TargetItemID:     "item-001",
		RequiredQuantity: 1,
		RewardGold:       100,
		ExpiresAt:        now.Add(1 * time.Hour),
	}
	dRepo.deliveries[noTxDelID] = CharacterDelivery{
		ID:          noTxDelID,
		CharacterID: charID,
		QuestID:     noTxQID,
		Status:      StatusInProgress,
	}
	res, err = noTxSvc.CompleteDelivery(ctx, charID, noTxDelID, now)
	if err != nil {
		t.Fatalf("no tx CompleteDelivery failed: %v", err)
	}
	if res.RewardedGold != 100 {
		t.Errorf("expected 100 gold, got %d", res.RewardedGold)
	}
}

func TestDelivery_HistoryAndOptions(t *testing.T) {
	svc, dRepo, cRepo, iRepo := setupDeliveryTest(t)
	ctx := context.Background()
	now := time.Now()

	charID := "hist-char-1"
	cRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "HistChar"}

	qID := "hist-quest-1"
	dRepo.quests[qID] = Quest{ID: qID, TargetItemName: "薬草"}
	delID := "hist-del-1"
	dRepo.deliveries[delID] = CharacterDelivery{
		ID:          delID,
		CharacterID: charID,
		QuestID:     qID,
		Status:      StatusInProgress,
	}

	// 1. GetCharacterDeliveries
	deliveries, err := svc.GetCharacterDeliveries(ctx, charID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d (err: %v)", len(deliveries), err)
	}
	if deliveries[0].Quest == nil || deliveries[0].Quest.TargetItemName != "薬草" {
		t.Errorf("expected hydrated quest, got %+v", deliveries[0].Quest)
	}

	// 2. GetActiveCharacterDeliveries
	activeDels, err := svc.GetActiveCharacterDeliveries(ctx, charID)
	if err != nil || len(activeDels) != 1 {
		t.Fatalf("expected 1 active delivery, got %d (err: %v)", len(activeDels), err)
	}

	// 3. GetSentParcels and GetIncomingParcels (non-cursor)
	pID := "hist-parcel-1"
	dRepo.parcels[pID] = Parcel{
		ID:                   pID,
		SenderCharacterID:    charID,
		RecipientCharacterID: "other-char",
		Status:               ParcelStatusPending,
		CreatedAt:            now,
	}

	sent, err := svc.GetSentParcels(ctx, charID)
	if err != nil || len(sent) != 1 {
		t.Fatalf("expected 1 sent parcel, got %d (err: %v)", len(sent), err)
	}

	incoming, err := svc.GetIncomingParcels(ctx, "other-char")
	if err != nil || len(incoming) != 1 {
		t.Fatalf("expected 1 incoming parcel, got %d (err: %v)", len(incoming), err)
	}

	// 4. WithRandomSource Option
	customSvc, err := NewService(dRepo, cRepo, iRepo, WithRandomSource(mockRandSource{val: 2}))
	if err != nil {
		t.Fatalf("NewService with random source failed: %v", err)
	}
	quests := customSvc.GenerateQuests(1, now)
	if len(quests) != 1 {
		t.Fatalf("expected 1 quest generated, got %d", len(quests))
	}
}

type mockRandSource struct {
	val int
}

func (m mockRandSource) Intn(max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	return m.val % max, nil
}
