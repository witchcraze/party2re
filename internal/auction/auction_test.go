package auction_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/auction"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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
		return corecharacter.Character{}, auction.ErrTargetNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) FindByName(ctx context.Context, name string) (corecharacter.Character, error) {
	for _, c := range m.chars {
		if c.Name == name {
			return c, nil
		}
	}
	return corecharacter.Character{}, auction.ErrTargetNotFound
}

func (m *mockCharRepo) FindByNameForUpdate(ctx context.Context, name string) (corecharacter.Character, error) {
	return m.FindByName(ctx, name)
}

func (m *mockCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	m.chars[char.ID] = char
	return nil
}

type mockEquipRepo struct {
	equips map[string]coreequipment.Equipment
}

func newMockEquipRepo() *mockEquipRepo {
	return &mockEquipRepo{equips: make(map[string]coreequipment.Equipment)}
}

func (m *mockEquipRepo) FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error) {
	e, ok := m.equips[characterID]
	if !ok {
		eq, _ := coreequipment.New(characterID)
		return eq, nil
	}
	return e, nil
}

func (m *mockEquipRepo) Save(ctx context.Context, value coreequipment.Equipment) error {
	m.equips[value.CharacterID] = value
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
		i, _ := coreinventory.New(characterID)
		return i, nil
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

type mockDepotRepo struct {
	depots map[string]depot.Depot
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{depots: make(map[string]depot.Depot)}
}

func (m *mockDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depots[characterID]
	if !ok {
		d, _ = depot.NewDepot(characterID)
		d.Capacity = 10
		return d, nil
	}
	return d, nil
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockDepotRepo) Save(ctx context.Context, d depot.Depot) error {
	m.depots[d.CharacterID] = d
	return nil
}

type mockCatalog struct {
	defs map[string]coreitem.Definition
}

func (m *mockCatalog) FindByID(id string) (coreitem.Definition, error) {
	if def, ok := m.defs[id]; ok {
		return def, nil
	}
	return coreitem.Definition{}, errors.New("not found")
}

func setupAuctionService(t *testing.T, opts ...auction.Option) (*auction.Service, *mockCharRepo, *mockEquipRepo, *mockInvRepo, *mockDepotRepo) {
	t.Helper()
	charRepo := newMockCharRepo()
	equipRepo := newMockEquipRepo()
	invRepo := newMockInvRepo()
	depotRepo := newMockDepotRepo()
	catalog := &mockCatalog{
		defs: map[string]coreitem.Definition{
			"sword-1": {ID: "sword-1", Name: "鋼の剣", Slot: coreitem.SlotMainHand},
			"armor-1": {ID: "armor-1", Name: "鉄の鎧", Slot: coreitem.SlotBody},
			"taboo-1": {ID: "taboo-1", Name: "禁断の書", Slot: coreitem.SlotAccessory},
		},
	}
	svc, err := auction.NewService(&mockTxProvider{}, charRepo, equipRepo, invRepo, depotRepo, catalog, opts...)
	if err != nil {
		t.Fatalf("failed to create auction service: %v", err)
	}
	return svc, charRepo, equipRepo, invRepo, depotRepo
}

func TestSendGold_Success(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _, _, _ := setupAuctionService(t)

	charRepo.chars["char-1"] = corecharacter.Character{ID: "char-1", Name: "Alice", Money: 1000}
	charRepo.chars["char-2"] = corecharacter.Character{ID: "char-2", Name: "Bob", Money: 500}

	res, err := svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-2",
		Gold:              300,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TransferredGold != 300 {
		t.Errorf("expected 300 transferred gold, got %d", res.TransferredGold)
	}
	if charRepo.chars["char-1"].Money != 700 {
		t.Errorf("expected Alice money 700, got %d", charRepo.chars["char-1"].Money)
	}
	if charRepo.chars["char-2"].Money != 800 {
		t.Errorf("expected Bob money 800, got %d", charRepo.chars["char-2"].Money)
	}
}

func TestSendGold_TargetByName(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _, _, _ := setupAuctionService(t)

	charRepo.chars["char-1"] = corecharacter.Character{ID: "char-1", Name: "Alice", Money: 1000}
	charRepo.chars["char-2"] = corecharacter.Character{ID: "char-2", Name: "Bob", Money: 500}

	res, err := svc.Send(ctx, auction.SendRequest{
		SenderCharacterID:   "char-1",
		TargetCharacterName: "Bob",
		Gold:                200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TargetCharacterID != "char-2" {
		t.Errorf("expected target ID char-2, got %s", res.TargetCharacterID)
	}
	if charRepo.chars["char-1"].Money != 800 {
		t.Errorf("expected Alice money 800, got %d", charRepo.chars["char-1"].Money)
	}
}

func TestSendGold_Errors(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _, _, _ := setupAuctionService(t)

	charRepo.chars["char-1"] = corecharacter.Character{ID: "char-1", Name: "Alice", Money: 100}
	charRepo.chars["char-2"] = corecharacter.Character{ID: "char-2", Name: "Bob", Money: 50}

	// Insufficient money
	_, err := svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-2",
		Gold:              500,
	})
	if !errors.Is(err, auction.ErrInsufficientMoney) {
		t.Errorf("expected ErrInsufficientMoney, got %v", err)
	}

	// Invalid amount (0)
	_, err = svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-2",
		Gold:              0,
	})
	if !errors.Is(err, auction.ErrNothingToSend) {
		t.Errorf("expected ErrNothingToSend, got %v", err)
	}

	// Negative amount
	_, err = svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-2",
		Gold:              -10,
	})
	if !errors.Is(err, auction.ErrInvalidSendAmount) {
		t.Errorf("expected ErrInvalidSendAmount, got %v", err)
	}

	// Send to self
	_, err = svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-1",
		Gold:              50,
	})
	if !errors.Is(err, auction.ErrCannotSendToSelf) {
		t.Errorf("expected ErrCannotSendToSelf, got %v", err)
	}

	// Target not found
	_, err = svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "nonexistent",
		Gold:              50,
	})
	if !errors.Is(err, auction.ErrTargetNotFound) {
		t.Errorf("expected ErrTargetNotFound, got %v", err)
	}
}

