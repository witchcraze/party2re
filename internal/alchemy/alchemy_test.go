package alchemy

import (
	"context"
	"errors"
	"slices"
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
		return depot.Depot{}, depot.ErrNotFound
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

type mockLegendInductor struct {
	calls []legendCall
	err   error
}

type legendCall struct {
	Category    string
	CharacterID string
}

func (m *mockLegendInductor) RecordLegend(_ context.Context, category, characterID string) error {
	m.calls = append(m.calls, legendCall{Category: category, CharacterID: characterID})
	return m.err
}

type mockNews struct{ titles []string }

func (m *mockNews) PublishNews(_ context.Context, category, title, _, _ string, _ time.Time) error {
	m.titles = append(m.titles, category+":"+title)
	return nil
}

func setupTestService(t *testing.T, simTime *time.Time) (*Service, *memoryCharRepo, *memoryDepotRepo, *memoryAlchemyRepo, *corecharacter.Character) {
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchem")
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

func TestSynthesize_ConsumesEveryGearInstanceBeforeStarting(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 14, 0, 0, 0, timer.JST)
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchem")
	charRepo.characters[char.ID] = char

	herb, _ := coreitem.NewDefinition("item-001", "Herb", 30)
	blade, _ := coreitem.NewEquipmentDefinition("weapon-06", "Blade", 100, coreitem.SlotMainHand)
	product, _ := coreitem.NewDefinition("item-002", "Super Herb", 100)
	itemCatalog, _ := coreitem.NewCatalog([]coreitem.Definition{herb, blade, product})
	recipe, _ := NewRecipe("rec-blade-fusion", "Fuse Blades", "item-002", 1, []Ingredient{{"weapon-06", 2}})
	recipeCatalog, _ := NewRecipeCatalog([]Recipe{recipe})
	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipeCatalog, itemCatalog, WithNowFunc(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	if err := svc.UnlockRecipe(ctx, char.ID, "rec-blade-fusion"); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}

	// Two identical gear instances followed by an unrelated item: the second
	// gear slot must not be skipped when the first one is removed.
	blade1, _ := coreitem.NewInstance("weapon-06", 1)
	blade2, _ := coreitem.NewInstance("weapon-06", 1)
	herbStack, _ := coreitem.NewInstance("item-001", 1)
	dep, _ := depot.NewDepot(char.ID)
	dep.Items = []coreitem.Instance{blade1, blade2, herbStack}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("Save depot failed: %v", err)
	}

	if _, err := svc.Synthesize(ctx, char.ID, "rec-blade-fusion"); err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}

	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if qty := updatedDepot.Quantity("weapon-06"); qty != 0 {
		t.Errorf("weapon-06 quantity = %d, want 0", qty)
	}
	if qty := updatedDepot.Quantity("item-001"); qty != 1 {
		t.Errorf("item-001 quantity = %d, want 1", qty)
	}
}

func TestSynthesize_ConsumesEveryGearInstanceForRealRecipes(t *testing.T) {
	ctx := context.Background()
	recipeCatalog, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog failed: %v", err)
	}

	for _, recipeID := range []string{"recipe-043", "recipe-064", "recipe-065", "recipe-084"} {
		t.Run(recipeID, func(t *testing.T) {
			recipe, err := recipeCatalog.FindByID(recipeID)
			if err != nil {
				t.Fatalf("FindByID failed: %v", err)
			}
			gearID := recipe.Ingredients[0].DefinitionID

			depotRepo := newMemoryDepotRepo()
			alchemyRepo := newMemoryAlchemyRepo()
			svc, char := newConsumptionTestService(t, alchemyRepo, depotRepo, recipe)
			if err := svc.UnlockRecipe(ctx, char.ID, recipeID); err != nil {
				t.Fatalf("UnlockRecipe failed: %v", err)
			}

			gear1, _ := coreitem.NewInstance(gearID, 1)
			gear2, _ := coreitem.NewInstance(gearID, 1)
			herbStack, _ := coreitem.NewInstance("item-001", 1)
			dep, _ := depot.NewDepot(char.ID)
			dep.Items = []coreitem.Instance{gear1, gear2, herbStack}
			if err := depotRepo.Save(ctx, dep); err != nil {
				t.Fatalf("Save depot failed: %v", err)
			}

			if _, err := svc.Synthesize(ctx, char.ID, recipeID); err != nil {
				t.Fatalf("Synthesize failed: %v", err)
			}

			updatedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
			if qty := updatedDepot.Quantity(gearID); qty != 0 {
				t.Errorf("%s quantity = %d, want 0", gearID, qty)
			}
			if qty := updatedDepot.Quantity("item-001"); qty != 1 {
				t.Errorf("item-001 quantity = %d, want 1", qty)
			}
		})
	}
}

