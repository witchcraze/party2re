package gemstore

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

//go:embed data/gems.json
var rawGemsJSON []byte

//go:embed data/sales.json
var rawSalesJSON []byte

//go:embed data/recipes.json
var rawRecipesJSON []byte

//go:embed data/orb_appraisals.json
var rawOrbAppraisalsJSON []byte

// Gem represents a gem or jewel orb available in the gem store.
type Gem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Price       int    `json:"price"`
	SlotCost    int    `json:"slot_cost"`
	MPCost      int    `json:"mp_cost"`
	Description string `json:"description"`
}

// salesTier lists the gems on sale from MinJobLevel (transfer count) up to the next tier.
type salesTier struct {
	MinJobLevel int      `json:"min_job_level"`
	GemIDs      []string `json:"gem_ids"`
}

// Recipe represents a synthesis formula for creating advanced gems.
type Recipe struct {
	ID            string `json:"id"`
	ResultID      string `json:"result_id"`
	ResultName    string `json:"result_name"`
	Material1     string `json:"material_1"`
	Material1Name string `json:"material_1_name"`
	Material2     string `json:"material_2"`
	Material2Name string `json:"material_2_name"`
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
	gemsByID    map[string]Gem
	gemsByName  map[string]Gem
	allGems     []Gem
	gemRanks    map[string]int
	recipesByID map[string]Recipe
	allRecipes  []Recipe
	orbPools    map[string][]string
	orbPoolDefs []OrbAppraisalPool
	salesTiers  []salesTier // descending MinJobLevel; the last tier starts at 0
	salesByTier []map[string]bool
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

	var tiers []salesTier
	if err := json.Unmarshal(rawSalesJSON, &tiers); err != nil {
		return nil, fmt.Errorf("unmarshal gem sales tiers: %w", err)
	}

	var orbPools []OrbAppraisalPool
	if err := json.Unmarshal(rawOrbAppraisalsJSON, &orbPools); err != nil {
		return nil, fmt.Errorf("unmarshal orb appraisals catalog: %w", err)
	}

	c := &Catalog{
		gemsByID:    make(map[string]Gem, len(gems)),
		gemsByName:  make(map[string]Gem, len(gems)),
		allGems:     gems,
		gemRanks:    make(map[string]int, len(gems)),
		orbPools:    make(map[string][]string, len(orbPools)),
		orbPoolDefs: orbPools,
	}

	for i, g := range gems {
		c.gemsByID[g.ID] = g
		c.gemsByName[g.Name] = g
		c.gemRanks[g.ID] = i
		c.gemRanks[g.Name] = i
	}

	for i, t := range tiers {
		if (i > 0 && t.MinJobLevel >= tiers[i-1].MinJobLevel) || (i == len(tiers)-1 && t.MinJobLevel != 0) {
			return nil, fmt.Errorf("gem sales tiers must descend to min_job_level 0")
		}
		set := make(map[string]bool, len(t.GemIDs))
		for _, id := range t.GemIDs {
			if _, ok := c.gemsByID[id]; !ok {
				return nil, fmt.Errorf("invalid gem_id %q in sales tier %d", id, t.MinJobLevel)
			}
			set[id] = true
		}
		c.salesByTier = append(c.salesByTier, set)
	}
	c.salesTiers = tiers

	items, err := coreitem.DefaultCatalog()
	if err != nil {
		return nil, fmt.Errorf("load recipe item definitions: %w", err)
	}
	if err := c.loadRecipes(recipes, items); err != nil {
		return nil, err
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

// GemRank returns the catalog index of the gem for sorting (lower is earlier).
func (c *Catalog) GemRank(idOrName string) int {
	if rank, ok := c.gemRanks[strings.TrimSpace(idOrName)]; ok {
		return rank
	}
	return 9999
}

// GetGemsForJobLevel returns the gems on sale at the given job_lv (transfer count), in catalog order.
func (c *Catalog) GetGemsForJobLevel(jobLevel int) []Gem {
	var eligible []Gem
	for _, g := range c.allGems {
		if c.IsOnSale(g.ID, jobLevel) {
			eligible = append(eligible, g)
		}
	}
	return eligible
}

// IsOnSale reports whether the gem is in the sales list of the tier for jobLevel.
// Tiers are explicit lists, not thresholds: a higher tier may drop a gem a lower tier sells.
func (c *Catalog) IsOnSale(gemID string, jobLevel int) bool {
	for i, t := range c.salesTiers {
		if jobLevel >= t.MinJobLevel {
			return c.salesByTier[i][gemID]
		}
	}
	return false
}

// FindRecipeByID finds a recipe by its ID.
func (c *Catalog) FindRecipeByID(id string) (Recipe, bool) {
	r, ok := c.recipesByID[id]
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
