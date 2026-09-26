package helperquest_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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

type mockFarmMonster struct {
	id          string
	characterID string
	monsterID   string
	isAtHome    bool
}

type mockFarmRepo struct {
	monsters map[string]mockFarmMonster
}

func newMockFarmRepo() *mockFarmRepo {
	return &mockFarmRepo{monsters: make(map[string]mockFarmMonster)}
}

func (r *mockFarmRepo) ListRanchMonsters(_ context.Context, characterID string) ([]helperquest.FarmMonster, error) {
	var list []helperquest.FarmMonster
	for _, m := range r.monsters {
		if m.characterID == characterID && !m.isAtHome {
			list = append(list, helperquest.FarmMonster{
				ID:        m.id,
				MonsterID: m.monsterID,
			})
		}
	}
	return list, nil
}

func (r *mockFarmRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.monsters[id]; !ok {
		return errors.New("monster not found")
	}
	delete(r.monsters, id)
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

func TestCompleteQuest_WeaponTurnInAndRewardFromDepot(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0, JobLevel: 5},
		},
	}

	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	w1, _ := item.NewInstance("weapon-01", 1)
	w2, _ := item.NewInstance("weapon-01", 1)
	_ = dep.AddItem(w1)
	_ = dep.AddItem(w2)

	depotRepo := newMockDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-weapon",
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

	res, err := svc.CompleteQuest(ctx, "char-1", "q-weapon", now)
	if err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	// Weapons consumed, reward item in depot
	savedDep, err := depotRepo.FindByCharacterIDForUpdate(ctx, "char-1")
	if err != nil {
		t.Fatalf("failed to load depot: %v", err)
	}
	if len(savedDep.Items) != 1 {
		t.Fatalf("expected 1 item in depot, got %d", len(savedDep.Items))
	}
	if savedDep.Items[0].DefinitionID != "item-128" {
		t.Errorf("expected reward item-128 in depot, got %s", savedDep.Items[0].DefinitionID)
	}
	if res.Character.HelpCount != 1 {
		t.Errorf("expected HelpCount 1, got %d", res.Character.HelpCount)
	}
	if res.CompletedQuest.CompletedAt == nil {
		t.Errorf("expected CompletedAt to be set")
	}
}

func TestCompleteQuest_ItemTurnInAndRewardFromDepot(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0, JobLevel: 5},
		},
	}

	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	itInst, _ := item.NewInstance("item-001", 3)
	_ = dep.AddItem(itInst)

	depotRepo := newMockDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-item",
		Title:         "非常用にその1",
		Kind:          helperquest.KindItem,
		TargetID:      "item-001",
		TargetName:    "薬草",
		RequiredCount: 2,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	res, err := svc.CompleteQuest(ctx, "char-1", "q-item", now)
	if err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	savedDep, err := depotRepo.FindByCharacterIDForUpdate(ctx, "char-1")
	if err != nil {
		t.Fatalf("failed to load depot: %v", err)
	}
	if savedDep.Quantity("item-001") != 1 {
		t.Errorf("expected 1 remaining item-001 in depot, got %d", savedDep.Quantity("item-001"))
	}
	if savedDep.Quantity("item-128") != 1 {
		t.Errorf("expected 1 reward item-128 in depot, got %d", savedDep.Quantity("item-128"))
	}
	if res.Character.HelpCount != 1 {
		t.Errorf("expected HelpCount 1, got %d", res.Character.HelpCount)
	}
}

func TestCompleteQuest_MonsterTurnInFromRanch(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0, JobLevel: 5},
		},
	}

	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	depotRepo := newMockDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	farmRepo := newMockFarmRepo()
	farmRepo.monsters["mon-1"] = mockFarmMonster{
		id:          "mon-1",
		characterID: "char-1",
		monsterID:   "monster-001",
	}
	farmRepo.monsters["mon-2"] = mockFarmMonster{
		id:          "mon-2",
		characterID: "char-1",
		monsterID:   "monster-001",
	}

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
		helperquest.WithFarmRepository(farmRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-monster",
		Title:         "かわいいのでその1",
		Kind:          helperquest.KindMonster,
		TargetID:      "monster-001",
		TargetName:    "ドットスライム",
		RequiredCount: 2,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	res, err := svc.CompleteQuest(ctx, "char-1", "q-monster", now)
	if err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	// Monsters should be removed from farm
	remainingMonsters, err := farmRepo.ListRanchMonsters(ctx, "char-1")
	if err != nil {
		t.Fatalf("failed to list farm monsters: %v", err)
	}
	if len(remainingMonsters) != 0 {
		t.Errorf("expected 0 monsters in farm, got %d", len(remainingMonsters))
	}

	// Reward should be in depot
	savedDep, err := depotRepo.FindByCharacterIDForUpdate(ctx, "char-1")
	if err != nil {
		t.Fatalf("failed to load depot: %v", err)
	}
	if savedDep.Quantity("item-128") != 1 {
		t.Errorf("expected 1 reward item-128 in depot, got %d", savedDep.Quantity("item-128"))
	}
	if res.Character.HelpCount != 1 {
		t.Errorf("expected HelpCount 1, got %d", res.Character.HelpCount)
	}
}