func newConsumptionTestService(t *testing.T, alchemyRepo Repository, depots DepotRepository, recipes ...Recipe) (*Service, *corecharacter.Character) {
	t.Helper()
	now := time.Date(2026, 9, 12, 14, 0, 0, 0, timer.JST)
	charRepo := newMemoryCharRepo()

	char, _ := corecharacter.New("Alchem")
	charRepo.characters[char.ID] = char

	herb, _ := coreitem.NewDefinition("item-001", "Herb", 30)
	blade, _ := coreitem.NewEquipmentDefinition("weapon-06", "Blade", 100, coreitem.SlotMainHand)
	product, _ := coreitem.NewDefinition("item-002", "Super Herb", 100)
	itemCatalog, _ := coreitem.NewCatalog([]coreitem.Definition{herb, blade, product})
	recipeCatalog, _ := NewRecipeCatalog(recipes)
	svc, err := NewService(charRepo, depots, alchemyRepo, recipeCatalog, itemCatalog, WithNowFunc(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	return svc, &char
}

func TestSynthesize_ConsumesMultiQuantityStacksInIngredientOrder(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()
	recipe, _ := NewRecipe("rec-blade-herb", "Blade and Herb", "item-002", 1, []Ingredient{{"item-001", 3}, {"weapon-06", 2}})
	svc, char := newConsumptionTestService(t, alchemyRepo, depotRepo, recipe)
	if err := svc.UnlockRecipe(ctx, char.ID, "rec-blade-herb"); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}

	// Herb comes first in the recipe and its 3 units span two slots. Removing the
	// first slot must not hide the second herb slot from the loop, and the blade
	// stack must give up only the 2 units the recipe needs.
	bladeStack, _ := coreitem.NewInstance("weapon-06", 3)
	herbSlot1, _ := coreitem.NewInstance("item-001", 1)
	herbSlot2, _ := coreitem.NewInstance("item-001", 2)
	bladeSingle, _ := coreitem.NewInstance("weapon-06", 1)
	dep, _ := depot.NewDepot(char.ID)
	dep.Items = []coreitem.Instance{bladeStack, herbSlot1, herbSlot2, bladeSingle}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("Save depot failed: %v", err)
	}

	if _, err := svc.Synthesize(ctx, char.ID, "rec-blade-herb"); err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}

	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if qty := updatedDepot.Quantity("item-001"); qty != 0 {
		t.Errorf("item-001 quantity = %d, want 0", qty)
	}
	if qty := updatedDepot.Quantity("weapon-06"); qty != 2 {
		t.Errorf("weapon-06 quantity = %d, want 2", qty)
	}
}

func TestSynthesize_RejectsShortfallAcrossDuplicateIngredients(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()
	// Each entry passes the upfront check on its own (3 >= 2), but together they
	// need 4 blades and the depot holds 3.
	recipe, _ := NewRecipe("rec-blade-pair", "Blade Pair", "item-002", 1, []Ingredient{{"weapon-06", 2}, {"weapon-06", 2}})
	svc, char := newConsumptionTestService(t, alchemyRepo, depotRepo, recipe)
	if err := svc.UnlockRecipe(ctx, char.ID, "rec-blade-pair"); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}

	blade, _ := coreitem.NewInstance("weapon-06", 3)
	dep, _ := depot.NewDepot(char.ID)
	dep.Items = []coreitem.Instance{blade}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("Save depot failed: %v", err)
	}

	_, err := svc.Synthesize(ctx, char.ID, "rec-blade-pair")
	if !errors.Is(err, ErrInsufficientMaterials) {
		t.Fatalf("expected ErrInsufficientMaterials, got %v", err)
	}

	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if qty := updatedDepot.Quantity("weapon-06"); qty != 3 {
		t.Errorf("weapon-06 quantity = %d, want 3 after rejected synthesis", qty)
	}
	state, _ := alchemyRepo.GetSynthesisState(ctx, char.ID)
	if state.State == StateOngoing {
		t.Errorf("synthesis state = %v after rejected synthesis, want not ongoing", state.State)
	}
}

