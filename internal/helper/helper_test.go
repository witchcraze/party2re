package helper

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type stubQuestRepo struct {
	quests map[string]Quest
}

func newStubQuestRepo() *stubQuestRepo {
	return &stubQuestRepo{quests: make(map[string]Quest)}
}

func (r *stubQuestRepo) Save(_ context.Context, q Quest) error {
	r.quests[q.ID] = q
	return nil
}

func (r *stubQuestRepo) FindByID(_ context.Context, id string) (Quest, error) {
	q, ok := r.quests[id]
	if !ok {
		return Quest{}, ErrQuestNotFound
	}
	return q, nil
}

func (r *stubQuestRepo) ListActive(_ context.Context, now time.Time) ([]Quest, error) {
	var list []Quest
	for _, q := range r.quests {
		if q.CompletedAt == nil && q.ExpiresAt.After(now) {
			list = append(list, q)
		}
	}
	return list, nil
}

type stubCharRepo struct {
	characters map[string]corecharacter.Character
}

func (r *stubCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *stubCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	r.characters[c.ID] = c
	return nil
}

type stubInvRepo struct {
	inventories map[string]coreinventory.Inventory
}

func (r *stubInvRepo) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := r.inventories[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (r *stubInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.inventories[inv.CharacterID] = inv
	return nil
}

type stubGuildRepo struct {
	guildPoints map[string]int
	charGuild   map[string]string
}

func (r *stubGuildRepo) FindGuildIDByCharacterID(_ context.Context, characterID string) (string, error) {
	return r.charGuild[characterID], nil
}

func (r *stubGuildRepo) AddGuildPoints(_ context.Context, guildID string, points int) error {
	r.guildPoints[guildID] += points
	return nil
}

type mockRandomSource struct {
	values []int
	index  int
}

func (m *mockRandomSource) Intn(max int) (int, error) {
	if len(m.values) == 0 {
		return 0, nil
	}
	val := m.values[m.index%len(m.values)]
	m.index++
	if max <= 0 {
		return 0, nil
	}
	return val % max, nil
}

func TestGenerateQuest(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	rand := &mockRandomSource{values: []int{0, 1, 0, 0}} // weapon, normal, count, etc.

	q, err := GenerateQuest(rand, now)
	if err != nil {
		t.Fatalf("GenerateQuest failed: %v", err)
	}

	if q.Kind != KindWeapon {
		t.Errorf("expected KindWeapon, got %v", q.Kind)
	}
	if q.RequiredCount < 2 || q.RequiredCount > 8 {
		t.Errorf("unexpected required count: %d", q.RequiredCount)
	}
	if !q.ExpiresAt.Equal(now.Add(6 * 24 * time.Hour)) {
		t.Errorf("expected 6 days expiration, got %v", q.ExpiresAt)
	}
}

func TestCompleteQuestSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	questRepo := newStubQuestRepo()
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {
				ID:        "char-1",
				Name:      "Adventurer",
				HelpCount: 0,
			},
		},
	}

	inv, _ := coreinventory.New("char-1")
	inst1, _ := item.NewInstance("weapon-01", 1)
	inst2, _ := item.NewInstance("weapon-01", 1)
	_ = inv.Add(inst1)
	_ = inv.Add(inst2)

	invRepo := &stubInvRepo{
		inventories: map[string]coreinventory.Inventory{
			"char-1": inv,
		},
	}
	guildRepo := &stubGuildRepo{
		guildPoints: make(map[string]int),
		charGuild:   map[string]string{"char-1": "guild-1"},
	}

	svc := NewService(questRepo, charRepo, invRepo, guildRepo)

	quest := Quest{
		ID:            "quest-1",
		Title:         "店を始めたいのでその1",
		Kind:          KindWeapon,
		TargetID:      "weapon-01",
		TargetName:    "ヒノキの棒",
		RequiredCount: 2,
		RewardItemID:  "item-128",
		IsRare:        false,
		IsGuild:       false,
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	result, err := svc.CompleteQuest(ctx, "char-1", "quest-1", now)
	if err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	if result.CompletedQuest.CompletedAt == nil {
		t.Errorf("expected CompletedAt to be set")
	}
	if result.Character.HelpCount != 1 {
		t.Errorf("expected HelpCount 1, got %d", result.Character.HelpCount)
	}
	if len(result.Inventory.Items) != 1 {
		t.Errorf("expected 1 reward item in inventory, got %d items", len(result.Inventory.Items))
	}
	if result.Inventory.Items[0].DefinitionID != "item-128" {
		t.Errorf("expected reward item-128, got %s", result.Inventory.Items[0].DefinitionID)
	}
}

