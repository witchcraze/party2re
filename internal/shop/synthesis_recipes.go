package shop

import (
	"errors"
	"strings"
	"sync"

	"github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrRecipeNotFound    = errors.New("synthesis recipe not found")
	ErrMaterialsNotFound = errors.New("required synthesis materials not found in depot")
)

type SynthesisRecipe struct {
	ID                    string `json:"id"`
	Target                string `json:"target"`
	LegacyName            string `json:"legacy_name"`
	ProductName           string `json:"product_name"`
	ProductDefinitionID   string `json:"product_definition_id"`
	Material1Name         string `json:"material1_name"`
	Material1DefinitionID string `json:"material1_definition_id"`
	Material2Name         string `json:"material2_name"`
	Material2DefinitionID string `json:"material2_definition_id"`
	SuccessRate           int    `json:"success_rate"`
}

type recipeRaw struct {
	id          string
	target      string
	legacyName  string
	productID   string
	mat1ID      string
	mat2ID      string
	successRate int
}

var rawRecipes = []recipeRaw{
	{"recipe-01", "命の宝珠", "命の宝珠", "item-152", "item-016", "item-064", 30},
	{"recipe-02", "不思議な宝珠", "不思議な宝珠", "item-153", "item-017", "item-065", 30},
	{"recipe-03", "力の宝珠", "力の宝珠", "item-154", "item-018", "item-061", 30},
	{"recipe-04", "守りの宝珠", "守りの宝珠", "item-155", "item-019", "item-062", 30},
	{"recipe-05", "素早さの宝珠", "素早さの宝珠", "item-156", "item-020", "item-063", 30},
	{"recipe-06", "スキルの宝珠", "スキルの宝珠", "item-157", "item-021", "item-060", 15},
	{"recipe-07", "幸せのブローチ", "幸せのブローチ", "item-167", "item-022", "item-136", 99},
	{"recipe-08", "捕縛ネット", "捕縛ネット", "item-213", "item-041", "item-081", 99},
	{"recipe-09", "見聞絵巻", "見聞絵巻", "item-214", "item-141", "item-131", 75},
	{"recipe-10", "リインストーン", "リインストーン", "item-159", "item-146", "item-081", 50},
	{"recipe-11", "リフライトストーン", "リフライトストーン", "item-186", "item-146", "item-015", 50},
	{"recipe-12", "ディフラストーン", "ディフラストーン", "item-256", "item-146", "item-011", 50},
	{"recipe-13", "アダマントの盾", "アダマントの盾", "item-162", "item-161", "item-014", 75},
	{"recipe-14", "水晶の盾", "水晶の盾", "item-163", "item-161", "item-015", 75},
	{"recipe-15", "イージスの盾", "イージスの盾", "item-209", "item-161", "item-185", 75},
	{"recipe-16", "鬼神の御面", "鬼神の御面", "item-200", "item-145", "armor-51", 75},
	{"recipe-17", "暴走の護符", "暴走の護符", "item-205", "item-145", "item-115", 99},
	{"recipe-18", "仇討の護符", "仇討の護符", "item-212", "item-145", "item-192", 99},
	{"recipe-19", "連撃の脇差", "連撃の脇差", "item-239", "item-147", "item-132", 50},
	{"recipe-20", "ジェットブースター", "ジェットブースター", "item-201", "item-183", "armor-40", 75},
	{"recipe-21", "神木の御香", "神木の御香", "item-247", "weapon-01", "item-087", 30},
	{"recipe-22", "聖銀の足枷", "聖銀の足枷", "item-202", "item-198", "item-132", 99},
	{"recipe-23", "神具の霊鞘", "神具の霊鞘", "item-166", "item-137", "item-103", 99},
	{"recipe-24", "覚醒の紅玉", "覚醒の紅玉", "item-240", "item-158", "item-154", 75},
	{"recipe-25", "覚醒の蒼玉", "覚醒の蒼玉", "item-241", "item-158", "item-155", 75},
	{"recipe-26", "覚醒の翠玉", "覚醒の翠玉", "item-242", "item-158", "item-156", 75},
	{"recipe-27", "記憶のカケラ", "記憶のカケラ", "item-168", "item-104", "item-150", 99},
	{"recipe-28", "聖なるマナ", "聖なるマナ", "item-203", "item-071", "item-013", 99},
	{"recipe-29", "水晶の原石1", "水晶の原石", "item-257", "item-251", "item-004", 30},
	{"recipe-30", "水晶の原石2", "水晶の原石", "item-257", "item-252", "item-004", 30},
	{"recipe-31", "水晶の原石3", "水晶の原石", "item-257", "item-253", "item-004", 75},
	{"recipe-32", "水晶の原石4", "水晶の原石", "item-257", "item-254", "item-004", 75},
	{"recipe-33", "水晶の原石5", "水晶の原石", "item-257", "item-255", "item-004", 99},
	{"recipe-34", "ベホイミの書", "ベホイミの書", "item-170", "item-169", "item-003", 75},
	{"recipe-35", "ベホマラーの書", "ベホマラーの書", "item-171", "item-170", "item-004", 50},
	{"recipe-36", "ケアルラの書", "ケアルラの書", "item-172", "item-170", "item-084", 50},
	{"recipe-37", "ザオラルの書", "ザオラルの書", "item-173", "item-170", "item-103", 15},
	{"recipe-38", "マホカンタの書", "マホカンタの書", "item-174", "item-170", "item-066", 25},
	{"recipe-39", "スクルトの書", "スクルトの書", "item-176", "item-175", "item-144", 75},
	{"recipe-40", "ピオラの書", "ピオラの書", "item-177", "item-175", "item-151", 99},
	{"recipe-41", "ピオリムの書", "ピオリムの書", "item-178", "item-177", "item-144", 75},
	{"recipe-42", "バイキルトの書", "バイキルトの書", "item-179", "item-177", "item-145", 30},
	{"recipe-43", "エスナの書", "エスナの書", "item-221", "item-172", "item-010", 30},
	{"recipe-44", "メラゾーマの書", "メラゾーマの書", "item-224", "item-223", "item-187", 50},
	{"recipe-45", "ベギラゴンの書", "ベギラゴンの書", "item-227", "item-226", "item-187", 50},
	{"recipe-46", "ベホマズンの禁書", "ベホマズンの禁書", "item-250", "item-196", "item-203", 15},
	{"recipe-47", "モシャスの禁書", "モシャスの禁書", "item-211", "item-141", "item-136", 15},
	{"recipe-48", "エクスカリバー", "エクスカリバー", "weapon-71", "weapon-40", "item-028", 99},
}