func TestSendItem_EquippedSlot(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, equipRepo, invRepo, depotRepo := setupAuctionService(t)

	charRepo.chars["char-1"] = corecharacter.Character{ID: "char-1", Name: "Alice", Money: 500}
	charRepo.chars["char-2"] = corecharacter.Character{ID: "char-2", Name: "Bob", Money: 500}

	// Setup Alice's inventory and equipment
	aliceInv, _ := coreinventory.New("char-1")
	swordInst := coreitem.Instance{ID: "inst-sword-1", DefinitionID: "sword-1", Quantity: 1, EnhancementLevel: 3}
	_ = aliceInv.Add(swordInst)
	_ = invRepo.Save(ctx, aliceInv)

	aliceEquip, _ := coreequipment.New("char-1")
	aliceEquip.Slots[coreitem.SlotMainHand] = "inst-sword-1"
	_ = equipRepo.Save(ctx, aliceEquip)

	// Send equipped weapon to Bob
	res, err := svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-2",
		Slot:              "weapon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TransferredItem == nil || res.TransferredItem.DefinitionID != "sword-1" {
		t.Fatalf("expected sword-1 transferred, got %+v", res.TransferredItem)
	}
	if res.TransferredItem.EnhancementLevel != 3 {
		t.Errorf("expected enhancement 3, got %d", res.TransferredItem.EnhancementLevel)
	}

	// Verify Alice's equipment slot is unequipped
	eq, _ := equipRepo.FindByCharacterID(ctx, "char-1")
	if _, equipped := eq.Equipped(coreitem.SlotMainHand); equipped {
		t.Error("expected main-hand to be empty")
	}

	// Verify Alice's inventory consumed the item
	inv, _ := invRepo.FindByCharacterID(ctx, "char-1")
	if _, ok := inv.Find("inst-sword-1"); ok {
		t.Error("expected sword to be consumed from Alice inventory")
	}

	// Verify Bob's depot received the item
	dep, _ := depotRepo.FindByCharacterID(ctx, "char-2")
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "sword-1" {
		t.Fatalf("expected sword in Bob depot, got %+v", dep.Items)
	}
	if dep.Items[0].EnhancementLevel != 3 {
		t.Errorf("expected enhancement 3 in Bob depot, got %d", dep.Items[0].EnhancementLevel)
	}
}