func TestCompleteGuildQuestAwardsPoints(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	questRepo := newStubQuestRepo()
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Member", HelpCount: 2},
		},
	}
	inv, _ := coreinventory.New("char-1")
	for i := 0; i < 4; i++ {
		inst, _ := item.NewInstance("item-001", 1)
		_ = inv.Add(inst)
	}
	invRepo := &stubInvRepo{
		inventories: map[string]coreinventory.Inventory{"char-1": inv},
	}
	guildRepo := &stubGuildRepo{
		guildPoints: make(map[string]int),
		charGuild:   map[string]string{"char-1": "guild-1"},
	}

	svc := NewService(questRepo, charRepo, invRepo, guildRepo)

	quest := Quest{
		ID:            "quest-g1",
		Title:         "コレクション用その2",
		Kind:          KindItem,
		TargetID:      "item-001",
		TargetName:    "薬草",
		RequiredCount: 4,
		RewardItemID:  "item-126", // 幸福袋
		IsRare:        false,
		IsGuild:       true,
		ExpiresAt:     now.Add(24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	result, err := svc.CompleteQuest(ctx, "char-1", "quest-g1", now)
	if err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	if result.Character.HelpCount != 3 {
		t.Errorf("expected HelpCount 3, got %d", result.Character.HelpCount)
	}
	if guildRepo.guildPoints["guild-1"] != 100 {
		t.Errorf("expected 100 guild points, got %d", guildRepo.guildPoints["guild-1"])
	}
}

func TestCompleteQuestRejectsExpired(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	questRepo := newStubQuestRepo()
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "Member"},
		},
	}
	invRepo := &stubInvRepo{inventories: make(map[string]coreinventory.Inventory)}
	guildRepo := &stubGuildRepo{charGuild: make(map[string]string)}

	svc := NewService(questRepo, charRepo, invRepo, guildRepo)

	quest := Quest{
		ID:            "quest-exp",
		Title:         "Expired Quest",
		Kind:          KindWeapon,
		TargetID:      "weapon-01",
		RequiredCount: 1,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(-1 * time.Hour),
		CreatedAt:     now.Add(-7 * 24 * time.Hour),
	}
	_ = questRepo.Save(ctx, quest)

	_, err := svc.CompleteQuest(ctx, "char-1", "quest-exp", now)
	if !errors.Is(err, ErrQuestExpired) {
		t.Errorf("expected ErrQuestExpired, got %v", err)
	}
}

func TestGetActiveHelperItemIDs(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	questRepo := newStubQuestRepo()
	_ = questRepo.Save(ctx, Quest{
		ID:        "q1",
		Kind:      KindWeapon,
		TargetID:  "weapon-05",
		ExpiresAt: now.Add(10 * time.Hour),
	})
	_ = questRepo.Save(ctx, Quest{
		ID:        "q2",
		Kind:      KindItem,
		TargetID:  "item-007",
		ExpiresAt: now.Add(10 * time.Hour),
	})

	svc := NewService(questRepo, &stubCharRepo{}, &stubInvRepo{}, &stubGuildRepo{})
	itemIDs, err := svc.GetActiveHelperItemIDs(ctx, now)
	if err != nil {
		t.Fatalf("GetActiveHelperItemIDs failed: %v", err)
	}

	if len(itemIDs) != 2 {
		t.Fatalf("expected 2 active item IDs, got %d", len(itemIDs))
	}
}
