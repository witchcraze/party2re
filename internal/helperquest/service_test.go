package helperquest_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/helperquest"
)

type mockQuestRepo struct {
	quests              map[string]helperquest.Quest
	failReplacementSave bool
}

func newMockQuestRepo() *mockQuestRepo {
	return &mockQuestRepo{quests: make(map[string]helperquest.Quest)}
}

func (r *mockQuestRepo) Save(_ context.Context, q helperquest.Quest) error {
	if r.failReplacementSave && q.CompletedAt == nil {
		return errors.New("failed to save replacement quest")
	}
	r.quests[q.ID] = q
	return nil
}

func (r *mockQuestRepo) FindByID(_ context.Context, id string) (helperquest.Quest, error) {
	q, ok := r.quests[id]
	if !ok {
		return helperquest.Quest{}, helperquest.ErrQuestNotFound
	}
	return q, nil
}

func (r *mockQuestRepo) ListActive(_ context.Context, now time.Time) ([]helperquest.Quest, error) {
	var list []helperquest.Quest
	for _, q := range r.quests {
		if q.CompletedAt == nil && q.ExpiresAt.After(now) {
			list = append(list, q)
		}
	}
	return list, nil
}

type mockCharRepo struct {
	characters map[string]corecharacter.Character
}

func (r *mockCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *mockCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	r.characters[c.ID] = c
	return nil
}

type mockInvRepo struct {
	inventories map[string]coreinventory.Inventory
}

func (r *mockInvRepo) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := r.inventories[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (r *mockInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, characterID)
}

func (r *mockInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.inventories[inv.CharacterID] = inv
	return nil
}

type mockDepotRepo struct {
	depots map[string]depot.Depot
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{depots: make(map[string]depot.Depot)}
}

func (r *mockDepotRepo) FindByCharacterIDForUpdate(_ context.Context, characterID string) (depot.Depot, error) {
	d, ok := r.depots[characterID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (r *mockDepotRepo) Save(_ context.Context, d depot.Depot) error {
	r.depots[d.CharacterID] = d
	return nil
}

type mockTxProvider struct {
	rollbackCalled bool
}

func (p *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	err := fn(ctx)
	if err != nil {
		p.rollbackCalled = true
		return err
	}
	return nil
}

func fillInventory(t *testing.T, charID string, count int) coreinventory.Inventory {
	t.Helper()
	inv, err := coreinventory.New(charID)
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}
	for i := 0; i < count; i++ {
		inst, err := item.NewInstance(fmt.Sprintf("item-filler-%03d", i), 1)
		if err != nil {
			t.Fatalf("failed to create item: %v", err)
		}
		if err := inv.Add(inst); err != nil {
			t.Fatalf("failed to add item %d: %v", i, err)
		}
	}
	return inv
}

func TestCompleteQuest_RewardToInventory(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0, JobLevel: 5},
		},
	}
	inv, _ := coreinventory.New("char-1")
	inst, _ := item.NewInstance("weapon-01", 2)
	_ = inv.Add(inst)

	invRepo := &mockInvRepo{inventories: map[string]coreinventory.Inventory{"char-1": inv}}
	depotRepo := newMockDepotRepo()

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		invRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-normal",
		Title:         "店を始めたいのでその1",
		Kind:          helperquest.KindWeapon,
		TargetID:      "weapon-01",
		TargetName:    "ヒノキの棒",
		RequiredCount: 2,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	res, err := svc.CompleteQuest(ctx, "char-1", "q-normal", now)
	if err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	// Reward should be in inventory
	if len(res.Inventory.Items) != 1 {
		t.Fatalf("expected 1 item in inventory, got %d", len(res.Inventory.Items))
	}
	if res.Inventory.Items[0].DefinitionID != "item-128" {
		t.Errorf("expected reward item-128 in inventory, got %s", res.Inventory.Items[0].DefinitionID)
	}

	// Depot should have no items
	dep, err := depotRepo.FindByCharacterIDForUpdate(ctx, "char-1")
	if err == nil && len(dep.Items) > 0 {
		t.Errorf("expected empty depot, got %d items", len(dep.Items))
	}
}

func TestCompleteQuest_RewardOverflowToDepotWhenInventoryFull(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0, JobLevel: 5},
		},
	}

	// Inventory with 20 items: 19 filler items + 1 stack of required items (2x weapon-01).
	// RequiredCount is 1, so consuming 1 leaves 1x weapon-01 in that slot.
	// Therefore, inventory remains at exactly 20 slots (full).
	inv := fillInventory(t, "char-1", 19)
	stackedTarget, _ := item.NewInstance("weapon-01", 2)
	if err := inv.Add(stackedTarget); err != nil {
		t.Fatalf("failed to add stacked target: %v", err)
	}
	if !inv.IsFull() {
		t.Fatalf("expected inventory to be full (20 items), got %d", len(inv.Items))
	}

	invRepo := &mockInvRepo{inventories: map[string]coreinventory.Inventory{"char-1": inv}}
	depotRepo := newMockDepotRepo()

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		invRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-overflow",
		Title:         "店を始めたいのでその2",
		Kind:          helperquest.KindWeapon,
		TargetID:      "weapon-01",
		TargetName:    "ヒノキの棒",
		RequiredCount: 1,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	res, err := svc.CompleteQuest(ctx, "char-1", "q-overflow", now)
	if err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	// Inventory should still be full at 20 items
	savedInv, err := invRepo.FindByCharacterID(ctx, "char-1")
	if err != nil {
		t.Fatalf("failed to load saved inventory: %v", err)
	}
	if len(savedInv.Items) != 20 {
		t.Errorf("expected 20 items in inventory, got %d", len(savedInv.Items))
	}
	if len(res.Inventory.Items) != 20 {
		t.Errorf("expected 20 items in result inventory, got %d", len(res.Inventory.Items))
	}

	// Depot should have received the reward item
	dep, err := depotRepo.FindByCharacterIDForUpdate(ctx, "char-1")
	if err != nil {
		t.Fatalf("failed to load depot: %v", err)
	}
	if len(dep.Items) != 1 {
		t.Fatalf("expected 1 item in depot, got %d", len(dep.Items))
	}
	if dep.Items[0].DefinitionID != "item-128" {
		t.Errorf("expected reward item-128 in depot, got %s", dep.Items[0].DefinitionID)
	}

	// Quest completed and character help count incremented
	if res.CompletedQuest.CompletedAt == nil {
		t.Errorf("expected CompletedAt to be set")
	}
	if res.Character.HelpCount != 1 {
		t.Errorf("expected HelpCount 1, got %d", res.Character.HelpCount)
	}
}

