package alchemy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
)

type memoryCharRepo struct {
	mu         sync.Mutex
	characters map[string]corecharacter.Character
}

func newMemoryCharRepo() *memoryCharRepo {
	return &memoryCharRepo{characters: make(map[string]corecharacter.Character)}
}

func (r *memoryCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *memoryCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

type memoryDepotRepo struct {
	mu     sync.Mutex
	depots map[string]depot.Depot
}

func newMemoryDepotRepo() *memoryDepotRepo {
	return &memoryDepotRepo{depots: make(map[string]depot.Depot)}
}

func (r *memoryDepotRepo) FindByCharacterID(_ context.Context, characterID string) (depot.Depot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.depots[characterID]
	if !ok {
		newDepot, _ := depot.NewDepot(characterID)
		return newDepot, nil
	}
	itemsCopy := make([]coreitem.Instance, len(d.Items))
	copy(itemsCopy, d.Items)
	d.Items = itemsCopy
	return d, nil
}

func (r *memoryDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return r.FindByCharacterID(ctx, characterID)
}

func (r *memoryDepotRepo) Save(_ context.Context, d depot.Depot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	itemsCopy := make([]coreitem.Instance, len(d.Items))
	copy(itemsCopy, d.Items)
	d.Items = itemsCopy
	r.depots[d.CharacterID] = d
	return nil
}

type memoryAlchemyRepo struct {
	mu      sync.Mutex
	states  map[string]Synthesis
	recipes map[string][]DiscoveredRecipe
	compAlc map[string]bool
}

func newMemoryAlchemyRepo() *memoryAlchemyRepo {
	return &memoryAlchemyRepo{
		states:  make(map[string]Synthesis),
		recipes: make(map[string][]DiscoveredRecipe),
		compAlc: make(map[string]bool),
	}
}

func (r *memoryAlchemyRepo) GetSynthesisState(_ context.Context, characterID string) (Synthesis, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.states[characterID]
	if !ok {
		return Synthesis{CharacterID: characterID, State: StateNone}, nil
	}
	return s, nil
}

func (r *memoryAlchemyRepo) GetSynthesisStateForUpdate(ctx context.Context, characterID string) (Synthesis, error) {
	return r.GetSynthesisState(ctx, characterID)
}

func (r *memoryAlchemyRepo) SaveSynthesisState(_ context.Context, s Synthesis) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[s.CharacterID] = s
	return nil
}

func (r *memoryAlchemyRepo) CompleteOngoingSynthesis(_ context.Context, characterID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.states[characterID]
	if ok && s.State == StateOngoing {
		s.State = StateCompleted
		r.states[characterID] = s
	}
	return nil
}

func (r *memoryAlchemyRepo) GetDiscoveredRecipes(_ context.Context, characterID string) ([]DiscoveredRecipe, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.recipes[characterID]
	res := make([]DiscoveredRecipe, len(list))
	copy(res, list)
	return res, nil
}

func (r *memoryAlchemyRepo) SaveDiscoveredRecipe(_ context.Context, characterID, recipeID string, isCrafted bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.recipes[characterID]
	for i, d := range list {
		if d.RecipeID == recipeID {
			list[i].IsCrafted = isCrafted
			r.recipes[characterID] = list
			return nil
		}
	}
	r.recipes[characterID] = append(list, DiscoveredRecipe{
		RecipeID:     recipeID,
		IsCrafted:    isCrafted,
		DiscoveredAt: time.Now().UTC(),
	})
	return nil
}

func (r *memoryAlchemyRepo) MarkRecipeCrafted(_ context.Context, characterID, recipeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.recipes[characterID]
	now := time.Now().UTC()
	for i, d := range list {
		if d.RecipeID == recipeID {
			list[i].IsCrafted = true
			list[i].CraftedAt = &now
			r.recipes[characterID] = list
			return nil
		}
	}
	r.recipes[characterID] = append(list, DiscoveredRecipe{
		RecipeID:     recipeID,
		IsCrafted:    true,
		DiscoveredAt: now,
		CraftedAt:    &now,
	})
	return nil
}

