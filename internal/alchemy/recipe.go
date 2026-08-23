package alchemy

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

//go:embed data/recipes.json
var recipesData []byte

var (
	ErrInvalidRecipe   = errors.New("invalid recipe definition")
	ErrRecipeNotFound  = errors.New("recipe not found")
)

type Ingredient struct {
	DefinitionID string `json:"definition_id"`
	Quantity     int    `json:"quantity"`
}

type Recipe struct {
	ID                     string       `json:"id"`
	Name                   string       `json:"name"`
	ResultItemDefinitionID string       `json:"result_item_definition_id"`
	ResultQuantity         int          `json:"result_quantity"`
	Ingredients            []Ingredient `json:"ingredients"`
	GoldFee                int          `json:"gold_fee"`
}

func NewRecipe(
	id string,
	name string,
	resultItemID string,
	resultQuantity int,
	ingredients []Ingredient,
	goldFee int,
) (Recipe, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(resultItemID) == "" {
		return Recipe{}, ErrInvalidRecipe
	}
	if resultQuantity <= 0 || goldFee < 0 || len(ingredients) == 0 {
		return Recipe{}, ErrInvalidRecipe
	}
	for _, ing := range ingredients {
		if strings.TrimSpace(ing.DefinitionID) == "" || ing.Quantity <= 0 {
			return Recipe{}, ErrInvalidRecipe
		}
	}
	return Recipe{
		ID:                     strings.TrimSpace(id),
		Name:                   strings.TrimSpace(name),
		ResultItemDefinitionID: strings.TrimSpace(resultItemID),
		ResultQuantity:         resultQuantity,
		Ingredients:            ingredients,
		GoldFee:                goldFee,
	}, nil
}

type RecipeCatalog struct {
	recipes map[string]Recipe
}

func NewRecipeCatalog(recipes []Recipe) (*RecipeCatalog, error) {
	catalog := &RecipeCatalog{recipes: make(map[string]Recipe, len(recipes))}
	for _, r := range recipes {
		validated, err := NewRecipe(r.ID, r.Name, r.ResultItemDefinitionID, r.ResultQuantity, r.Ingredients, r.GoldFee)
		if err != nil {
			return nil, err
		}
		if _, exists := catalog.recipes[validated.ID]; exists {
			return nil, ErrInvalidRecipe
		}
		catalog.recipes[validated.ID] = validated
	}
	return catalog, nil
}

func (c *RecipeCatalog) FindByID(id string) (Recipe, error) {
	if c == nil {
		return Recipe{}, ErrRecipeNotFound
	}
	r, ok := c.recipes[id]
	if !ok {
		return Recipe{}, ErrRecipeNotFound
	}
	return r, nil
}

func (c *RecipeCatalog) All() []Recipe {
	if c == nil {
		return nil
	}
	list := make([]Recipe, 0, len(c.recipes))
	for _, r := range c.recipes {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

func InitialRecipeCatalog() (*RecipeCatalog, error) {
	var data []Recipe
	if err := json.Unmarshal(recipesData, &data); err != nil {
		return nil, fmt.Errorf("decode recipes: %w", err)
	}
	return NewRecipeCatalog(data)
}
