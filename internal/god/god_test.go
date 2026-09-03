package god_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/god"
)

type mockCharacterRepo struct {
	mu         sync.RWMutex
	characters map[string]corecharacter.Character
}

func newMockCharacterRepo() *mockCharacterRepo {
	return &mockCharacterRepo{
		characters: make(map[string]corecharacter.Character),
	}
}

func (m *mockCharacterRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharacterRepo) Update(ctx context.Context, character corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.characters[character.ID] = character
	return nil
}

type mockDepotRepo struct {
	mu     sync.RWMutex
	depots map[string]depot.Depot
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{
		depots: make(map[string]depot.Depot),
	}
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.depots[characterID]
	if !ok {
		d, _ = depot.NewDepot(characterID)
		m.depots[characterID] = d
	}
	return d, nil
}

func (m *mockDepotRepo) Save(ctx context.Context, dep depot.Depot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.depots[dep.CharacterID] = dep
	return nil
}

func TestGod_GetWishes_Heaven(t *testing.T) {
	repo := newMockCharacterRepo()
	svc, err := god.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Low-level character (Lv10)
	charLv10 := corecharacter.Character{ID: "char_10", Name: "Novice", Level: 10}
	repo.characters[charLv10.ID] = charLv10

	wishes, err := svc.GetWishes(ctx, "char_10", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GetWishes failed: %v", err)
	}

	hasLimitBreak := false
	hasRestore := false
	for _, w := range wishes {
		if w.ID == "wish_limit_break_level" {
			hasLimitBreak = true
		}
		if w.ID == "wish_restore_level_limit" {
			hasRestore = true
		}
	}
	if hasLimitBreak {
		t.Error("Lv10 character should not see wish_limit_break_level")
	}
	if hasRestore {
		t.Error("Lv10 character should not see wish_restore_level_limit")
	}

	// 2. Max level character (Lv99, !OverLevel)
	charLv99 := corecharacter.Character{ID: "char_99", Name: "Master", Level: 99}
	repo.characters[charLv99.ID] = charLv99

	wishes99, err := svc.GetWishes(ctx, "char_99", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GetWishes failed: %v", err)
	}

	hasLimitBreak = false
	for _, w := range wishes99 {
		if w.ID == "wish_limit_break_level" {
			hasLimitBreak = true
		}
	}
	if !hasLimitBreak {
		t.Error("Lv99 character should see wish_limit_break_level")
	}

	// 3. OverLevel character
	charOver := corecharacter.Character{ID: "char_over", Name: "Transcended", Level: 120, OverLevel: true}
	repo.characters[charOver.ID] = charOver

	wishesOver, err := svc.GetWishes(ctx, "char_over", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GetWishes failed: %v", err)
	}

	hasRestore = false
	for _, w := range wishesOver {
		if w.ID == "wish_restore_level_limit" {
			hasRestore = true
		}
	}
	if !hasRestore {
		t.Error("OverLevel character should see wish_restore_level_limit")
	}
}

func TestGod_GetWishes_Underworld(t *testing.T) {
	repo := newMockCharacterRepo()
	svc, err := god.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	char := corecharacter.Character{
		ID:        "char_u",
		Name:      "Seeker",
		OverDepot: 2,
	}
	repo.characters[char.ID] = char

	wishes, err := svc.GetWishes(context.Background(), "char_u", god.RealmUnderworld)
	if err != nil {
		t.Fatalf("GetWishes underworld failed: %v", err)
	}

	if len(wishes) != 5 {
		t.Fatalf("expected 5 underworld wishes, got %d", len(wishes))
	}

	for _, w := range wishes {
		if w.ID == "wish_expand_depot" {
			if w.CurrentTier != 2 || w.MaxTier != 5 || !w.Available {
				t.Errorf("unexpected wish_expand_depot state: %+v", w)
			}
		}
	}
}

