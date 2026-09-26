package helperquest

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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

func (r *stubCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *stubCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	r.characters[c.ID] = c
	return nil
}

type stubDepotRepo struct {
	depots map[string]depot.Depot
}

func newStubDepotRepo() *stubDepotRepo {
	return &stubDepotRepo{depots: make(map[string]depot.Depot)}
}

func (r *stubDepotRepo) FindByCharacterIDForUpdate(_ context.Context, characterID string) (depot.Depot, error) {
	d, ok := r.depots[characterID]
	if !ok {
		return depot.NewDepotWithCapacity(characterID, 5, 0, 0)
	}
	return d, nil
}

func (r *stubDepotRepo) Save(_ context.Context, d depot.Depot) error {
	r.depots[d.CharacterID] = d
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

type stubTransactionProvider struct{}

func (p *stubTransactionProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
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

	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	inst1, _ := item.NewInstance("weapon-01", 1)
	inst2, _ := item.NewInstance("weapon-01", 1)
	_ = dep.AddItem(inst1)
	_ = dep.AddItem(inst2)

	depotRepo := newStubDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	guildRepo := &stubGuildRepo{
		guildPoints: make(map[string]int),
		charGuild:   map[string]string{"char-1": "guild-1"},
	}

	svc := NewService(questRepo, charRepo, guildRepo, &stubTransactionProvider{}, WithDepotRepository(depotRepo))

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
	if len(result.Depot.Items) != 1 {
		t.Errorf("expected 1 reward item in depot, got %d items", len(result.Depot.Items))
	}
	if result.Depot.Items[0].DefinitionID != "item-128" {
		t.Errorf("expected reward item-128, got %s", result.Depot.Items[0].DefinitionID)
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
	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	inst, _ := item.NewInstance("item-001", 4)
	_ = dep.AddItem(inst)

	depotRepo := newStubDepotRepo()
	_ = depotRepo.Save(ctx, dep)

	guildRepo := &stubGuildRepo{
		guildPoints: make(map[string]int),
		charGuild:   map[string]string{"char-1": "guild-1"},
	}

	svc := NewService(questRepo, charRepo, guildRepo, &stubTransactionProvider{}, WithDepotRepository(depotRepo))

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
	guildRepo := &stubGuildRepo{charGuild: make(map[string]string)}

	svc := NewService(questRepo, charRepo, guildRepo, &stubTransactionProvider{}, WithDepotRepository(newStubDepotRepo()))

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

	svc := NewService(questRepo, &stubCharRepo{}, &stubGuildRepo{}, &stubTransactionProvider{})
	itemIDs, err := svc.GetActiveHelperItemIDs(ctx, now)
	if err != nil {
		t.Fatalf("GetActiveHelperItemIDs failed: %v", err)
	}

	if len(itemIDs) != 2 {
		t.Fatalf("expected 2 active item IDs, got %d", len(itemIDs))
	}
}

type failingTxProvider struct{}

func (p *failingTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	_ = fn(ctx)
	return errors.New("simulated tx commit failure")
}

func TestCompleteQuestTransactionFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	questRepo := newStubQuestRepo()
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "HelperHero"},
		},
	}
	dep, _ := depot.NewDepotWithCapacity("char-1", 5, 0, 0)
	depInst, _ := item.NewInstance("weapon-01", 3)
	_ = dep.AddItem(depInst)

	depotRepo := newStubDepotRepo()
	_ = depotRepo.Save(ctx, dep)
	guildRepo := &stubGuildRepo{charGuild: make(map[string]string)}

	quest := Quest{
		ID:            "quest-fail-tx",
		Title:         "Sample Quest",
		Kind:          KindWeapon,
		TargetID:      "weapon-01",
		RequiredCount: 2,
		RewardItemID:  "item-128",
		ExpiresAt:     now.Add(6 * 24 * time.Hour),
		CreatedAt:     now,
	}
	_ = questRepo.Save(ctx, quest)

	svc := NewService(questRepo, charRepo, guildRepo, &failingTxProvider{}, WithDepotRepository(depotRepo))

	_, err := svc.CompleteQuest(ctx, "char-1", "quest-fail-tx", now)
	if err == nil {
		t.Fatal("expected error from failed transaction, got nil")
	}
}

func TestService_SetRandomSource(t *testing.T) {
	questRepo := newStubQuestRepo()
	svc := NewService(questRepo, &stubCharRepo{}, &stubGuildRepo{}, &stubTransactionProvider{})
	mockRand := &mockRandomSource{values: []int{0, 1, 0, 0}}
	svc.SetRandomSource(mockRand)
	if svc.randomSource != mockRand {
		t.Fatalf("expected randomSource to be updated")
	}
}