// failingSaveDepotRepo fails every depot save so the rollback path can be checked.
type failingSaveDepotRepo struct {
	*memoryDepotRepo
}

func (r *failingSaveDepotRepo) Save(context.Context, depot.Depot) error {
	return errors.New("depot save failed")
}

func TestSynthesize_RollsBackWhenDepotSaveFails(t *testing.T) {
	ctx := context.Background()
	depotRepo := &failingSaveDepotRepo{memoryDepotRepo: newMemoryDepotRepo()}
	alchemyRepo := newMemoryAlchemyRepo()
	recipe, _ := NewRecipe("rec-blade-fusion", "Fuse Blades", "item-002", 1, []Ingredient{{"weapon-06", 2}})
	svc, char := newConsumptionTestService(t, alchemyRepo, depotRepo, recipe)
	if err := svc.UnlockRecipe(ctx, char.ID, "rec-blade-fusion"); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}

	blade1, _ := coreitem.NewInstance("weapon-06", 1)
	blade2, _ := coreitem.NewInstance("weapon-06", 1)
	herbStack, _ := coreitem.NewInstance("item-001", 1)
	dep, _ := depot.NewDepot(char.ID)
	dep.Items = []coreitem.Instance{blade1, blade2, herbStack}
	if err := depotRepo.memoryDepotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("Save depot failed: %v", err)
	}

	if _, err := svc.Synthesize(ctx, char.ID, "rec-blade-fusion"); err == nil {
		t.Fatal("Synthesize succeeded, want depot save error")
	}

	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if qty := updatedDepot.Quantity("weapon-06"); qty != 2 {
		t.Errorf("weapon-06 quantity = %d, want 2 after failed save", qty)
	}
	if qty := updatedDepot.Quantity("item-001"); qty != 1 {
		t.Errorf("item-001 quantity = %d, want 1 after failed save", qty)
	}
	state, _ := alchemyRepo.GetSynthesisState(ctx, char.ID)
	if state.State == StateOngoing {
		t.Errorf("synthesis state = %v after failed save, want not ongoing", state.State)
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

func TestSynthesize_CompletesOnlyThroughSleep(t *testing.T) {
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

	if res.State != StateOngoing {
		t.Errorf("state = %v, want ongoing", res.State)
	}

	// lib/home.cgi:191 is the only completion path, so elapsed time changes nothing.
	simTime = time.Date(2026, 9, 14, 6, 30, 0, 0, timer.JST)
	if st, err := svc.GetStatus(ctx, char.ID); err != nil || st.State != StateOngoing {
		t.Errorf("status after midnight = %+v, %v; want ongoing", st, err)
	}
	if _, err = svc.Claim(ctx, char.ID); !errors.Is(err, ErrSynthesisNotReady) {
		t.Errorf("expected ErrSynthesisNotReady after midnight without sleep, got %v", err)
	}
	if _, err = svc.Synthesize(ctx, char.ID, "rec-super-herb"); !errors.Is(err, ErrSynthesisInProgress) {
		t.Errorf("expected ErrSynthesisInProgress after midnight without sleep, got %v", err)
	}

	if err := svc.CompleteOngoingSynthesis(ctx, char.ID); err != nil {
		t.Fatalf("CompleteOngoingSynthesis failed: %v", err)
	}
	claimRes, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim failed after sleep: %v", err)
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

// craftAll crafts both test recipes through delivery.
func craftAll(t *testing.T, svc *Service, depotRepo *memoryDepotRepo, charID string) {
	t.Helper()
	ctx := context.Background()
	_ = svc.UnlockRecipe(ctx, charID, "rec-super-herb")
	_ = svc.UnlockRecipe(ctx, charID, "rec-dragon-grass")
	dep, _ := depot.NewDepot(charID)
	dep.Capacity = 10
	h1, _ := coreitem.NewInstance("item-001", 10)
	b1, _ := coreitem.NewInstance("item-042", 5)
	_ = dep.AddItem(h1)
	_ = dep.AddItem(b1)
	_ = depotRepo.Save(ctx, dep)
	for _, id := range []string{"rec-super-herb", "rec-dragon-grass"} {
		if _, err := svc.Synthesize(ctx, charID, id); err != nil {
			t.Fatalf("Synthesize %s: %v", id, err)
		}
		if err := svc.CompleteOngoingSynthesis(ctx, charID); err != nil {
			t.Fatalf("CompleteOngoingSynthesis %s: %v", id, err)
		}
		if _, err := svc.Claim(ctx, charID); err != nil {
			t.Fatalf("Claim %s: %v", id, err)
		}
	}
}

// Legacy alchemy.cgi awards comp_alc only when れしぴ is viewed, never at delivery.
func TestViewRecipes_AwardsCompAlcOnceOnFirstCompleteView(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, depotRepo, _, char := setupTestService(t, &now)
	legend := &mockLegendInductor{}
	news := &mockNews{}
	svc.SetLegendInductor(legend)
	svc.SetNewsPublisher(news)

	_ = svc.UnlockRecipe(ctx, char.ID, "rec-super-herb")
	partial, err := svc.ViewRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("ViewRecipes before completion: %v", err)
	}
	if partial.CompAlcAwarded || partial.Compendium.LearnedCount != 1 {
		t.Fatalf("incomplete view must not award: %+v", partial)
	}

	craftAll(t, svc, depotRepo, char.ID)
	if len(legend.calls) != 0 || len(news.titles) != 0 {
		t.Fatalf("delivery must not award comp_alc: legend=%v news=%v", legend.calls, news.titles)
	}
	if comp, _ := svc.GetCompendium(ctx, char.ID); comp.CompAlc || comp.CompletionPercentage != 100 {
		t.Fatalf("observation must not award: %+v", comp)
	}

	first, err := svc.ViewRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("ViewRecipes: %v", err)
	}
	if !first.CompAlcAwarded || !first.Compendium.CompAlc || first.Compendium.CompletionPercentage != 100 {
		t.Fatalf("first complete view must award: %+v", first)
	}
	if len(legend.calls) != 1 || legend.calls[0] != (legendCall{Category: "comp_alc", CharacterID: char.ID}) {
		t.Fatalf("unexpected legend calls: %+v", legend.calls)
	}
	if want := []string{"alchemy:" + char.Name + "が錬金レシピをコンプリートする！"}; !slices.Equal(news.titles, want) {
		t.Fatalf("news = %v, want %v", news.titles, want)
	}

	again, err := svc.ViewRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("ViewRecipes again: %v", err)
	}
	if again.CompAlcAwarded || !again.Compendium.CompAlc || len(legend.calls) != 1 || len(news.titles) != 1 {
		t.Fatalf("repeat view must not award again: %+v legend=%d news=%d", again, len(legend.calls), len(news.titles))
	}
}

