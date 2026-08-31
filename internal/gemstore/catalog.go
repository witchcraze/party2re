package gemstore

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed data/gems.json
var rawGemsJSON []byte

//go:embed data/recipes.json
var rawRecipesJSON []byte

// Gem represents a gem or jewel orb available in the gem store.
type Gem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Price         int    `json:"price"`
	RequiredLevel int    `json:"required_level"`
	SlotCost      int    `json:"slot_cost"`
	MPCost        int    `json:"mp_cost"`
	Description   string `json:"description"`
}

// Recipe represents a synthesis formula for creating advanced gems.
type Recipe struct {
	ID         string `json:"id"`
	ResultName string `json:"result_name"`
	Material1  string `json:"material_1"`
	Material2  string `json:"material_2"`
}

// Catalog provides in-memory access to gem definitions and synthesis recipes.
type Catalog struct {
	gemsByID     map[string]Gem
	gemsByName   map[string]Gem
	allGems      []Gem
	recipesByID  map[string]Recipe
	allRecipes   []Recipe
	recipesByMat map[string]Recipe
}

// DefaultCatalog parses and returns the embedded gem store catalog.
func DefaultCatalog() (*Catalog, error) {
	var gems []Gem
	if err := json.Unmarshal(rawGemsJSON, &gems); err != nil {
		return nil, fmt.Errorf("unmarshal gems catalog: %w", err)
	}

	var recipes []Recipe
	if err := json.Unmarshal(rawRecipesJSON, &recipes); err != nil {
		return nil, fmt.Errorf("unmarshal recipes catalog: %w", err)
	}

	c := &Catalog{
		gemsByID:     make(map[string]Gem, len(gems)),
		gemsByName:   make(map[string]Gem, len(gems)),
		allGems:      gems,
		recipesByID:  make(map[string]Recipe, len(recipes)),
		allRecipes:   recipes,
		recipesByMat: make(map[string]Recipe, len(recipes)),
	}

	for _, g := range gems {
		c.gemsByID[g.ID] = g
		c.gemsByName[g.Name] = g
	}

	for _, r := range recipes {
		c.recipesByID[r.ID] = r
		key1 := fmt.Sprintf("%s|%s", r.Material1, r.Material2)
		key2 := fmt.Sprintf("%s|%s", r.Material2, r.Material1)
		c.recipesByMat[key1] = r
		c.recipesByMat[key2] = r
	}

	return c, nil
}

// FindGemByID finds a gem by its ID.
func (c *Catalog) FindGemByID(id string) (Gem, bool) {
	g, ok := c.gemsByID[id]
	return g, ok
}

// FindGemByName finds a gem by its display name.
func (c *Catalog) FindGemByName(name string) (Gem, bool) {
	g, ok := c.gemsByName[name]
	return g, ok
}

// AllGems returns all registered gems.
func (c *Catalog) AllGems() []Gem {
	result := make([]Gem, len(c.allGems))
	copy(result, c.allGems)
	return result
}

// GetGemsForLevel returns gems that a character of given level is eligible to purchase.
func (c *Catalog) GetGemsForLevel(level int) []Gem {
	var eligible []Gem
	for _, g := range c.allGems {
		if level >= g.RequiredLevel {
			eligible = append(eligible, g)
		}
	}
	return eligible
}

// FindRecipeByID finds a recipe by its ID.
func (c *Catalog) FindRecipeByID(id string) (Recipe, bool) {
	r, ok := c.recipesByID[id]
	return r, ok
}

// FindRecipeByMaterials finds a recipe matching two material names (order independent).
func (c *Catalog) FindRecipeByMaterials(mat1, mat2 string) (Recipe, bool) {
	key := fmt.Sprintf("%s|%s", strings.TrimSpace(mat1), strings.TrimSpace(mat2))
	r, ok := c.recipesByMat[key]
	return r, ok
}

// AllRecipes returns all registered recipes.
func (c *Catalog) AllRecipes() []Recipe {
	result := make([]Recipe, len(c.allRecipes))
	copy(result, c.allRecipes)
	return result
}

// AppraiseUnidentifiedItem maps an unidentified orb name to a revealed Gem.
func (c *Catalog) AppraiseUnidentifiedItem(name string) (Gem, bool) {
	switch strings.TrimSpace(name) {
	case "光る宝珠":
		g, ok := c.FindGemByID("gem_atk_2")
		return g, ok
	case "ひび割れた宝珠":
		g, ok := c.FindGemByID("gem_guard_1")
		return g, ok
	case "多彩色の宝珠":
		g, ok := c.FindGemByID("gem_all_magic_1")
		return g, ok
	case "黒ずんだ宝珠":
		g, ok := c.FindGemByID("gem_instant_death_1")
		return g, ok
	case "妖しい宝珠":
		g, ok := c.FindGemByID("gem_awakening_1")
		return g, ok
	default:
		return Gem{}, false
	}
}
