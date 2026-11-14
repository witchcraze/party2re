package shop_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/shop"
)

func TestAllSynthesisRecipes_UseCleanRoomCatalogNamesAndPreserveSpecs(t *testing.T) {
	cat, err := item.InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog() error = %v", err)
	}

	expectedSpecs := []struct {
		id          string
		productID   string
		mat1ID      string
		mat2ID      string
		successRate int
	}{
		{"recipe-01", "item-152", "item-016", "item-064", 30},
		{"recipe-02", "item-153", "item-017", "item-065", 30},
		{"recipe-03", "item-154", "item-018", "item-061", 30},
		{"recipe-04", "item-155", "item-019", "item-062", 30},
		{"recipe-05", "item-156", "item-020", "item-063", 30},
		{"recipe-06", "item-157", "item-021", "item-060", 15},
		{"recipe-07", "item-167", "item-022", "item-136", 99},
		{"recipe-08", "item-213", "item-041", "item-081", 99},
		{"recipe-09", "item-214", "item-141", "item-131", 75},
		{"recipe-10", "item-159", "item-146", "item-081", 50},
		{"recipe-11", "item-186", "item-146", "item-015", 50},
		{"recipe-12", "item-256", "item-146", "item-011", 50},
		{"recipe-13", "item-162", "item-161", "item-014", 75},
		{"recipe-14", "item-163", "item-161", "item-015", 75},
		{"recipe-15", "item-209", "item-161", "item-185", 75},
		{"recipe-16", "item-200", "item-145", "armor-51", 75},
		{"recipe-17", "item-205", "item-145", "item-115", 99},
		{"recipe-18", "item-212", "item-145", "item-192", 99},
		{"recipe-19", "item-239", "item-147", "item-132", 50},
		{"recipe-20", "item-201", "item-183", "armor-40", 75},
		{"recipe-21", "item-247", "weapon-01", "item-087", 30},
		{"recipe-22", "item-202", "item-198", "item-132", 99},
		{"recipe-23", "item-166", "item-137", "item-103", 99},
		{"recipe-24", "item-240", "item-158", "item-154", 75},
		{"recipe-25", "item-241", "item-158", "item-155", 75},
		{"recipe-26", "item-242", "item-158", "item-156", 75},
		{"recipe-27", "item-168", "item-104", "item-150", 99},
		{"recipe-28", "item-203", "item-071", "item-013", 99},
		{"recipe-29", "item-257", "item-251", "item-004", 30},
		{"recipe-30", "item-257", "item-252", "item-004", 30},
		{"recipe-31", "item-257", "item-253", "item-004", 75},
		{"recipe-32", "item-257", "item-254", "item-004", 75},
		{"recipe-33", "item-257", "item-255", "item-004", 99},
		{"recipe-34", "item-170", "item-169", "item-003", 75},
		{"recipe-35", "item-171", "item-170", "item-004", 50},
		{"recipe-36", "item-172", "item-170", "item-084", 50},
		{"recipe-37", "item-173", "item-170", "item-103", 15},
		{"recipe-38", "item-174", "item-170", "item-066", 25},
		{"recipe-39", "item-176", "item-175", "item-144", 75},
		{"recipe-40", "item-177", "item-175", "item-151", 99},
		{"recipe-41", "item-178", "item-177", "item-144", 75},
		{"recipe-42", "item-179", "item-177", "item-145", 30},
		{"recipe-43", "item-221", "item-172", "item-010", 30},
		{"recipe-44", "item-224", "item-223", "item-187", 50},
		{"recipe-45", "item-227", "item-226", "item-187", 50},
		{"recipe-46", "item-250", "item-196", "item-203", 15},
		{"recipe-47", "item-211", "item-141", "item-136", 15},
		{"recipe-48", "weapon-71", "weapon-40", "item-028", 99},
	}

	distinctLegacyNames := map[string]bool{
		"エクスカリバー":  true,
		"ハグレメタルの剣": true,
		"ハグレメタルの鎧": true,
		"ギガントアーマー": true,
		"ひのきの棒":    true,
	}
	for _, ref := range item.LegacyItemCatalog {
		if ref.LegacyName != "" && ref.LegacyName != ref.CleanRoomName {
			distinctLegacyNames[ref.LegacyName] = true
		}
	}

	recipes := shop.AllSynthesisRecipes()
	if len(recipes) != len(expectedSpecs) {
		t.Fatalf("len(AllSynthesisRecipes()) = %d, want %d", len(recipes), len(expectedSpecs))
	}

	productCounts := make(map[string]int, len(recipes))
	for _, r := range recipes {
		productCounts[r.ProductDefinitionID]++
	}

	seenTargets := make(map[string]bool, len(recipes))
	seenVariants := make(map[string]int)

	for i, r := range recipes {
		spec := expectedSpecs[i]
		if r.ID != spec.id ||
			r.ProductDefinitionID != spec.productID ||
			r.Material1DefinitionID != spec.mat1ID ||
			r.Material2DefinitionID != spec.mat2ID ||
			r.SuccessRate != spec.successRate {
			t.Fatalf("recipe[%d] spec mismatch: got %+v, want %+v", i, r, spec)
		}

		prodDef, err := cat.FindByID(r.ProductDefinitionID)
		if err != nil {
			t.Fatalf("%s product %s lookup: %v", r.ID, r.ProductDefinitionID, err)
		}
		mat1Def, err := cat.FindByID(r.Material1DefinitionID)
		if err != nil {
			t.Fatalf("%s material1 %s lookup: %v", r.ID, r.Material1DefinitionID, err)
		}
		mat2Def, err := cat.FindByID(r.Material2DefinitionID)
		if err != nil {
			t.Fatalf("%s material2 %s lookup: %v", r.ID, r.Material2DefinitionID, err)
		}

		if r.ProductName != prodDef.Name {
			t.Errorf("%s ProductName = %q, want catalog name %q", r.ID, r.ProductName, prodDef.Name)
		}
		if r.Material1Name != mat1Def.Name {
			t.Errorf("%s Material1Name = %q, want catalog name %q", r.ID, r.Material1Name, mat1Def.Name)
		}
		if r.Material2Name != mat2Def.Name {
			t.Errorf("%s Material2Name = %q, want catalog name %q", r.ID, r.Material2Name, mat2Def.Name)
		}

		wantTarget := prodDef.Name
		if productCounts[r.ProductDefinitionID] > 1 {
			seenVariants[r.ProductDefinitionID]++
			wantTarget = prodDef.Name + string(rune('0'+seenVariants[r.ProductDefinitionID]))
		}
		if r.Target != wantTarget {
			t.Errorf("%s Target = %q, want %q", r.ID, r.Target, wantTarget)
		}
		if seenTargets[r.Target] {
			t.Errorf("duplicate recipe target %q", r.Target)
		}
		seenTargets[r.Target] = true

		for _, exposed := range []string{r.Target, r.ProductName, r.Material1Name, r.Material2Name} {
			if distinctLegacyNames[exposed] {
				t.Errorf("%s exposed forbidden legacy name %q", r.ID, exposed)
			}
		}

		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal %s: %v", r.ID, err)
		}
		if strings.Contains(string(raw), "legacy_name") {
			t.Errorf("%s JSON still exposes legacy_name: %s", r.ID, raw)
		}
	}
}

