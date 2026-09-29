package home

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
)

var ErrNoRecipesToLearn = errors.New("no more recipes can be learned from this book")

var basicRecipeBases = []string{
	"薬草", "上薬草", "特薬草", "毒消し草", "身代わり人形", "竜のウロコ", "モンスター銅貨", "モンスター銀貨",
	"闇のロザリオ", "魔獣の皮", "祈りの指輪", "金の指輪", "ひのきの棒", "こんぼう", "ブロンズナイフ", "ダガーナイフ",
	"ルーンスタッフ", "ロングスピア", "クサナギの剣", "銅の剣", "聖銀のレイピア", "鎖がま", "鉄の斧", "金の斧",
	"おおきづち", "布の服", "旅人の服", "皮の鎧", "鎖かたびら", "鉄の鎧", "鋼鉄の鎧", "さまよう鎧", "ゾンビメイル", "魔法の法衣",
}

var appliedRecipeBases = []string{
	"世界樹の葉", "魔法の聖水", "幸せの種", "スキルの種", "身代わり石像", "銀のたてごと", "魔法の粉", "悪魔の粉",
	"金の鶏", "モンスター金貨", "金のブレスレット", "怒りのタトゥー", "ソーサリーリング", "ゾンビキラー", "バトルフォーク",
	"ホーリーランス", "モーニングスター", "鋼鉄の剣", "スライムピアス", "理力の杖", "バスタードソード", "堕天使のレイピア",
	"バトルアックス", "山賊の斧", "銀の胸当て", "みかわしの服", "シルバーメイル", "賢者のローブ", "毛皮のマント", "魔人の鎧",
}

type LearnedRecipeInfo struct {
	ID                     string
	Name                   string
	ResultItemDefinitionID string
	Ingredient1Name        string
	Ingredient2Name        string
}

type RecipeLearner interface {
	LearnRecipe(ctx context.Context, characterID string, allowedBases []string) (LearnedRecipeInfo, error)
}

func (s *Service) SetRecipeLearner(learner RecipeLearner) {
	s.recipeLearner = learner
}

func (s *Service) applyRecipeScrollConsumable(ctx context.Context, char *corecharacter.Character, def item.Definition) (string, bool, error) {
	var allowedBases []string
	switch def.ID {
	case "item-127":
		allowedBases = basicRecipeBases
	case "item-128":
		allowedBases = appliedRecipeBases
	case "item-129":
		allowedBases = nil
	default:
		switch def.Name {
		case "基本錬金レシピ":
			allowedBases = basicRecipeBases
		case "応用錬金レシピ":
			allowedBases = appliedRecipeBases
		case "神の錬金レシピ":
			allowedBases = nil
		default:
			return "", false, nil
		}
	}

	if s.recipeLearner == nil {
		return "", true, errors.New("recipe learner not configured")
	}

	info, err := s.recipeLearner.LearnRecipe(ctx, char.ID, allowedBases)
	if err != nil {
		if errors.Is(err, ErrNoRecipesToLearn) {
			return "この錬金レシピからこれ以上習得できる錬金方法はないようだ…", true, nil
		}
		return "", true, err
	}

	ing1 := info.Ingredient1Name
	if ing1 == "" {
		ing1 = "？？？"
	}
	ing2 := info.Ingredient2Name
	if ing2 == "" {
		ing2 = ing1
	}

	msg := fmt.Sprintf("%s は錬金レシピを読んだ！【%s × %s ＝ ？？？】の錬金方法を習得した！", char.Name, ing1, ing2)
	return msg, true, nil
}
