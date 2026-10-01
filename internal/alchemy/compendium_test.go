package alchemy

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

func TestLearnRecipe_BaseIngredientOnly(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchemist")
	charRepo.characters[char.ID] = char

	itemA, _ := coreitem.NewDefinition("item-A", "BaseHerb", 10)
	itemB, _ := coreitem.NewDefinition("item-B", "BombStone", 20)
	itemC, _ := coreitem.NewDefinition("item-C", "ResultOne", 50)
	itemD, _ := coreitem.NewDefinition("item-D", "ResultTwo", 60)
	items, _ := coreitem.NewCatalog([]coreitem.Definition{itemA, itemB, itemC, itemD})

	// Recipe 1: Base is item-A, Secondary is item-B
	r1, _ := NewRecipe("rec-1", "Herb + Stone", "item-C", 1, []Ingredient{{"item-A", 1}, {"item-B", 1}})
	// Recipe 2: Base is item-B, Secondary is item-A
	r2, _ := NewRecipe("rec-2", "Stone + Herb", "item-D", 1, []Ingredient{{"item-B", 1}, {"item-A", 1}})
	recipes, _ := NewRecipeCatalog([]Recipe{r1, r2})

	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipes, items)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Learning with allowedBases = ["BaseHerb"] MUST only match rec-1
	learned, err := svc.LearnRecipe(ctx, char.ID, []string{"BaseHerb"})
	if err != nil {
		t.Fatalf("LearnRecipe failed: %v", err)
	}
	if learned.ID != "rec-1" {
		t.Fatalf("expected rec-1, got %s", learned.ID)
	}

	// Second attempt with "BaseHerb" must return ErrNoRecipesToLearn because rec-2 has item-B as base
	_, err = svc.LearnRecipe(ctx, char.ID, []string{"BaseHerb"})
	if !errors.Is(err, ErrNoRecipesToLearn) {
		t.Fatalf("expected ErrNoRecipesToLearn, got %v", err)
	}
}

func TestLearnRecipe_GodRecipeLeakagePrevented(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchemist")
	charRepo.characters[char.ID] = char

	recipes, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog failed: %v", err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("coreitem.InitialCatalog failed: %v", err)
	}

	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipes, items)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Advanced scroll pool includes "幸せの種" (item-022), but NOT "幸せの帽子" (item-111) or "幸せのくつ" (item-105).
	// recipe-009 (幸せのくつの合成): base is item-111 (幸せの帽子), secondary is item-022 (幸せの種).
	// recipe-011 (幸せの帽子の合成): base is item-105 (幸せのくつ), secondary is item-022 (幸せの種).
	// Both are God-only recipes marked with '#' in legacy CGI.
	// With the base-only restriction, neither recipe must ever be learned via advancedRecipePool.
	advancedRecipePool := []string{
		"世界樹の葉", "魔法の聖水", "幸せの種", "スキルの種", "身代わり石像", "銀のたてごと", "魔法の粉", "悪魔の粉",
		"金の鶏", "モンスター金貨", "金のブレスレット", "怒りのタトゥー", "ソーサリーリング", "ゾンビキラー", "バトルフォーク", "ホーリーランス",
		"モーニングスター", "鋼鉄の剣", "スライムピアス", "理力の杖", "バスタードソード", "堕天使のレイピア", "バトルアックス", "山賊の斧",
		"銀の胸当て", "みかわしの服", "シルバーメイル", "賢者のローブ", "毛皮のマント", "魔人の鎧",
	}

	learnedCount := 0
	for {
		r, err := svc.LearnRecipe(ctx, char.ID, advancedRecipePool)
		if errors.Is(err, ErrNoRecipesToLearn) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.ID == "recipe-009" || r.ID == "recipe-011" {
			t.Fatalf("God recipe leaked into advancedRecipePool: %s (%s)", r.ID, r.Name)
		}
		baseDefID := r.Ingredients[0].DefinitionID
		baseDef, _ := items.FindByID(baseDefID)
		match := false
		for _, b := range advancedRecipePool {
			if b == baseDefID || b == baseDef.Name {
				match = true
				break
			}
		}
		if !match {
			t.Fatalf("learned recipe %s (%s) whose base %s (%s) is not in pool", r.ID, r.Name, baseDefID, baseDef.Name)
		}
		learnedCount++
	}

	if learnedCount == 0 {
		t.Fatalf("expected to learn recipes from advancedRecipePool, got 0")
	}

	// Now use God recipe scroll (empty pool) - recipe-009 and recipe-011 should be learnable!
	godLearned, err := svc.LearnRecipe(ctx, char.ID, nil)
	if err != nil {
		t.Fatalf("God scroll failed to learn remaining recipe: %v", err)
	}
	if godLearned.ID == "" {
		t.Fatalf("expected valid recipe from God scroll")
	}
}

func TestLearnRecipe_BasicRecipePool(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchemist")
	charRepo.characters[char.ID] = char

	recipes, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog failed: %v", err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("coreitem.InitialCatalog failed: %v", err)
	}

	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipes, items)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	basicRecipePool := []string{
		"薬草", "上薬草", "特薬草", "毒消し草", "身代わり人形", "竜のウロコ", "モンスター銅貨", "モンスター銀貨",
		"闇のロザリオ", "魔獣の皮", "祈りの指輪", "金の指輪", "ひのきの棒", "こんぼう", "ブロンズナイフ", "ダガーナイフ",
		"ルーンスタッフ", "ロングスピア", "クサナギの剣", "銅の剣", "聖銀のレイピア", "鎖がま", "鉄の斧", "金の斧",
		"おおきづち", "布の服", "旅人の服", "皮の鎧", "鎖かたびら", "鉄の鎧", "鋼鉄の鎧", "さまよう鎧", "ゾンビメイル", "魔法の法衣",
	}

	learnedCount := 0
	for {
		r, err := svc.LearnRecipe(ctx, char.ID, basicRecipePool)
		if errors.Is(err, ErrNoRecipesToLearn) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		baseDefID := r.Ingredients[0].DefinitionID
		baseDef, _ := items.FindByID(baseDefID)
		match := false
		for _, b := range basicRecipePool {
			if b == baseDefID || b == baseDef.Name {
				match = true
				break
			}
		}
		if !match {
			t.Fatalf("learned recipe %s (%s) whose base %s (%s) is not in basicRecipePool", r.ID, r.Name, baseDefID, baseDef.Name)
		}
		learnedCount++
	}

	if learnedCount == 0 {
		t.Fatalf("expected to learn recipes from basicRecipePool, got 0")
	}
}
