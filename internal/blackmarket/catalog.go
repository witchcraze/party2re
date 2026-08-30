package blackmarket

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data/blackmarket_items.json
var defaultBlackMarketItemsJSON []byte

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

// Catalog holds all black market item offerings.
type Catalog struct {
	items   []Item
	byID    map[string]Item
	byDefID map[string]Item
}

// LoadDefaultCatalog loads the embedded black market catalog.
func LoadDefaultCatalog() (*Catalog, error) {
	return LoadCatalog(defaultBlackMarketItemsJSON)
}

// LoadCatalog parses JSON data into a black market catalog.
func LoadCatalog(data []byte) (*Catalog, error) {
	var items []Item
	if err := json.Unmarshal(data, &items); err != nil {
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

	return &Catalog{
		items:   items,
		byID:    byID,
		byDefID: byDefID,
	}, nil
}

// Items returns a copy of all items in the catalog.
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