func TestGod_GrantWish_Heaven(t *testing.T) {
	repo := newMockCharacterRepo()
	svc, err := god.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Stats wish
	char := corecharacter.Character{
		ID:    "char_hero",
		Name:  "Hero",
		Level: 50,
		Stats: corecharacter.Stats{
			MaxHP:   100,
			MaxMP:   50,
			HP:      80,
			MP:      40,
			Attack:  30,
			Defense: 20,
			Agility: 15,
		},
		Money:       1000,
		SmallMedals: 5,
	}
	repo.characters[char.ID] = char

	resStats, err := svc.GrantWish(ctx, "char_hero", "wish_stats", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish stats failed: %v", err)
	}
	if resStats.Character.Stats.MaxHP != 140 || resStats.Character.Stats.Attack != 70 {
		t.Errorf("expected MaxHP 140, Attack 70, got %+v", resStats.Character.Stats)
	}

	// 2. Money wish
	resMoney, err := svc.GrantWish(ctx, "char_hero", "wish_money", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish money failed: %v", err)
	}
	if resMoney.Character.Money != 101000 {
		t.Errorf("expected money 101000, got %d", resMoney.Character.Money)
	}

	// 3. Small medals wish
	resMedals, err := svc.GrantWish(ctx, "char_hero", "wish_small_medals", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish small medals failed: %v", err)
	}
	if resMedals.Character.SmallMedals != 25 {
		t.Errorf("expected small medals 25, got %d", resMedals.Character.SmallMedals)
	}

	// 4. Full recovery
	resRec, err := svc.GrantWish(ctx, "char_hero", "wish_full_recovery", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish recovery failed: %v", err)
	}
	if resRec.Character.Stats.HP != resRec.Character.Stats.MaxHP || resRec.Character.Stats.MP != resRec.Character.Stats.MaxMP {
		t.Errorf("expected full HP/MP recovery, got %+v", resRec.Character.Stats)
	}

	// 5. Limit break level (fail if < 99)
	_, err = svc.GrantWish(ctx, "char_hero", "wish_limit_break_level", god.RealmHeaven)
	if !errors.Is(err, god.ErrWishRequirement) {
		t.Errorf("expected ErrWishRequirement for Lv50 char, got %v", err)
	}

	// Make Lv99
	char.Level = 99
	repo.characters[char.ID] = char
	resLimit, err := svc.GrantWish(ctx, "char_hero", "wish_limit_break_level", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish limit break failed for Lv99: %v", err)
	}
	if !resLimit.Character.OverLevel {
		t.Error("expected OverLevel to be true")
	}

	// 6. Stats wish fails when OverLevel
	_, err = svc.GrantWish(ctx, "char_hero", "wish_stats", god.RealmHeaven)
	if !errors.Is(err, god.ErrWishRequirement) {
		t.Errorf("expected ErrWishRequirement for stats wish when OverLevel, got %v", err)
	}

	// 7. Restore level limit
	resRestore, err := svc.GrantWish(ctx, "char_hero", "wish_restore_level_limit", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish restore level limit failed: %v", err)
	}
	if resRestore.Character.OverLevel {
		t.Error("expected OverLevel to be false")
	}

	// 8. Lover joke wish
	resLover, err := svc.GrantWish(ctx, "char_hero", "wish_lover", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish lover failed: %v", err)
	}
	if resLover.Message == "" {
		t.Error("expected joke response message")
	}

	// 9. Secret maid wish
	resMaid, err := svc.GrantWish(ctx, "char_hero", "wish_secret_maid", god.RealmHeaven)
	if err != nil {
		t.Fatalf("GrantWish maid failed: %v", err)
	}
	if resMaid.Message == "" {
		t.Error("expected maid granted message")
	}
}

