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
	ProductName           string `json:"product_name"`
	ProductDefinitionID   string `json:"product_definition_id"`
	Material1Name         string `json:"material1_name"`
	Material1DefinitionID string `json:"material1_definition_id"`
	Material2Name         string `json:"material2_name"`
	Material2DefinitionID string `json:"material2_definition_id"`
	SuccessRate           int    `json:"success_rate"`
}

type recipeRaw struct {
	id           string
	targetSuffix string
	productID    string
	mat1ID       string
	mat2ID       string
	successRate  int
}

var rawRecipes = []recipeRaw{
	{"recipe-01", "", "item-152", "item-016", "item-064", 30},
	{"recipe-02", "", "item-153", "item-017", "item-065", 30},
	{"recipe-03", "", "item-154", "item-018", "item-061", 30},
	{"recipe-04", "", "item-155", "item-019", "item-062", 30},
	{"recipe-05", "", "item-156", "item-020", "item-063", 30},
	{"recipe-06", "", "item-157", "item-021", "item-060", 15},
	{"recipe-07", "", "item-167", "item-022", "item-136", 99},
	{"recipe-08", "", "item-213", "item-041", "item-081", 99},
	{"recipe-09", "", "item-214", "item-141", "item-131", 75},
	{"recipe-10", "", "item-159", "item-146", "item-081", 50},
	{"recipe-11", "", "item-186", "item-146", "item-015", 50},
	{"recipe-12", "", "item-256", "item-146", "item-011", 50},
	{"recipe-13", "", "item-162", "item-161", "item-014", 75},
	{"recipe-14", "", "item-163", "item-161", "item-015", 75},
	{"recipe-15", "", "item-209", "item-161", "item-185", 75},
	{"recipe-16", "", "item-200", "item-145", "armor-51", 75},
	{"recipe-17", "", "item-205", "item-145", "item-115", 99},
	{"recipe-18", "", "item-212", "item-145", "item-192", 99},
	{"recipe-19", "", "item-239", "item-147", "item-132", 50},
	{"recipe-20", "", "item-201", "item-183", "armor-40", 75},
	{"recipe-21", "", "item-247", "weapon-01", "item-087", 30},
	{"recipe-22", "", "item-202", "item-198", "item-132", 99},
	{"recipe-23", "", "item-166", "item-137", "item-103", 99},
	{"recipe-24", "", "item-240", "item-158", "item-154", 75},
	{"recipe-25", "", "item-241", "item-158", "item-155", 75},
	{"recipe-26", "", "item-242", "item-158", "item-156", 75},
	{"recipe-27", "", "item-168", "item-104", "item-150", 99},
	{"recipe-28", "", "item-203", "item-071", "item-013", 99},
	{"recipe-29", "1", "item-257", "item-251", "item-004", 30},
	{"recipe-30", "2", "item-257", "item-252", "item-004", 30},
	{"recipe-31", "3", "item-257", "item-253", "item-004", 75},
	{"recipe-32", "4", "item-257", "item-254", "item-004", 75},
	{"recipe-33", "5", "item-257", "item-255", "item-004", 99},
	{"recipe-34", "", "item-170", "item-169", "item-003", 75},
	{"recipe-35", "", "item-171", "item-170", "item-004", 50},
	{"recipe-36", "", "item-172", "item-170", "item-084", 50},
	{"recipe-37", "", "item-173", "item-170", "item-103", 15},
	{"recipe-38", "", "item-174", "item-170", "item-066", 25},
	{"recipe-39", "", "item-176", "item-175", "item-144", 75},
	{"recipe-40", "", "item-177", "item-175", "item-151", 99},
	{"recipe-41", "", "item-178", "item-177", "item-144", 75},
	{"recipe-42", "", "item-179", "item-177", "item-145", 30},
	{"recipe-43", "", "item-221", "item-172", "item-010", 30},
	{"recipe-44", "", "item-224", "item-223", "item-187", 50},
	{"recipe-45", "", "item-227", "item-226", "item-187", 50},
	{"recipe-46", "", "item-250", "item-196", "item-203", 15},
	{"recipe-47", "", "item-211", "item-141", "item-136", 15},
	{"recipe-48", "", "weapon-71", "weapon-40", "item-028", 99},
}

var (
	allRecipesOnce sync.Once
	cachedRecipes  []SynthesisRecipe
)

func AllSynthesisRecipes() []SynthesisRecipe {
	allRecipesOnce.Do(func() {
		cat, err := item.DefaultCatalog()
		cachedRecipes = make([]SynthesisRecipe, len(rawRecipes))
		for i, r := range rawRecipes {
			prodName := r.productID
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
				Target:                prodName + r.targetSuffix,
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
	// 4. Exact match on product definition ID
	for _, r := range recipes {
		if r.ProductDefinitionID == q {
			return r, nil
		}
	}
	return SynthesisRecipe{}, ErrRecipeNotFound
}
