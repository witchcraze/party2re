package home

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

type mockRecipeLearner struct {
	lastCharID string
	lastPool   []string
	result     LearnedRecipe
	err        error
}

func (m *mockRecipeLearner) LearnRecipe(_ context.Context, characterID string, pool []string) (LearnedRecipe, error) {
	m.lastCharID = characterID
	m.lastPool = pool
	if m.err != nil {
		return LearnedRecipe{}, m.err
	}
	return m.result, nil
}

type mockHomeTxRunner struct {
	chars map[string]*corecharacter.Character
	invs  map[string]*coreinventory.Inventory
	depot map[string]*depot.Depot
}

func (r *mockHomeTxRunner) ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
	c, ok := r.chars[req.CharacterID]
	if !ok {
		return nil, economy.ErrCharacterNotFound
	}
	charCopy := *c

	if req.LockInventory && req.Cost.ItemInstanceID != "" {
		inv, ok := r.invs[req.CharacterID]
		if !ok {
			return nil, economy.ErrItemNotFound
		}
		if err := inv.Consume(req.Cost.ItemInstanceID, req.Cost.ItemInstanceQty); err != nil {
			return nil, economy.ErrInsufficientItemQuantity
		}
	}

	tc := &economy.TxContext{
		Context:   ctx,
		Character: charCopy,
	}

	if err := fn(tc); err != nil {
		return nil, err
	}

	*c = tc.Character
	return &economy.TransactionResult{Character: tc.Character}, nil
}

func setupRecipeTestEnvironment() (
	context.Context,
	*mockCatalog,
	*mockInventoryManager,
	*mockDepotManager,
	map[string]corecharacter.Character,
	CharacterReader,
	CharacterUpdater,
	Repository,
) {
	ctx := context.Background()

	catalog := &mockCatalog{
		defs: map[string]coreitem.Definition{
			"item-127": {
				ID:            "item-127",
				Name:          "基本錬金レシピ",
				Price:         500,
				UsageCategory: coreitem.UsageCategoryAnytime,
			},
			"item-128": {
				ID:            "item-128",
				Name:          "応用錬金レシピ",
				Price:         2000,
				UsageCategory: coreitem.UsageCategoryAnytime,
			},
			"item-129": {
				ID:            "item-129",
				Name:          "神の錬金レシピ",
				Price:         7000,
				UsageCategory: coreitem.UsageCategoryAnytime,
			},
		},
	}

	chars := map[string]corecharacter.Character{
		"char-hero": {
			ID:       "char-hero",
			PlayerID: "player-1",
			Name:     "勇者",
			Level:    10,
		},
	}

	charReader := &mockCharReader{chars: chars}
	charUpdater := &mockCharUpdater{chars: chars}
	invMgr := &mockInventoryManager{invs: make(map[string]coreinventory.Inventory)}
	depotMgr := &mockDepotManager{depots: make(map[string]depot.Depot)}
	repo := newMockHomeRepo(chars)

	return ctx, catalog, invMgr, depotMgr, chars, charReader, charUpdater, repo
}

func TestRecipeScroll_Pools(t *testing.T) {
	if len(basicRecipePool) != 34 {
		t.Fatalf("expected 34 items in basicRecipePool, got %d", len(basicRecipePool))
	}
	if len(advancedRecipePool) != 30 {
		t.Fatalf("expected 30 items in advancedRecipePool, got %d", len(advancedRecipePool))
	}
}

func TestRecipeScroll_BasicRecipe_Learn(t *testing.T) {
	ctx, catalog, invMgr, depotMgr, chars, charReader, charUpdater, repo := setupRecipeTestEnvironment()

	inv, _ := coreinventory.New("char-hero")
	_ = inv.Add(coreitem.Instance{ID: "inst-127", DefinitionID: "item-127", Quantity: 1})
	invMgr.invs["char-hero"] = inv

	learner := &mockRecipeLearner{
		result: LearnedRecipe{BaseName: "薬草", MaterialName: "薬草"},
	}

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithRecipeLearner(learner),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.UseHomeItem(ctx, "char-hero", "inst-127", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem failed: %v", err)
	}

	if !res.Consumed {
		t.Errorf("expected item to be consumed")
	}
	expectedMsg := "勇者 は錬金レシピを読んだ！【薬草 × 薬草 ＝ ？？？】の錬金方法を習得した！"
	if res.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
	}

	if learner.lastCharID != "char-hero" {
		t.Errorf("expected learner characterID 'char-hero', got %q", learner.lastCharID)
	}
	if !reflect.DeepEqual(learner.lastPool, basicRecipePool) {
		t.Errorf("expected basicRecipePool passed to learner, got %v", learner.lastPool)
	}

	// Verify inventory consumed
	curInv, _ := invMgr.FindByCharacterID(ctx, "char-hero")
	if len(curInv.Items) != 0 {
		t.Errorf("expected 0 items in inventory after consumption, got %d", len(curInv.Items))
	}
	_ = chars
}