func TestCompleteQuest_MonsterTurnIn_IgnoresHomePets(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0},
		},
	}
	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	depotRepo := newMockDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	farmRepo := newMockFarmRepo()
	// Monster is living at home as a pet, NOT in the ranch box
	farmRepo.monsters["mon-home"] = mockFarmMonster{
		id:          "mon-home",
		characterID: "char-1",
		monsterID:   "monster-001",
		isAtHome:    true,
	}

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
		helperquest.WithFarmRepository(farmRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-mon-home",
		Title:         "かわいいのでその2",
		Kind:          helperquest.KindMonster,
		TargetID:      "monster-001",
		TargetName:    "ドットスライム",
		RequiredCount: 1,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "q-mon-home", now)
	if !errors.Is(err, helperquest.ErrInsufficientItems) {
		t.Fatalf("expected ErrInsufficientItems when monster is only at home, got: %v", err)
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

	// Depot is completely full (5 slots, distinct items)
	depotRepo := newMockDepotRepo()
	cap := depot.CalculateCapacity(0, 0, 0)
	fullDepot, _ := depot.NewDepotWithCapacity("char-1", 0, 0, 0)
	for i := 0; i < cap; i++ {
		dummy, _ := item.NewInstance(fmt.Sprintf("weapon-filler-%03d", i), 1)
		_ = fullDepot.AddItem(dummy)
	}
	_ = depotRepo.Save(ctx, fullDepot)

	farmRepo := newMockFarmRepo()
	farmRepo.monsters["mon-1"] = mockFarmMonster{
		id:          "mon-1",
		characterID: "char-1",
		monsterID:   "monster-001",
	}

	txProvider := &mockTxProvider{}
	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		txProvider,
		helperquest.WithDepotRepository(depotRepo),
		helperquest.WithFarmRepository(farmRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-depot-full",
		Title:         "かわいいのでその3",
		Kind:          helperquest.KindMonster,
		TargetID:      "monster-001",
		TargetName:    "ドットスライム",
		RequiredCount: 1,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "q-depot-full", now)
	if err == nil {
		t.Fatal("expected error when depot is full, got nil")
	}

	if !errors.Is(err, depot.ErrDepotFull) && !errors.Is(err, helperquest.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got: %v", err)
	}

	if !txProvider.rollbackCalled {
		t.Error("expected transaction rollback to be triggered")
	}
}

func TestCompleteQuest_InsufficientDepotItems(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0},
		},
	}
	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	w, _ := item.NewInstance("weapon-01", 1)
	_ = dep.AddItem(w)
	depotRepo := newMockDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-insufficient",
		Title:         "店を始めたいのでその2",
		Kind:          helperquest.KindWeapon,
		TargetID:      "weapon-01",
		RequiredCount: 2, // Needs 2, depot only has 1
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "q-insufficient", now)
	if !errors.Is(err, helperquest.ErrInsufficientItems) {
		t.Fatalf("expected ErrInsufficientItems, got: %v", err)
	}
}

func TestCompleteQuest_InsufficientFarmMonsters(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0},
		},
	}
	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	depotRepo := newMockDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	farmRepo := newMockFarmRepo()
	farmRepo.monsters["mon-1"] = mockFarmMonster{
		id:          "mon-1",
		characterID: "char-1",
		monsterID:   "monster-001",
	}

	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
		helperquest.WithFarmRepository(farmRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-mon-insufficient",
		Title:         "かわいいのでその4",
		Kind:          helperquest.KindMonster,
		TargetID:      "monster-001",
		RequiredCount: 2, // Needs 2, farm only has 1
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "q-mon-insufficient", now)
	if !errors.Is(err, helperquest.ErrInsufficientItems) {
		t.Fatalf("expected ErrInsufficientItems, got: %v", err)
	}
}

func TestCompleteQuest_NilDepotRepo_ReturnsError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0},
		},
	}

	// No depot repo configured
	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		&mockTxProvider{},
	)

	quest := helperquest.Quest{
		ID:            "q-no-depot",
		Title:         "店を始めたいのでその3",
		Kind:          helperquest.KindWeapon,
		TargetID:      "weapon-01",
		RequiredCount: 1,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "q-no-depot", now)
	if err == nil {
		t.Fatal("expected error when depot repo is nil, got nil")
	}
}

func TestCompleteQuest_MonsterQuest_NilFarmRepo_ReturnsError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	questRepo := newMockQuestRepo()
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Hero", HelpCount: 0},
		},
	}
	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	depotRepo := newMockDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	// No farm repo configured
	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		&mockTxProvider{},
		helperquest.WithDepotRepository(depotRepo),
	)

	quest := helperquest.Quest{
		ID:            "q-no-farm",
		Title:         "かわいいのでその5",
		Kind:          helperquest.KindMonster,
		TargetID:      "monster-001",
		RequiredCount: 1,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "q-no-farm", now)
	if err == nil {
		t.Fatal("expected error when farm repo is nil for monster quest, got nil")
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
	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	w, _ := item.NewInstance("weapon-01", 2)
	_ = dep.AddItem(w)
	depotRepo := newMockDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	txProvider := &mockTxProvider{}
	svc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		txProvider,
		helperquest.WithDepotRepository(depotRepo),
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