var (
	allRecipesOnce sync.Once
	cachedRecipes  []SynthesisRecipe
)

func AllSynthesisRecipes() []SynthesisRecipe {
	allRecipesOnce.Do(func() {
		cat, err := item.InitialCatalog()
		cachedRecipes = make([]SynthesisRecipe, len(rawRecipes))
		for i, r := range rawRecipes {
			prodName := r.legacyName
			mat1Name := r.mat1ID
			mat2Name := r.mat2ID
			if err == nil {
				if d, err := cat.FindByID(r.productID); err == nil {
					prodName = d.Name
				}
				if d, err := cat.FindByID(r.mat1ID); err == nil {
					mat1Name = d.Name
				}
				if d, err := cat.FindByID(r.mat2ID); err == nil {
					mat2Name = d.Name
				}
			}
			cachedRecipes[i] = SynthesisRecipe{
				ID:                    r.id,
				Target:                r.target,
				LegacyName:            r.legacyName,
				ProductName:           prodName,
				ProductDefinitionID:   r.productID,
				Material1Name:         mat1Name,
				Material1DefinitionID: r.mat1ID,
				Material2Name:         mat2Name,
				Material2DefinitionID: r.mat2ID,
				SuccessRate:           r.successRate,
			}
		}
	})
	res := make([]SynthesisRecipe, len(cachedRecipes))
	copy(res, cachedRecipes)
	return res
}

func FindSynthesisRecipe(query string) (SynthesisRecipe, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return SynthesisRecipe{}, ErrRecipeNotFound
	}
	recipes := AllSynthesisRecipes()
	// 1. Exact match on target
	for _, r := range recipes {
		if r.Target == q {
			return r, nil
		}
	}
	// 2. Exact match on ID
	for _, r := range recipes {
		if r.ID == q {
			return r, nil
		}
	}
	// 3. Exact match on clean room product name
	for _, r := range recipes {
		if r.ProductName == q {
			return r, nil
		}
	}
	// 4. Exact match on legacy name
	for _, r := range recipes {
		if r.LegacyName == q {
			return r, nil
		}
	}
	// 5. Exact match on product definition ID
	for _, r := range recipes {
		if r.ProductDefinitionID == q {
			return r, nil
		}
	}
	return SynthesisRecipe{}, ErrRecipeNotFound
}