func TestGod_GrantWish_Underworld(t *testing.T) {
	charRepo := newMockCharacterRepo()
	depotRepo := newMockDepotRepo()

	svc, err := god.NewService(charRepo, god.WithDepotRepository(depotRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	char := corecharacter.Character{
		ID:   "char_underworld",
		Name: "DungeonMaster",
	}
	charRepo.characters[char.ID] = char

	// 1. Expand depot 5 times
	for tier := 1; tier <= 5; tier++ {
		res, err := svc.GrantWish(ctx, "char_underworld", "wish_expand_depot", god.RealmUnderworld)
		if err != nil {
			t.Fatalf("tier %d depot expand failed: %v", tier, err)
		}
		if res.Character.OverDepot != tier {
			t.Errorf("expected OverDepot %d, got %d", tier, res.Character.OverDepot)
		}

		dep, _ := depotRepo.FindByCharacterIDForUpdate(ctx, "char_underworld")
		expectedCap := depot.DefaultDepotCapacity + (tier * 10)
		if dep.Capacity != expectedCap {
			t.Errorf("expected depot capacity %d, got %d", expectedCap, dep.Capacity)
		}
	}

	// 6th time -> fails (maxed)
	_, err = svc.GrantWish(ctx, "char_underworld", "wish_expand_depot", god.RealmUnderworld)
	if !errors.Is(err, god.ErrLimitBreakMaxed) {
		t.Errorf("expected ErrLimitBreakMaxed on 6th expand, got %v", err)
	}

	// 2. Expand flea market
	resFlea, err := svc.GrantWish(ctx, "char_underworld", "wish_expand_flea_market", god.RealmUnderworld)
	if err != nil {
		t.Fatalf("expand flea market failed: %v", err)
	}
	if resFlea.Character.OverFlea != 1 {
		t.Errorf("expected OverFlea 1, got %d", resFlea.Character.OverFlea)
	}

	// 3. Expand monster
	resMon, err := svc.GrantWish(ctx, "char_underworld", "wish_expand_monster", god.RealmUnderworld)
	if err != nil {
		t.Fatalf("expand monster failed: %v", err)
	}
	if resMon.Character.OverMonster != 1 {
		t.Errorf("expected OverMonster 1, got %d", resMon.Character.OverMonster)
	}
	if resMon.Wish.Description != "モンスター預入上限アップ (+50枠)" {
		t.Errorf("expected description 'モンスター預入上限アップ (+50枠)', got %q", resMon.Wish.Description)
	}
	if resMon.Message != "モンスター預入上限が +50 拡張されました！ (段階: 1/5)" {
		t.Errorf("expected message 'モンスター預入上限が +50 拡張されました！ (段階: 1/5)', got %q", resMon.Message)
	}

	// 4. Expand job memory
	resJob, err := svc.GrantWish(ctx, "char_underworld", "wish_expand_job_memory", god.RealmUnderworld)
	if err != nil {
		t.Fatalf("expand job memory failed: %v", err)
	}
	if resJob.Character.OverFuture != 1 {
		t.Errorf("expected OverFuture 1, got %d", resJob.Character.OverFuture)
	}

	// 5. Expand shop store
	resStore, err := svc.GrantWish(ctx, "char_underworld", "wish_expand_shop_store", god.RealmUnderworld)
	if err != nil {
		t.Fatalf("expand shop store failed: %v", err)
	}
	if resStore.Character.OverStore != 1 {
		t.Errorf("expected OverStore 1, got %d", resStore.Character.OverStore)
	}
}

func TestGod_Dialogue(t *testing.T) {
	repo := newMockCharacterRepo()
	svc, _ := god.NewService(repo)

	hLines := svc.GetDialogue(god.RealmHeaven)
	if len(hLines) == 0 {
		t.Error("expected non-empty dialogue for heaven")
	}

	uLines := svc.GetDialogue(god.RealmUnderworld)
	if len(uLines) == 0 {
		t.Error("expected non-empty dialogue for underworld")
	}
}

func TestGod_Concurrency(t *testing.T) {
	repo := newMockCharacterRepo()
	svc, _ := god.NewService(repo)

	const workerCount = 20
	errChan := make(chan error, workerCount)

	for i := 0; i < workerCount; i++ {
		charID := fmt.Sprintf("char_worker_%d", i)
		char := corecharacter.Character{ID: charID, Name: "Worker", Money: 1000}
		repo.characters[charID] = char
	}

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			charID := fmt.Sprintf("char_worker_%d", workerID)
			res, err := svc.GrantWish(context.Background(), charID, "wish_money", god.RealmHeaven)
			if err != nil {
				errChan <- err
				return
			}
			if res.Character.Money != 101000 {
				errChan <- errors.New("unexpected money after wish")
				return
			}
			errChan <- nil
		}(i)
	}

	for i := 0; i < workerCount; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("worker failed: %v", err)
		}
	}
}