func TestViewRecipes_LegendFailure_PropagatesWithoutAward(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, depotRepo, _, char := setupTestService(t, &now)
	errLegendInjected := errors.New("legend injection error")
	news := &mockNews{}
	svc.SetLegendInductor(&mockLegendInductor{err: errLegendInjected})
	svc.SetNewsPublisher(news)
	craftAll(t, svc, depotRepo, char.ID)

	if _, err := svc.ViewRecipes(ctx, char.ID); !errors.Is(err, errLegendInjected) {
		t.Fatalf("expected errLegendInjected, got %v", err)
	}
	if len(news.titles) != 0 {
		t.Fatalf("failed award must not publish news: %v", news.titles)
	}
}

func TestClaim_SynthesisHookFailure_PropagatesError(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, depotRepo, _, char := setupTestService(t, &now)
	errSynthesisInjected := errors.New("synthesis hook failure")
	svc.SetSynthesisHook(func(_ context.Context, _, _ string) error {
		return errSynthesisInjected
	})

	_ = svc.UnlockRecipe(ctx, char.ID, "rec-super-herb")

	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	h1, _ := coreitem.NewInstance("item-001", 10)
	_ = dep.AddItem(h1)
	_ = depotRepo.Save(ctx, dep)

	_, _ = svc.Synthesize(ctx, char.ID, "rec-super-herb")
	_ = svc.CompleteOngoingSynthesis(ctx, char.ID)
	_, err := svc.Claim(ctx, char.ID)
	if err == nil {
		t.Fatal("expected error from Claim when SynthesisHook fails, got nil")
	}
	if !errors.Is(err, errSynthesisInjected) {
		t.Errorf("expected errSynthesisInjected, got: %v", err)
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

func TestSynthesize_DepotNotFound_ReturnsErrInsufficientMaterials(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, _, _, char := setupTestService(t, &now)

	// Unlock recipe
	if err := svc.UnlockRecipe(ctx, char.ID, "rec-super-herb"); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}

	// Character has NO depot record saved
	_, err := svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if !errors.Is(err, ErrInsufficientMaterials) {
		t.Fatalf("expected ErrInsufficientMaterials, got %v", err)
	}
}

func TestClaim_DepotNotFound_CreatesDepotAndRefreshesCapacity(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, charRepo, depotRepo, alchemyRepo, char := setupTestService(t, &now)

	char.JobLevel = 10
	char.OverDepot = 1
	charRepo.characters[char.ID] = *char

	// Synthesis state is completed
	alchemyRepo.states[char.ID] = Synthesis{
		CharacterID: char.ID,
		RecipeID:    "rec-super-herb",
		State:       StateCompleted,
	}

	// Character has NO depot record saved
	res, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if res.CreatedItem.DefinitionID != "item-002" {
		t.Errorf("expected item-002, got %s", res.CreatedItem.DefinitionID)
	}

	createdDepot, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	expectedCap := depot.CalculateCapacity(10, 0, 1)
	if createdDepot.Capacity != expectedCap {
		t.Errorf("expected refreshed capacity %d, got %d", expectedCap, createdDepot.Capacity)
	}
}

func TestClaim_RefreshesStaleDepotCapacity(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, charRepo, depotRepo, alchemyRepo, char := setupTestService(t, &now)

	char.JobLevel = 29
	charRepo.characters[char.ID] = *char

	// Existing depot with capacity 1 and 1 item (full according to stale capacity)
	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 1
	dItem, _ := coreitem.NewInstance("item-999", 1)
	_ = dep.AddItem(dItem)
	_ = depotRepo.Save(ctx, dep)

	alchemyRepo.states[char.ID] = Synthesis{
		CharacterID: char.ID,
		RecipeID:    "rec-super-herb",
		State:       StateCompleted,
	}

	// Claim should refresh capacity to 150 (JobLevel 29) and succeed
	res, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim failed due to stale capacity: %v", err)
	}
	if res.CreatedItem.DefinitionID != "item-002" {
		t.Errorf("expected item-002, got %s", res.CreatedItem.DefinitionID)
	}

	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if updatedDepot.Capacity != 150 {
		t.Errorf("expected capacity 150, got %d", updatedDepot.Capacity)
	}
	if len(updatedDepot.Items) != 2 {
		t.Errorf("expected 2 items in depot, got %d", len(updatedDepot.Items))
	}
}