func (r *memoryAlchemyRepo) CountCraftedRecipes(_ context.Context, characterID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, d := range r.recipes[characterID] {
		if d.IsCrafted {
			count++
		}
	}
	return count, nil
}

func (r *memoryAlchemyRepo) SetCompAlcTitle(_ context.Context, characterID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compAlc[characterID] = true
	s := r.states[characterID]
	s.CompAlc = true
	r.states[characterID] = s
	return nil
}

func setupTestService(t *testing.T, simTime *time.Time) (*Service, *memoryCharRepo, *memoryDepotRepo, *memoryAlchemyRepo, *corecharacter.Character) {
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchemist")
	char.Money = 0 // Parity check: 0 gold required!
	charRepo.characters[char.ID] = char

	herb, _ := coreitem.NewDefinition("item-001", "Herb", 30)
	bombStone, _ := coreitem.NewDefinition("item-042", "Bomb Stone", 100)
	superHerb, _ := coreitem.NewDefinition("item-002", "Super Herb", 100)
	dragonGrass, _ := coreitem.NewDefinition("item-041", "Dragon Grass", 250)
	itemCatalog, _ := coreitem.NewCatalog([]coreitem.Definition{herb, bombStone, superHerb, dragonGrass})

	r1, _ := NewRecipe("rec-super-herb", "Synthesize Super Herb", "item-002", 1, []Ingredient{{"item-001", 2}})
	r2, _ := NewRecipe("rec-dragon-grass", "Synthesize Dragon Grass", "item-041", 1, []Ingredient{{"item-001", 1}, {"item-042", 1}})
	recipeCatalog, _ := NewRecipeCatalog([]Recipe{r1, r2})

	opts := []Option{}
	if simTime != nil {
		opts = append(opts, WithNowFunc(func() time.Time {
			return *simTime
		}))
	}

	service, err := NewService(charRepo, depotRepo, alchemyRepo, recipeCatalog, itemCatalog, opts...)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	return service, charRepo, depotRepo, alchemyRepo, &char
}

func TestSynthesize_RequiresZeroGoldFeeAndConsumesFromDepot(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 14, 0, 0, 0, timer.JST)
	svc, charRepo, depotRepo, _, char := setupTestService(t, &now)

	// Unlock recipe first
	if err := svc.UnlockRecipe(ctx, char.ID, "rec-super-herb"); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}

	// Prepare Depot with materials
	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	h1, _ := coreitem.NewInstance("item-001", 3)
	_ = dep.AddItem(h1)
	_ = depotRepo.Save(ctx, dep)

	// Character has 0 money, synthesis MUST succeed without gold fee
	res, err := svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if res.State != StateOngoing {
		t.Errorf("expected StateOngoing, got %v", res.State)
	}

	// Verify depot materials deducted: 3 - 2 = 1 remaining
	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if qty := updatedDepot.Quantity("item-001"); qty != 1 {
		t.Errorf("depot item-001 quantity = %d, want 1", qty)
	}

	// Verify character money untouched
	c, _ := charRepo.FindByID(ctx, char.ID)
	if c.Money != 0 {
		t.Errorf("character money = %d, want 0", c.Money)
	}
}

func TestSynthesize_FailsWhenMaterialsMissing(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, _, _, char := setupTestService(t, &now)

	_ = svc.UnlockRecipe(ctx, char.ID, "rec-super-herb")

	_, err := svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if !errors.Is(err, ErrInsufficientMaterials) {
		t.Errorf("expected ErrInsufficientMaterials, got %v", err)
	}
}

func TestSynthesize_FailsWhenRecipeNotLearned(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, depotRepo, _, char := setupTestService(t, &now)

	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	h1, _ := coreitem.NewInstance("item-001", 5)
	_ = dep.AddItem(h1)
	_ = depotRepo.Save(ctx, dep)

	// Not unlocked
	_, err := svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if !errors.Is(err, ErrRecipeNotLearned) {
		t.Errorf("expected ErrRecipeNotLearned, got %v", err)
	}
}