func TestRecipeScroll_AdvancedRecipe_Learn(t *testing.T) {
	ctx, catalog, invMgr, depotMgr, _, charReader, charUpdater, repo := setupRecipeTestEnvironment()

	inv, _ := coreinventory.New("char-hero")
	_ = inv.Add(coreitem.Instance{ID: "inst-128", DefinitionID: "item-128", Quantity: 1})
	invMgr.invs["char-hero"] = inv

	learner := &mockRecipeLearner{
		result: LearnedRecipe{BaseName: "世界樹の葉", MaterialName: "魔法の聖水"},
	}

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithRecipeLearner(learner),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.UseHomeItem(ctx, "char-hero", "inst-128", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem failed: %v", err)
	}

	if !res.Consumed {
		t.Errorf("expected item to be consumed")
	}
	expectedMsg := "勇者 は錬金レシピを読んだ！【世界樹の葉 × 魔法の聖水 ＝ ？？？】の錬金方法を習得した！"
	if res.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
	}

	if !reflect.DeepEqual(learner.lastPool, advancedRecipePool) {
		t.Errorf("expected advancedRecipePool passed to learner, got %v", learner.lastPool)
	}
}

func TestRecipeScroll_GodRecipe_Learn(t *testing.T) {
	ctx, catalog, invMgr, depotMgr, _, charReader, charUpdater, repo := setupRecipeTestEnvironment()

	inv, _ := coreinventory.New("char-hero")
	_ = inv.Add(coreitem.Instance{ID: "inst-129", DefinitionID: "item-129", Quantity: 1})
	invMgr.invs["char-hero"] = inv

	learner := &mockRecipeLearner{
		result: LearnedRecipe{BaseName: "幸せの帽子", MaterialName: "幸せの種"},
	}

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithRecipeLearner(learner),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.UseHomeItem(ctx, "char-hero", "inst-129", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem failed: %v", err)
	}

	if !res.Consumed {
		t.Errorf("expected item to be consumed")
	}
	expectedMsg := "勇者 は錬金レシピを読んだ！【幸せの帽子 × 幸せの種 ＝ ？？？】の錬金方法を習得した！"
	if res.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
	}

	if learner.lastPool != nil {
		t.Errorf("expected nil pool for GodRecipe, got %v", learner.lastPool)
	}
}

func TestRecipeScroll_ExhaustedPool_StillConsumesItem(t *testing.T) {
	ctx, catalog, invMgr, depotMgr, _, charReader, charUpdater, repo := setupRecipeTestEnvironment()

	inv, _ := coreinventory.New("char-hero")
	_ = inv.Add(coreitem.Instance{ID: "inst-127", DefinitionID: "item-127", Quantity: 1})
	invMgr.invs["char-hero"] = inv

	learner := &mockRecipeLearner{
		err: ErrNoRecipesToLearn,
	}

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithRecipeLearner(learner),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.UseHomeItem(ctx, "char-hero", "inst-127", "inventory")
	if err != nil {
		t.Fatalf("expected nil error on exhausted pool, got: %v", err)
	}

	if !res.Consumed {
		t.Errorf("expected item to be consumed even when pool exhausted")
	}
	expectedMsg := "この錬金レシピからこれ以上習得できる錬金方法はないようだ…"
	if res.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
	}

	// Verify inventory consumed
	curInv, _ := invMgr.FindByCharacterID(ctx, "char-hero")
	if len(curInv.Items) != 0 {
		t.Errorf("expected item to be consumed from inventory")
	}
}

func TestRecipeScroll_LearnerUnavailable(t *testing.T) {
	ctx, catalog, invMgr, depotMgr, _, charReader, charUpdater, repo := setupRecipeTestEnvironment()

	inv, _ := coreinventory.New("char-hero")
	_ = inv.Add(coreitem.Instance{ID: "inst-127", DefinitionID: "item-127", Quantity: 1})
	invMgr.invs["char-hero"] = inv

	// No recipe learner configured
	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.UseHomeItem(ctx, "char-hero", "inst-127", "inventory")
	if !errors.Is(err, ErrRecipeLearnerUnavailable) {
		t.Fatalf("expected ErrRecipeLearnerUnavailable, got: %v", err)
	}

	// Verify inventory NOT consumed
	curInv, _ := invMgr.FindByCharacterID(ctx, "char-hero")
	if len(curInv.Items) != 1 {
		t.Errorf("expected item to remain in inventory on error")
	}
}