func TestSendItem_DepotFull(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, equipRepo, invRepo, depotRepo := setupAuctionService(t)

	charRepo.chars["char-1"] = corecharacter.Character{ID: "char-1", Name: "Alice"}
	charRepo.chars["char-2"] = corecharacter.Character{ID: "char-2", Name: "Bob"}

	aliceInv, _ := coreinventory.New("char-1")
	swordInst := coreitem.Instance{ID: "inst-1", DefinitionID: "sword-1", Quantity: 1}
	_ = aliceInv.Add(swordInst)
	_ = invRepo.Save(ctx, aliceInv)

	aliceEquip, _ := coreequipment.New("char-1")
	aliceEquip.Slots[coreitem.SlotMainHand] = "inst-1"
	_ = equipRepo.Save(ctx, aliceEquip)

	// Fill Bob's depot to max capacity (minimum dynamic capacity = 5)
	bobDepot, _ := depot.NewDepot("char-2")
	for i := 0; i < 5; i++ {
		_ = bobDepot.AddItem(coreitem.Instance{ID: fmt.Sprintf("existing-%d", i), DefinitionID: fmt.Sprintf("item-%d", i), Quantity: 1})
	}
	_ = depotRepo.Save(ctx, bobDepot)

	_, err := svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-2",
		Slot:              "weapon",
	})
	if !errors.Is(err, auction.ErrDepotFull) {
		t.Errorf("expected ErrDepotFull, got %v", err)
	}
}

func TestSendItem_HighJobLevel_ExceedsInitialCapacity(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, equipRepo, invRepo, depotRepo := setupAuctionService(t)

	charRepo.chars["char-1"] = corecharacter.Character{ID: "char-1", Name: "Alice"}
	charRepo.chars["char-2"] = corecharacter.Character{
		ID:       "char-2",
		Name:     "Bob",
		JobLevel: 20, // dynamic capacity = 20*5+5 = 105
	}

	aliceInv, _ := coreinventory.New("char-1")
	swordInst := coreitem.Instance{ID: "inst-1", DefinitionID: "sword-1", Quantity: 1}
	_ = aliceInv.Add(swordInst)
	_ = invRepo.Save(ctx, aliceInv)

	aliceEquip, _ := coreequipment.New("char-1")
	aliceEquip.Slots[coreitem.SlotMainHand] = "inst-1"
	_ = equipRepo.Save(ctx, aliceEquip)

	// Fill Bob's depot with 6 distinct items (exceeds initial capacity of 5)
	bobDepot, _ := depot.NewDepot("char-2")
	for i := 0; i < 6; i++ {
		bobDepot.Items = append(bobDepot.Items, coreitem.Instance{
			ID:           fmt.Sprintf("existing-%d", i),
			DefinitionID: fmt.Sprintf("item-%d", i),
			Quantity:     1,
		})
	}
	_ = depotRepo.Save(ctx, bobDepot)

	res, err := svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-2",
		Slot:              "weapon",
	})
	if err != nil {
		t.Fatalf("Send item failed for high JobLevel recipient with >5 depot items: %v", err)
	}
	if res.TransferredItem == nil || res.TransferredItem.DefinitionID != "sword-1" {
		t.Fatalf("expected sword-1 transferred, got %+v", res.TransferredItem)
	}

	updatedBobDepot, _ := depotRepo.FindByCharacterID(ctx, "char-2")
	if len(updatedBobDepot.Items) != 7 {
		t.Errorf("expected 7 items in Bob depot, got %d", len(updatedBobDepot.Items))
	}
}