func TestFindSynthesisRecipe_CleanRoomAndIDOnly(t *testing.T) {
	recipes := shop.AllSynthesisRecipes()
	productCounts := make(map[string]int, len(recipes))
	for _, r := range recipes {
		productCounts[r.ProductDefinitionID]++
	}

	for _, r := range recipes {
		gotByID, err := shop.FindSynthesisRecipe(r.ID)
		if err != nil || gotByID.ID != r.ID {
			t.Fatalf("FindSynthesisRecipe(%q) = (%+v, %v), want %s", r.ID, gotByID, err, r.ID)
		}

		gotByTarget, err := shop.FindSynthesisRecipe(r.Target)
		if err != nil || gotByTarget.ID != r.ID {
			t.Fatalf("FindSynthesisRecipe(%q) = (%+v, %v), want %s", r.Target, gotByTarget, err, r.ID)
		}

		if productCounts[r.ProductDefinitionID] == 1 {
			gotByName, err := shop.FindSynthesisRecipe(r.ProductName)
			if err != nil || gotByName.ID != r.ID {
				t.Fatalf("FindSynthesisRecipe(%q) = (%+v, %v), want %s", r.ProductName, gotByName, err, r.ID)
			}
			gotByProdID, err := shop.FindSynthesisRecipe(r.ProductDefinitionID)
			if err != nil || gotByProdID.ID != r.ID {
				t.Fatalf("FindSynthesisRecipe(%q) = (%+v, %v), want %s", r.ProductDefinitionID, gotByProdID, err, r.ID)
			}
		}
	}

	legacyQueries := []string{
		"ピオリムの書",
		"バイキルトの書",
		"メラゾーマの書",
		"ベギラゴンの書",
		"ベホマズンの禁書",
		"モシャスの禁書",
		"エクスカリバー",
		"エスナの書",
		"アダマントの盾",
		"イージスの盾",
		"ベホイミの書",
		"ベホマラーの書",
		"ケアルラの書",
		"ザオラルの書",
		"マホカンタの書",
		"スクルトの書",
		"ピオラの書",
	}
	for _, ref := range item.LegacyItemCatalog {
		if ref.LegacyName != "" && ref.LegacyName != ref.CleanRoomName {
			legacyQueries = append(legacyQueries, ref.LegacyName)
		}
	}

	for _, q := range legacyQueries {
		if got, err := shop.FindSynthesisRecipe(q); !errors.Is(err, shop.ErrRecipeNotFound) {
			t.Errorf("FindSynthesisRecipe(%q) = (%+v, %v), want ErrRecipeNotFound", q, got, err)
		}
	}
}
