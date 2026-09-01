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

//go:embed data/orb_appraisals.json
var rawOrbAppraisalsJSON []byte

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

// OrbAppraisalCandidate represents a single gem outcome and its relative weight within an orb pool.
type OrbAppraisalCandidate struct {
	GemID  string `json:"gem_id"`
	Weight int    `json:"weight"`
}

// OrbAppraisalPool represents the weighted loot pool for an unidentified orb type.
type OrbAppraisalPool struct {
	OrbName    string                  `json:"orb_name"`
	Candidates []OrbAppraisalCandidate `json:"candidates"`
}

// Catalog provides in-memory access to gem definitions, synthesis recipes, and orb appraisal pools.
type Catalog struct {
	gemsByID     map[string]Gem
	gemsByName   map[string]Gem
	allGems      []Gem
	recipesByID  map[string]Recipe
	allRecipes   []Recipe
	recipesByMat map[string]Recipe
	orbPools     map[string][]string
	orbPoolDefs  []OrbAppraisalPool
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

	var orbPools []OrbAppraisalPool
	if err := json.Unmarshal(rawOrbAppraisalsJSON, &orbPools); err != nil {
		return nil, fmt.Errorf("unmarshal orb appraisals catalog: %w", err)
	}

	c := &Catalog{
		gemsByID:     make(map[string]Gem, len(gems)),
		gemsByName:   make(map[string]Gem, len(gems)),
		allGems:      gems,
		recipesByID:  make(map[string]Recipe, len(recipes)),
		allRecipes:   recipes,
		recipesByMat: make(map[string]Recipe, len(recipes)),
		orbPools:     make(map[string][]string, len(orbPools)),
		orbPoolDefs:  orbPools,
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

	for _, p := range orbPools {
		var expanded []string
		for _, cand := range p.Candidates {
			if _, ok := c.gemsByID[cand.GemID]; !ok {
				return nil, fmt.Errorf("invalid gem_id %q in orb %q pool", cand.GemID, p.OrbName)
			}
			weight := cand.Weight
			if weight < 1 {
				weight = 1
			}
			for i := 0; i < weight; i++ {
				expanded = append(expanded, cand.GemID)
			}
		}
		c.orbPools[strings.TrimSpace(p.OrbName)] = expanded
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

// IsUnidentifiedOrb checks if an item name matches any known unidentified orb.
func (c *Catalog) IsUnidentifiedOrb(name string) bool {
	_, ok := c.orbPools[strings.TrimSpace(name)]
	return ok
}

// GetOrbAppraisalPools returns all registered orb appraisal pool definitions.
func (c *Catalog) GetOrbAppraisalPools() []OrbAppraisalPool {
	result := make([]OrbAppraisalPool, len(c.orbPoolDefs))
	copy(result, c.orbPoolDefs)
	return result
}

// GetOrbCandidateGemIDs returns the expanded slice of candidate gem IDs for an orb name.
func (c *Catalog) GetOrbCandidateGemIDs(orbName string) ([]string, bool) {
	pool, ok := c.orbPools[strings.TrimSpace(orbName)]
	if !ok {
		return nil, false
	}
	result := make([]string, len(pool))
	copy(result, pool)
	return result, true
}

// AppraiseUnidentifiedItem maps an unidentified orb name to a revealed Gem drawn from its weighted candidate pool.
func (c *Catalog) AppraiseUnidentifiedItem(name string, random RandomSource) (Gem, bool, error) {
	orbName := strings.TrimSpace(name)
	gemIDs, ok := c.orbPools[orbName]
	if !ok || len(gemIDs) == 0 {
		return Gem{}, false, nil
	}

	if random == nil {
		random = DefaultRandomSource()
	}

	idx, err := random.Intn(len(gemIDs))
	if err != nil {
		return Gem{}, true, fmt.Errorf("generate random index for orb %s: %w", orbName, err)
	}
	if idx < 0 || idx >= len(gemIDs) {
		idx = 0
	}

	gemID := gemIDs[idx]
	gem, ok := c.FindGemByID(gemID)
	if !ok {
		return Gem{}, true, fmt.Errorf("gem %q in orb %q pool not found in catalog", gemID, orbName)
	}

	return gem, true, nil
}