func TestSynthesize_FailsWhenSynthesisInProgress(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, timer.JST)
	svc, _, depotRepo, _, char := setupTestService(t, &now)

	_ = svc.UnlockRecipe(ctx, char.ID, "rec-super-herb")
	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	h1, _ := coreitem.NewInstance("item-001", 10)
	_ = dep.AddItem(h1)
	_ = depotRepo.Save(ctx, dep)

	_, err := svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if err != nil {
		t.Fatalf("first synthesize failed: %v", err)
	}

	// Second synthesize attempt while first is in progress MUST fail
	_, err = svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if !errors.Is(err, ErrSynthesisInProgress) {
		t.Errorf("expected ErrSynthesisInProgress, got %v", err)
	}
}

func TestSynthesize_OvernightMaturityAndClaim(t *testing.T) {
	ctx := context.Background()
	simTime := time.Date(2026, 9, 12, 20, 0, 0, 0, timer.JST)
	svc, _, depotRepo, _, char := setupTestService(t, &simTime)

	_ = svc.UnlockRecipe(ctx, char.ID, "rec-super-herb")
	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	h1, _ := coreitem.NewInstance("item-001", 2)
	_ = dep.AddItem(h1)
	_ = depotRepo.Save(ctx, dep)

	res, err := svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}

	// Expected maturity is next midnight JST: 2026-09-13 00:00:00 JST
	expectedMaturity := time.Date(2026, 9, 13, 0, 0, 0, 0, timer.JST)
	if !res.MaturesAt.Equal(expectedMaturity) {
		t.Errorf("matures at = %v, want %v", res.MaturesAt, expectedMaturity)
	}

	// Before midnight: Claim should fail
	simTime = time.Date(2026, 9, 12, 23, 59, 0, 0, timer.JST)
	_, err = svc.Claim(ctx, char.ID)
	if !errors.Is(err, ErrSynthesisNotReady) {
		t.Errorf("expected ErrSynthesisNotReady before midnight, got %v", err)
	}

	// After midnight: Claim succeeds and delivers product into Depot
	simTime = time.Date(2026, 9, 13, 6, 30, 0, 0, timer.JST)
	claimRes, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim failed after midnight: %v", err)
	}
	if claimRes.CreatedItem.DefinitionID != "item-002" {
		t.Errorf("expected item-002, got %s", claimRes.CreatedItem.DefinitionID)
	}

	// Verify product was delivered into Depot
	d, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if d.Quantity("item-002") != 1 {
		t.Errorf("depot item-002 quantity = %d, want 1", d.Quantity("item-002"))
	}
}

func TestSynthesize_HomeSleepHookCompletion(t *testing.T) {
	ctx := context.Background()
	simTime := time.Date(2026, 9, 12, 14, 0, 0, 0, timer.JST)
	svc, _, depotRepo, _, char := setupTestService(t, &simTime)

	_ = svc.UnlockRecipe(ctx, char.ID, "rec-super-herb")
	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	h1, _ := coreitem.NewInstance("item-001", 2)
	_ = dep.AddItem(h1)
	_ = depotRepo.Save(ctx, dep)

	_, err := svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}

	// Home sleep hook fires (still same afternoon, before midnight)
	if err := svc.CompleteOngoingSynthesis(ctx, char.ID); err != nil {
		t.Fatalf("CompleteOngoingSynthesis failed: %v", err)
	}

	st, err := svc.GetStatus(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if st.State != StateCompleted {
		t.Errorf("expected StateCompleted after home sleep hook, got %v", st.State)
	}

	// Claim now succeeds immediately
	claimRes, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if claimRes.CreatedItem.DefinitionID != "item-002" {
		t.Errorf("expected item-002, got %s", claimRes.CreatedItem.DefinitionID)
	}
}

