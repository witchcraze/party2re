package home

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrNoRecipesToLearn         = errors.New("no more recipes can be learned from this book")
	ErrRecipeLearnerUnavailable = errors.New("recipe learner service is not available")
)

// LearnedRecipe represents recipe combination discovery metadata.
type LearnedRecipe struct {
	BaseName     string
	MaterialName string
}

// RecipeLearner provides character recipe discovery matching allowed base material pools.
type RecipeLearner interface {
	LearnRecipe(ctx context.Context, characterID string, pool []string) (LearnedRecipe, error)
}

// SetRecipeLearner registers a cross-domain recipe learner service.
func (s *Service) SetRecipeLearner(l RecipeLearner) {
	s.recipeLearner = l
}

// WithRecipeLearner configures a RecipeLearner for the Service.
func WithRecipeLearner(l RecipeLearner) ServiceOption {
	return func(s *Service) {
		s.recipeLearner = l
	}
}

// basicRecipePool defines the 34 legacy base items learnable via 基本錬金レシピ (Item 127).
var basicRecipePool = []string{
	"薬草", "上薬草", "特薬草", "毒消し草", "身代わり人形", "竜のウロコ", "モンスター銅貨", "モンスター銀貨",
	"闇のロザリオ", "魔獣の皮", "祈りの指輪", "金の指輪", "ひのきの棒", "こんぼう", "ブロンズナイフ", "ダガーナイフ",
	"ルーンスタッフ", "ロングスピア", "クサナギの剣", "銅の剣", "聖銀のレイピア", "鎖がま", "鉄の斧", "金の斧",
	"おおきづち", "布の服", "旅人の服", "皮の鎧", "鎖かたびら", "鉄の鎧", "鋼鉄の鎧", "さまよう鎧", "ゾンビメイル", "魔法の法衣",
}

// advancedRecipePool defines the 30 legacy base items learnable via 応用錬金レシピ (Item 128).
var advancedRecipePool = []string{
	"世界樹の葉", "魔法の聖水", "幸せの種", "スキルの種", "身代わり石像", "銀のたてごと", "魔法の粉", "悪魔の粉",
	"金の鶏", "モンスター金貨", "金のブレスレット", "怒りのタトゥー", "ソーサリーリング", "ゾンビキラー", "バトルフォーク", "ホーリーランス",
	"モーニングスター", "鋼鉄の剣", "スライムピアス", "理力の杖", "バスタードソード", "堕天使のレイピア", "バトルアックス", "山賊の斧",
	"銀の胸当て", "みかわしの服", "シルバーメイル", "賢者のローブ", "毛皮のマント", "魔人の鎧",
}

func isRecipeScrollItem(name string) bool {
	switch name {
	case "基本錬金レシピ", "応用錬金レシピ", "神の錬金レシピ":
		return true
	default:
		return false
	}
}

func (s *Service) applyRecipeScrollConsumable(ctx context.Context, char *corecharacter.Character, itemName string) (string, error) {
	if s.recipeLearner == nil {
		return "", ErrRecipeLearnerUnavailable
	}

	var pool []string
	switch itemName {
	case "基本錬金レシピ":
		pool = basicRecipePool
	case "応用錬金レシピ":
		pool = advancedRecipePool
	case "神の錬金レシピ":
		pool = nil
	default:
		return "", fmt.Errorf("%w: %sはここでは使えません", ErrCannotUseHere, itemName)
	}

	res, err := s.recipeLearner.LearnRecipe(ctx, char.ID, pool)
	if err != nil {
		if errors.Is(err, ErrNoRecipesToLearn) {
			return "この錬金レシピからこれ以上習得できる錬金方法はないようだ…", nil
		}
		return "", err
	}

	return fmt.Sprintf("%s は錬金レシピを読んだ！【%s × %s ＝ ？？？】の錬金方法を習得した！", char.Name, res.BaseName, res.MaterialName), nil
}