type lockLog []string

type lockOrderCharRepo struct {
	*memoryCharRepo
	log *lockLog
}

func (r lockOrderCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	*r.log = append(*r.log, "characters")
	return r.memoryCharRepo.FindByIDForUpdate(ctx, id)
}

type lockOrderAlchemyRepo struct {
	*memoryAlchemyRepo
	log *lockLog
}

func (r lockOrderAlchemyRepo) GetSynthesisStateForUpdate(ctx context.Context, id string) (Synthesis, error) {
	*r.log = append(*r.log, "character_alchemy")
	return r.memoryAlchemyRepo.GetSynthesisStateForUpdate(ctx, id)
}

// The legend insert's foreign key locks characters, so ViewRecipes must take
// characters before character_alchemy like Claim and Synthesize.
func TestViewRecipes_LocksCharacterBeforeSynthesisState(t *testing.T) {
	now := time.Now().UTC()
	svc, charRepo, depotRepo, alcRepo, char := setupTestService(t, &now)
	svc.SetLegendInductor(&mockLegendInductor{})
	craftAll(t, svc, depotRepo, char.ID)
	var log lockLog
	svc.characters = lockOrderCharRepo{charRepo, &log}
	svc.repo = lockOrderAlchemyRepo{alcRepo, &log}

	res, err := svc.ViewRecipes(context.Background(), char.ID)
	if err != nil || !res.CompAlcAwarded {
		t.Fatalf("ViewRecipes: %+v, %v", res, err)
	}
	if want := (lockLog{"characters", "character_alchemy"}); !slices.Equal(log, want) {
		t.Fatalf("lock order = %v, want %v", log, want)
	}
}