func TestRecipeScroll_LearnerUnexpectedError(t *testing.T) {
	ctx, catalog, invMgr, depotMgr, _, charReader, charUpdater, repo := setupRecipeTestEnvironment()

	inv, _ := coreinventory.New("char-hero")
	_ = inv.Add(coreitem.Instance{ID: "inst-127", DefinitionID: "item-127", Quantity: 1})
	invMgr.invs["char-hero"] = inv

	learnerErr := errors.New("db failure")
	learner := &mockRecipeLearner{
		err: learnerErr,
	}

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithRecipeLearner(learner),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.UseHomeItem(ctx, "char-hero", "inst-127", "inventory")
	if !errors.Is(err, learnerErr) {
		t.Fatalf("expected %v, got %v", learnerErr, err)
	}

	// Verify inventory NOT consumed
	curInv, _ := invMgr.FindByCharacterID(ctx, "char-hero")
	if len(curInv.Items) != 1 {
		t.Errorf("expected item to remain in inventory on error")
	}
}

func TestRecipeScroll_WithTransactionRunner(t *testing.T) {
	ctx, catalog, invMgr, depotMgr, chars, charReader, charUpdater, repo := setupRecipeTestEnvironment()

	char := chars["char-hero"]
	inv, _ := coreinventory.New("char-hero")
	_ = inv.Add(coreitem.Instance{ID: "inst-127", DefinitionID: "item-127", Quantity: 1})
	invMgr.invs["char-hero"] = inv

	runner := &mockHomeTxRunner{
		chars: map[string]*corecharacter.Character{"char-hero": &char},
		invs:  map[string]*coreinventory.Inventory{"char-hero": &inv},
		depot: make(map[string]*depot.Depot),
	}

	learner := &mockRecipeLearner{
		result: LearnedRecipe{BaseName: "ひのきの棒", MaterialName: "こんぼう"},
	}

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithRecipeLearner(learner),
		WithTransactionRunner(runner),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.UseHomeItem(ctx, "char-hero", "inst-127", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem failed: %v", err)
	}

	if !res.Consumed {
		t.Errorf("expected consumed = true")
	}
	expectedMsg := "勇者 は錬金レシピを読んだ！【ひのきの棒 × こんぼう ＝ ？？？】の錬金方法を習得した！"
	if res.Message != expectedMsg {
		t.Errorf("expected %q, got %q", expectedMsg, res.Message)
	}

	// Exhausted with runner
	inv2, _ := coreinventory.New("char-hero")
	_ = inv2.Add(coreitem.Instance{ID: "inst-127-2", DefinitionID: "item-127", Quantity: 1})
	invMgr.invs["char-hero"] = inv2
	runner.invs["char-hero"] = &inv2

	learner.err = ErrNoRecipesToLearn
	res2, err := svc.UseHomeItem(ctx, "char-hero", "inst-127-2", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem failed: %v", err)
	}
	if !res2.Consumed {
		t.Errorf("expected consumed = true on exhausted")
	}
	if res2.Message != "この錬金レシピからこれ以上習得できる錬金方法はないようだ…" {
		t.Errorf("unexpected message: %q", res2.Message)
	}
}

func TestRecipeScroll_DepotConsumption(t *testing.T) {
	ctx, catalog, invMgr, depotMgr, _, charReader, charUpdater, repo := setupRecipeTestEnvironment()

	dp, _ := depot.NewDepot("char-hero")
	dp.Items = append(dp.Items, coreitem.Instance{ID: "inst-depot-127", DefinitionID: "item-127", Quantity: 1})
	depotMgr.depots["char-hero"] = dp

	learner := &mockRecipeLearner{
		result: LearnedRecipe{BaseName: "薬草", MaterialName: "薬草"},
	}

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithRecipeLearner(learner),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.UseHomeItem(ctx, "char-hero", "inst-depot-127", "depot")
	if err != nil {
		t.Fatalf("UseHomeItem from depot failed: %v", err)
	}

	if !res.Consumed {
		t.Errorf("expected item consumed from depot")
	}
	if len(depotMgr.depots["char-hero"].Items) != 0 {
		t.Errorf("expected depot items empty after consume")
	}
}

// Suppress unused imports
var _ = time.Now
