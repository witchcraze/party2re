package secretshop

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data/secret_items.json
var defaultSecretItemsJSON []byte

// Item represents a rare item available in the secret shop.
type Item struct {
	ID               string `json:"id"`
	ItemDefinitionID string `json:"item_definition_id"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Price            int    `json:"price"`
	Description      string `json:"description"`
}

// Catalog holds all available secret shop items.
type Catalog struct {
	items   []Item
	byID    map[string]Item
	byDefID map[string]Item
}

// LoadDefaultCatalog loads the embedded secret shop items.
func LoadDefaultCatalog() (*Catalog, error) {
	return LoadCatalog(defaultSecretItemsJSON)
}

// LoadCatalog parses JSON data into a secret shop catalog.
func LoadCatalog(data []byte) (*Catalog, error) {
	var items []Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to parse secret shop catalog: %w", err)
	}

	byID := make(map[string]Item, len(items))
	byDefID := make(map[string]Item, len(items))
	for _, item := range items {
		if item.ID == "" || item.ItemDefinitionID == "" || item.Price <= 0 {
			return nil, fmt.Errorf("invalid secret shop item: %+v", item)
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

// Items returns all items in the catalog.
func (c *Catalog) Items() []Item {
	copied := make([]Item, len(c.items))
	copy(copied, c.items)
	return copied
}

// FindByID finds an item by secret item ID.
func (c *Catalog) FindByID(id string) (Item, bool) {
	item, ok := c.byID[id]
	return item, ok
}

// FindByDefinitionID finds an item by core item definition ID.
func (c *Catalog) FindByDefinitionID(defID string) (Item, bool) {
	item, ok := c.byDefID[defID]
	return item, ok
}
