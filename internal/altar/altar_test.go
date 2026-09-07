package altar

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	m.chars[c.ID] = c
	return nil
}

type mockInvRepo struct {
	invs map[string]coreinventory.Inventory
}

func (m *mockInvRepo) FindByCharacterIDForUpdate(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.invs[characterID]
	if !ok {
		newInv, _ := coreinventory.New(characterID)
		m.invs[characterID] = newInv
		return newInv, nil
	}
	return inv, nil
}

func (m *mockInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	m.invs[inv.CharacterID] = inv
	return nil
}

type mockDepotRepo struct {
	depots map[string]depot.Depot
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(_ context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depots[characterID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (m *mockDepotRepo) Save(_ context.Context, dep depot.Depot) error {
	m.depots[dep.CharacterID] = dep
	return nil
}

type mockAltarRepo struct {
	awakenings []RamiaAwakening
}

func (m *mockAltarRepo) SaveAwakening(_ context.Context, a RamiaAwakening) error {
	m.awakenings = append(m.awakenings, a)
	return nil
}

func (m *mockAltarRepo) GetLatestAwakening(_ context.Context) (*RamiaAwakening, error) {
	if len(m.awakenings) == 0 {
		return nil, nil
	}
	return &m.awakenings[len(m.awakenings)-1], nil
}

type mockTxProvider struct{}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type mockItemDefProvider struct {
	defs map[string]coreitem.Definition
}

func (m *mockItemDefProvider) FindByID(id string) (coreitem.Definition, error) {
	def, ok := m.defs[id]
	if !ok {
		return coreitem.Definition{}, coreitem.ErrDefinitionNotFound
	}
	return def, nil
}

type mockCollector struct {
	recorded []string
}

func (m *mockCollector) RecordItemDiscovered(_ context.Context, characterID, itemID, itemName, category string) error {
	m.recorded = append(m.recorded, itemID)
	return nil
}

func setupTestService(t *testing.T) (*Service, *mockCharRepo, *mockInvRepo, *mockDepotRepo, *mockAltarRepo, *mockCollector) {
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	invRepo := &mockInvRepo{invs: make(map[string]coreinventory.Inventory)}
	depotRepo := &mockDepotRepo{depots: make(map[string]depot.Depot)}
	altarRepo := &mockAltarRepo{}
	collector := &mockCollector{}
	itemDefs := &mockItemDefProvider{
		defs: map[string]coreitem.Definition{
			ItemMirrorOfTruth:  {ID: ItemMirrorOfTruth, Name: "真実の鏡"},
			ItemMadamsInvite:   {ID: ItemMadamsInvite, Name: "マダムの招待状"},
			ItemTreasureMap:    {ID: ItemTreasureMap, Name: "宝の地図"},
			ItemLampOfDarkness: {ID: ItemLampOfDarkness, Name: "闇のランプ"},
		},
	}
	txProvider := &mockTxProvider{}

	svc, err := NewService(
		charRepo,
		invRepo,
		depotRepo,
		altarRepo,
		itemDefs,
		txProvider,
		WithCollectionRecorder(collector),
		WithNowFunc(func() time.Time {
			return time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
		}),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	return svc, charRepo, invRepo, depotRepo, altarRepo, collector
}

func TestNewServiceNilDependencies(t *testing.T) {
	_, err := NewService(nil, nil, nil, nil, nil, nil)
	if !errors.Is(err, ErrNilDependency) {
		t.Fatalf("expected ErrNilDependency, got %v", err)
	}
}

func TestGetStatusProgression(t *testing.T) {
	svc, charRepo, _, _, _, _ := setupTestService(t)
	ctx := context.Background()

	c, _ := corecharacter.New("Hero")
	c.ID = "char-1"
	charRepo.chars[c.ID] = c

	// 1. Initial: 0 orbs
	status, err := svc.GetStatus(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.OrbCount != 0 || status.HasAllOrbs || status.RamiaAwakened || status.RamiaPresent {
		t.Fatalf("unexpected initial status: %+v", status)
	}
	if status.MikoDialogue != MsgMikoGuardingEggs {
		t.Fatalf("expected guarding eggs dialogue, got %q", status.MikoDialogue)
	}

	// 2. All 6 orbs offered
	c.Orb = "srbgyp"
	charRepo.chars[c.ID] = c
	status, err = svc.GetStatus(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.OrbCount != 6 || !status.HasAllOrbs || status.RamiaAwakened {
		t.Fatalf("unexpected all orbs status: %+v", status)
	}
	if status.MikoDialogue != MsgMikoReadyToPray {
		t.Fatalf("expected ready to pray dialogue, got %q", status.MikoDialogue)
	}

	// 3. Ramia awakened
	c.Orb = "G"
	charRepo.chars[c.ID] = c
	status, err = svc.GetStatus(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.RamiaAwakened {
		t.Fatalf("expected RamiaAwakened to be true")
	}
	if status.MikoDialogue != MsgMikoAfterAwakening {
		t.Fatalf("expected after awakening dialogue, got %q", status.MikoDialogue)
	}
	if len(status.AvailableItems) != 4 {
		t.Fatalf("expected 4 available items, got %d", len(status.AvailableItems))
	}
}

func TestPrayInsufficientOrbs(t *testing.T) {
	svc, charRepo, _, _, _, _ := setupTestService(t)
	ctx := context.Background()

	c, _ := corecharacter.New("Hero")
	c.ID = "char-1"
	c.Orb = "srb" // only 3 orbs
	charRepo.chars[c.ID] = c

	_, err := svc.Pray(ctx, "char-1")
	if !errors.Is(err, ErrInsufficientOrbs) {
		t.Fatalf("expected ErrInsufficientOrbs, got %v", err)
	}
}

func TestPraySuccessAwakensRamia(t *testing.T) {
	svc, charRepo, _, _, altarRepo, _ := setupTestService(t)
	ctx := context.Background()

	c, _ := corecharacter.New("Hero")
	c.ID = "char-1"
	c.Orb = "srbgyp" // all 6 orbs
	charRepo.chars[c.ID] = c

	res, err := svc.Pray(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected pray error: %v", err)
	}
	if !res.RamiaAwakened || res.Message != MsgRamiaAwakening {
		t.Fatalf("unexpected pray result: %+v", res)
	}

	// Character orb should now be "G"
	updatedChar := charRepo.chars["char-1"]
	if updatedChar.Orb != "G" {
		t.Fatalf("expected character orb 'G', got %q", updatedChar.Orb)
	}

	// AltarRepo should have awakening
	if len(altarRepo.awakenings) != 1 {
		t.Fatalf("expected 1 awakening, got %d", len(altarRepo.awakenings))
	}
	awakening := altarRepo.awakenings[0]
	if awakening.CharacterID != "char-1" || awakening.CharacterName != "Hero" {
		t.Fatalf("unexpected awakening record: %+v", awakening)
	}

	// GetStatus should now show RamiaPresent
	status, err := svc.GetStatus(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.RamiaPresent || status.AwakenedBy != "Hero" {
		t.Fatalf("expected RamiaPresent by Hero, got %+v", status)
	}
}

func TestWishNotAwakened(t *testing.T) {
	svc, charRepo, _, _, _, _ := setupTestService(t)
	ctx := context.Background()

	c, _ := corecharacter.New("Hero")
	c.ID = "char-1"
	c.Orb = "srbgyp" // 6 orbs but not prayed yet
	charRepo.chars[c.ID] = c

	_, err := svc.Wish(ctx, "char-1", ItemMirrorOfTruth)
	if !errors.Is(err, ErrRamiaNotAwakened) {
		t.Fatalf("expected ErrRamiaNotAwakened, got %v", err)
	}
}

func TestWishInvalidItem(t *testing.T) {
	svc, charRepo, _, _, _, _ := setupTestService(t)
	ctx := context.Background()

	c, _ := corecharacter.New("Hero")
	c.ID = "char-1"
	c.Orb = "G"
	charRepo.chars[c.ID] = c

	_, err := svc.Wish(ctx, "char-1", "invalid-item")
	if !errors.Is(err, ErrInvalidWishItem) {
		t.Fatalf("expected ErrInvalidWishItem, got %v", err)
	}
}

func TestWishDeliveredToInventory(t *testing.T) {
	svc, charRepo, invRepo, _, _, collector := setupTestService(t)
	ctx := context.Background()

	c, _ := corecharacter.New("Hero")
	c.ID = "char-1"
	c.Orb = "G"
	charRepo.chars[c.ID] = c

	// Inventory is empty (len == 0 < maxInvCap 1)
	res, err := svc.Wish(ctx, "char-1", ItemMirrorOfTruth)
	if err != nil {
		t.Fatalf("unexpected wish error: %v", err)
	}
	if res.DeliveredTo != "inventory" || res.ItemID != ItemMirrorOfTruth {
		t.Fatalf("unexpected wish result: %+v", res)
	}

	// Verify inventory received item
	inv := invRepo.invs["char-1"]
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != ItemMirrorOfTruth {
		t.Fatalf("expected inventory to have %s, got %+v", ItemMirrorOfTruth, inv.Items)
	}

	// Verify collector recorded
	if len(collector.recorded) != 1 || collector.recorded[0] != ItemMirrorOfTruth {
		t.Fatalf("expected collector to record %s, got %+v", ItemMirrorOfTruth, collector.recorded)
	}

	// Verify orbs cleared
	updatedChar := charRepo.chars["char-1"]
	if updatedChar.Orb != "" {
		t.Fatalf("expected cleared orb, got %q", updatedChar.Orb)
	}
}

func TestWishDeliveredToDepotWhenInventoryFull(t *testing.T) {
	svc, charRepo, invRepo, depotRepo, _, _ := setupTestService(t)
	ctx := context.Background()

	c, _ := corecharacter.New("Hero")
	c.ID = "char-1"
	c.Orb = "G"
	charRepo.chars[c.ID] = c

	// Pre-fill inventory so len == 1 (== maxInvCap)
	inv, _ := coreinventory.New("char-1")
	dummy, _ := coreitem.NewInstance("item-001", 1)
	_ = inv.Add(dummy)
	invRepo.invs["char-1"] = inv

	res, err := svc.Wish(ctx, "char-1", ItemTreasureMap)
	if err != nil {
		t.Fatalf("unexpected wish error: %v", err)
	}
	if res.DeliveredTo != "depot" || res.ItemID != ItemTreasureMap {
		t.Fatalf("unexpected wish result: %+v", res)
	}

	// Inventory still has only dummy item
	if len(invRepo.invs["char-1"].Items) != 1 {
		t.Fatalf("expected inventory to remain 1 item")
	}

	// Depot should have received ItemTreasureMap
	dep := depotRepo.depots["char-1"]
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != ItemTreasureMap {
		t.Fatalf("expected depot to have %s, got %+v", ItemTreasureMap, dep.Items)
	}

	// Verify orbs cleared
	updatedChar := charRepo.chars["char-1"]
	if updatedChar.Orb != "" {
		t.Fatalf("expected cleared orb, got %q", updatedChar.Orb)
	}
}

func TestOfferOrb(t *testing.T) {
	svc, charRepo, _, _, _, _ := setupTestService(t)
	ctx := context.Background()

	c, _ := corecharacter.New("Hero")
	c.ID = "char-1"
	charRepo.chars[c.ID] = c

	// Invalid rune
	_, err := svc.OfferOrb(ctx, "char-1", 'z')
	if !errors.Is(err, ErrInvalidOrbRune) {
		t.Fatalf("expected ErrInvalidOrbRune, got %v", err)
	}

	// Offer silver orb
	res, err := svc.OfferOrb(ctx, "char-1", corecharacter.OrbSilver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TotalOrbs != 1 || res.OrbName != "シルバーオーブ" {
		t.Fatalf("unexpected offer result: %+v", res)
	}

	// Duplicate offer
	_, err = svc.OfferOrb(ctx, "char-1", corecharacter.OrbSilver)
	if !errors.Is(err, ErrOrbAlreadyOffered) {
		t.Fatalf("expected ErrOrbAlreadyOffered, got %v", err)
	}
}
