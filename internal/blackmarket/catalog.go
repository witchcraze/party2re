package blackmarket

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data/blackmarket_items.json
var defaultBlackMarketItemsJSON []byte

//go:embed data/blackmarket_prizes.json
var defaultBlackMarketPrizesJSON []byte

//go:embed data/blackmarket_sacrifices.json
var defaultBlackMarketSacrificesJSON []byte

// Item represents a contraband or special goods item available in the black market.
type Item struct {
	ID               string `json:"id"`
	ItemDefinitionID string `json:"item_definition_id"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	BasePrice        int    `json:"base_price"`
	DailyLimit       int    `json:"daily_limit"`
	Description      string `json:"description"`
}

// Prize represents an item obtainable by trading Rare Points or U-Rare Points.
type Prize struct {
	ID               string `json:"id"`
	ItemDefinitionID string `json:"item_definition_id"`
	Name             string `json:"name"`
	Cost             int    `json:"cost"`
	IsURare          bool   `json:"is_u_rare"`
	Description      string `json:"description"`
}

// SacrificeYield defines the point reward for sacrificing a rare item.
type SacrificeYield struct {
	ItemDefinitionID string `json:"item_definition_id"`
	RarePoints       int    `json:"rare_points"`
	URarePoints      int    `json:"u_rare_points"`
}

// Catalog holds all black market item offerings, prizes, and sacrifice definitions.
type Catalog struct {
	items             []Item
	byID              map[string]Item
	byDefID           map[string]Item
	prizes            []Prize
	prizesByID        map[string]Prize
	regularPrizes     []Prize
	uPrizes           []Prize
	sacrificesByDefID map[string]SacrificeYield
}

// LoadDefaultCatalog loads the embedded black market catalog, prizes, and sacrifices.
func LoadDefaultCatalog() (*Catalog, error) {
	return LoadAllCatalogs(defaultBlackMarketItemsJSON, defaultBlackMarketPrizesJSON, defaultBlackMarketSacrificesJSON)
}

// LoadCatalog parses JSON data into a black market contraband items catalog.
func LoadCatalog(itemsData []byte) (*Catalog, error) {
	return LoadAllCatalogs(itemsData, defaultBlackMarketPrizesJSON, defaultBlackMarketSacrificesJSON)
}

// LoadAllCatalogs parses items, prizes, and sacrifice data into a complete Catalog.
func LoadAllCatalogs(itemsData, prizesData, sacrificesData []byte) (*Catalog, error) {
	var items []Item
	if err := json.Unmarshal(itemsData, &items); err != nil {
		return nil, fmt.Errorf("failed to parse black market catalog: %w", err)
	}

	byID := make(map[string]Item, len(items))
	byDefID := make(map[string]Item, len(items))
	for _, item := range items {
		if item.ID == "" || item.ItemDefinitionID == "" || item.BasePrice <= 0 || item.DailyLimit <= 0 {
			return nil, fmt.Errorf("invalid black market item: %+v", item)
		}
		byID[item.ID] = item
		byDefID[item.ItemDefinitionID] = item
	}

	var prizes []Prize
	if err := json.Unmarshal(prizesData, &prizes); err != nil {
		return nil, fmt.Errorf("failed to parse black market prizes: %w", err)
	}

	prizesByID := make(map[string]Prize, len(prizes))
	var regularPrizes []Prize
	var uPrizes []Prize
	for _, p := range prizes {
		if p.ID == "" || p.ItemDefinitionID == "" || p.Cost <= 0 {
			return nil, fmt.Errorf("invalid black market prize: %+v", p)
		}
		prizesByID[p.ID] = p
		if p.IsURare {
			uPrizes = append(uPrizes, p)
		} else {
			regularPrizes = append(regularPrizes, p)
		}
	}

	var sacrifices []SacrificeYield
	if err := json.Unmarshal(sacrificesData, &sacrifices); err != nil {
		return nil, fmt.Errorf("failed to parse black market sacrifices: %w", err)
	}

	sacrificesByDefID := make(map[string]SacrificeYield, len(sacrifices))
	for _, s := range sacrifices {
		if s.ItemDefinitionID == "" || (s.RarePoints <= 0 && s.URarePoints <= 0) {
			return nil, fmt.Errorf("invalid black market sacrifice yield: %+v", s)
		}
		sacrificesByDefID[s.ItemDefinitionID] = s
	}

	return &Catalog{
		items:             items,
		byID:              byID,
		byDefID:           byDefID,
		prizes:            prizes,
		prizesByID:        prizesByID,
		regularPrizes:     regularPrizes,
		uPrizes:           uPrizes,
		sacrificesByDefID: sacrificesByDefID,
	}, nil
}

// Items returns a copy of all contraband items in the catalog.
func (c *Catalog) Items() []Item {
	copied := make([]Item, len(c.items))
	copy(copied, c.items)
	return copied
}

// FindByID finds an item by black market item ID.
func (c *Catalog) FindByID(id string) (Item, bool) {
	item, ok := c.byID[id]
	return item, ok
}

// FindByDefinitionID finds an item by core item definition ID.
func (c *Catalog) FindByDefinitionID(defID string) (Item, bool) {
	item, ok := c.byDefID[defID]
	return item, ok
}

// Prizes returns all prize offerings.
func (c *Catalog) Prizes() []Prize {
	copied := make([]Prize, len(c.prizes))
	copy(copied, c.prizes)
	return copied
}

// RegularPrizes returns regular rare point prize offerings.
func (c *Catalog) RegularPrizes() []Prize {
	copied := make([]Prize, len(c.regularPrizes))
	copy(copied, c.regularPrizes)
	return copied
}

// UPrizes returns u-rare point prize offerings.
func (c *Catalog) UPrizes() []Prize {
	copied := make([]Prize, len(c.uPrizes))
	copy(copied, c.uPrizes)
	return copied
}

// FindPrizeByID finds a prize by prize ID.
func (c *Catalog) FindPrizeByID(id string) (Prize, bool) {
	p, ok := c.prizesByID[id]
	return p, ok
}

// GetSacrificeYield returns the point yield for a given item definition ID.
func (c *Catalog) GetSacrificeYield(defID string) (SacrificeYield, bool) {
	y, ok := c.sacrificesByDefID[defID]
	return y, ok
}