func TestCompendium_100PercentCompletionAndTitle(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, depotRepo, _, char := setupTestService(t, &now)

	// Catalog has 2 recipes: rec-super-herb and rec-dragon-grass
	_ = svc.UnlockRecipe(ctx, char.ID, "rec-super-herb")
	_ = svc.UnlockRecipe(ctx, char.ID, "rec-dragon-grass")

	comp, err := svc.GetCompendium(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCompendium failed: %v", err)
	}
	if comp.TotalRecipes != 2 || comp.LearnedCount != 2 || comp.CraftedCount != 0 {
		t.Errorf("compendium initial: total=%d, learned=%d, crafted=%d", comp.TotalRecipes, comp.LearnedCount, comp.CraftedCount)
	}
	if comp.Entries[0].ResultItemName != "？？？" {
		t.Errorf("expected masked result item name ？？？, got %s", comp.Entries[0].ResultItemName)
	}

	// Craft first recipe
	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	h1, _ := coreitem.NewInstance("item-001", 10)
	b1, _ := coreitem.NewInstance("item-042", 5)
	_ = dep.AddItem(h1)
	_ = dep.AddItem(b1)
	_ = depotRepo.Save(ctx, dep)

	_, _ = svc.Synthesize(ctx, char.ID, "rec-super-herb")
	_ = svc.CompleteOngoingSynthesis(ctx, char.ID)
	claim1, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim 1 failed: %v", err)
	}
	if claim1.CompAlcAwarded {
		t.Errorf("comp_alc should not be awarded at 50%% completion")
	}

	// Craft second recipe (100% completion)
	_, _ = svc.Synthesize(ctx, char.ID, "rec-dragon-grass")
	_ = svc.CompleteOngoingSynthesis(ctx, char.ID)
	claim2, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim 2 failed: %v", err)
	}
	if !claim2.CompAlcAwarded {
		t.Errorf("comp_alc MUST be awarded at 100%% completion")
	}

	finalComp, _ := svc.GetCompendium(ctx, char.ID)
	if finalComp.CompletionPercentage != 100 || !finalComp.CompAlc {
		t.Errorf("expected 100%% completion and CompAlc true, got %+v", finalComp)
	}
	for _, entry := range finalComp.Entries {
		if entry.RecipeID == "rec-super-herb" && entry.ResultItemName != "Super Herb" {
			t.Errorf("expected Super Herb for rec-super-herb, got %s", entry.ResultItemName)
		}
		if entry.RecipeID == "rec-dragon-grass" && entry.ResultItemName != "Dragon Grass" {
			t.Errorf("expected Dragon Grass for rec-dragon-grass, got %s", entry.ResultItemName)
		}
	}
}

func TestLearnRecipe_PoolFilterAndExhaustion(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, _, _, char := setupTestService(t, &now)

	// Learn recipe with pool containing item-001 (Herb)
	r1, err := svc.LearnRecipe(ctx, char.ID, []string{"item-001"})
	if err != nil {
		t.Fatalf("LearnRecipe failed: %v", err)
	}
	if r1.ID != "rec-super-herb" && r1.ID != "rec-dragon-grass" {
		t.Errorf("unexpected learned recipe: %s", r1.ID)
	}

	// Learn second recipe
	r2, err := svc.LearnRecipe(ctx, char.ID, []string{"item-001"})
	if err != nil {
		t.Fatalf("LearnRecipe 2 failed: %v", err)
	}
	if r2.ID == r1.ID {
		t.Errorf("learned duplicate recipe: %s", r2.ID)
	}

	// Third attempt must return ErrNoRecipesToLearn
	_, err = svc.LearnRecipe(ctx, char.ID, []string{"item-001"})
	if !errors.Is(err, ErrNoRecipesToLearn) {
		t.Errorf("expected ErrNoRecipesToLearn, got %v", err)
	}
}