func TestSendItem_TabooRejection(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, equipRepo, invRepo, _ := setupAuctionService(t, auction.WithTabooItems("taboo-1"))

	charRepo.chars["char-1"] = corecharacter.Character{ID: "char-1", Name: "Alice"}
	charRepo.chars["char-2"] = corecharacter.Character{ID: "char-2", Name: "Bob"}

	aliceInv, _ := coreinventory.New("char-1")
	tabooInst := coreitem.Instance{ID: "inst-taboo", DefinitionID: "taboo-1", Quantity: 1}
	_ = aliceInv.Add(tabooInst)
	_ = invRepo.Save(ctx, aliceInv)

	aliceEquip, _ := coreequipment.New("char-1")
	aliceEquip.Slots[coreitem.SlotAccessory] = "inst-taboo"
	_ = equipRepo.Save(ctx, aliceEquip)

	_, err := svc.Send(ctx, auction.SendRequest{
		SenderCharacterID: "char-1",
		TargetCharacterID: "char-2",
		Slot:              "item",
	})
	if !errors.Is(err, auction.ErrTabooItem) {
		t.Errorf("expected ErrTabooItem, got %v", err)
	}
}

func TestInspect(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, equipRepo, invRepo, _ := setupAuctionService(t)

	charRepo.chars["char-2"] = corecharacter.Character{
		ID:       "char-2",
		Name:     "Bob",
		Level:    25,
		JobID:    "warrior",
		JobLevel: 3,
		Money:    12345,
	}

	bobInv, _ := coreinventory.New("char-2")
	_ = bobInv.Add(coreitem.Instance{ID: "sword-bob", DefinitionID: "sword-1", Quantity: 1, EnhancementLevel: 5})
	_ = bobInv.Add(coreitem.Instance{ID: "armor-bob", DefinitionID: "armor-1", Quantity: 1, EnhancementLevel: 2})
	_ = invRepo.Save(ctx, bobInv)

	bobEquip, _ := coreequipment.New("char-2")
	bobEquip.Slots[coreitem.SlotMainHand] = "sword-bob"
	bobEquip.Slots[coreitem.SlotBody] = "armor-bob"
	_ = equipRepo.Save(ctx, bobEquip)

	// Inspect by ID
	res, err := svc.Inspect(ctx, "char-1", "char-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Name != "Bob" || res.Money != 12345 || res.Level != 25 {
		t.Errorf("unexpected char info: %+v", res)
	}
	if res.Weapon == nil || res.Weapon.Name != "鋼の剣" || res.Weapon.EnhancementLevel != 5 {
		t.Errorf("unexpected weapon: %+v", res.Weapon)
	}
	if res.Armor == nil || res.Armor.Name != "鉄の鎧" || res.Armor.EnhancementLevel != 2 {
		t.Errorf("unexpected armor: %+v", res.Armor)
	}

	// Inspect by Name
	resByName, err := svc.InspectByName(ctx, "char-1", "Bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resByName.CharacterID != "char-2" {
		t.Errorf("expected char-2, got %s", resByName.CharacterID)
	}
}

func TestGetVenueInfo(t *testing.T) {
	svc, _, _, _, _ := setupAuctionService(t)
	info := svc.GetVenueInfo()

	if info.Title != "オークション会場" {
		t.Errorf("expected オークション会場, got %s", info.Title)
	}
	if info.NPCName != "@ワイルド" {
		t.Errorf("expected @ワイルド, got %s", info.NPCName)
	}
	if len(info.Dialogue) != 3 {
		t.Errorf("expected 3 dialogue entries, got %d", len(info.Dialogue))
	}
}