func TestCompleteQuest_DepotFullRollback(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0, JobLevel: 0},
		},
	}

	// Inventory is full: 19 fillers + 1 slot with 2x weapon-01
	inv := fillInventory(t, "char-1", 19)
	stackedTarget, _ := item.NewInstance("weapon-01", 2)
	_ = inv.Add(stackedTarget)
	invRepo := &mockInvRepo{inventories: map[string]coreinventory.Inventory{"char-1": inv}}

	// Depot is also completely full
	depotRepo := newMockDepotRepo()
	cap := depot.CalculateCapacity(0, 0, 0)
	fullDepot, _ := depot.NewDepotWithCapacity("char-1", 0, 0, 0)
	for i := 0; i < cap; i++ {
		dummy, _ := item.NewInstance(fmt.Sprintf("item-depot-%03d", i), 1)
		_ = fullDepot.AddItem(dummy)
	}
	_ = depotRepo.Save(ctx, fullDepot)

	txProvider := &mockTxProvider{}
	svc := helperquest.NewService(
		questRepo,
		charRepo,
		invRepo,
		nil,
		txProvider,
		helperquest.WithDepotRepository(depotRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-depot-full",
		Title:         "店を始めたいのでその3",
		Kind:          helperquest.KindWeapon,
		TargetID:      "weapon-01",
		TargetName:    "ヒノキの棒",
		RequiredCount: 1,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "q-depot-full", now)
	if err == nil {
		t.Fatal("expected error when both inventory and depot are full, got nil")
	}

	if !errors.Is(err, depot.ErrDepotFull) && !errors.Is(err, helperquest.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got: %v", err)
	}

	if !txProvider.rollbackCalled {
		t.Error("expected transaction rollback to be triggered")
	}
}

func TestCompleteQuest_NilDepotRepo_FullInventoryReturnsError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0},
		},
	}

	inv := fillInventory(t, "char-1", 19)
	stackedTarget, _ := item.NewInstance("weapon-01", 2)
	_ = inv.Add(stackedTarget)
	invRepo := &mockInvRepo{inventories: map[string]coreinventory.Inventory{"char-1": inv}}

	// No depot repository configured (nil)
	txProvider := &mockTxProvider{}
	svc := helperquest.NewService(
		questRepo,
		charRepo,
		invRepo,
		nil,
		txProvider,
	)

	quest := helperquest.Quest{
		ID:            "q-no-depot",
		Title:         "店を始めたいのでその4",
		Kind:          helperquest.KindWeapon,
		TargetID:      "weapon-01",
		TargetName:    "ヒノキの棒",
		RequiredCount: 1,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "q-no-depot", now)
	if err == nil {
		t.Fatal("expected error when inventory is full and no depot configured, got nil")
	}

	if !errors.Is(err, depot.ErrDepotFull) && !errors.Is(err, helperquest.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got: %v", err)
	}

	if !txProvider.rollbackCalled {
		t.Error("expected transaction rollback to be triggered")
	}
}

func TestCompleteQuest_ReplacementQuestSaveErrorPropagates(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0},
		},
	}
	inv, _ := coreinventory.New("char-1")
	inst, _ := item.NewInstance("weapon-01", 2)
	_ = inv.Add(inst)

	invRepo := &mockInvRepo{inventories: map[string]coreinventory.Inventory{"char-1": inv}}
	txProvider := &mockTxProvider{}

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		invRepo,
		nil,
		txProvider,
	)

	quest := helperquest.Quest{
		ID:            "q-rot-fail",
		Title:         "店を始めたいのでその5",
		Kind:          helperquest.KindWeapon,
		TargetID:      "weapon-01",
		TargetName:    "ヒノキの棒",
		RequiredCount: 2,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	// Configure replacement quest save failure
	questRepo.failReplacementSave = true

	_, err := svc.CompleteQuest(ctx, "char-1", "q-rot-fail", now)
	if err == nil {
		t.Fatal("expected error when saving replacement quest fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to save replacement quest") {
		t.Fatalf("expected replacement quest save error, got: %v", err)
	}

	if !txProvider.rollbackCalled {
		t.Error("expected transaction rollback to be triggered")
	}
}
