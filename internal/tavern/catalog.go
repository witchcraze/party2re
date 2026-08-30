package tavern

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data/menu.json
var defaultMenuJSON []byte

// MenuItem represents an available food or drink at the tavern.
type MenuItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Price       int    `json:"price"`
	HPHeal      int    `json:"hp_heal"`
	MPHeal      int    `json:"mp_heal"`
	Tickets     int    `json:"tickets"`
	Description string `json:"description"`
}

// Catalog holds the list of menu items available at the tavern.
type Catalog struct {
	items   []MenuItem
	itemMap map[string]MenuItem
}

// NewCatalog creates a new Catalog from the given menu items.
func NewCatalog(items []MenuItem) (*Catalog, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("tavern catalog must not be empty")
	}

	itemMap := make(map[string]MenuItem, len(items))
	for _, item := range items {
		if item.ID == "" {
			return nil, fmt.Errorf("menu item ID cannot be empty")
		}
		if item.Price < 0 {
			return nil, fmt.Errorf("menu item price cannot be negative: %s", item.ID)
		}
		if _, exists := itemMap[item.ID]; exists {
			return nil, fmt.Errorf("duplicate menu item ID: %s", item.ID)
		}
		itemMap[item.ID] = item
	}

	return &Catalog{
		items:   items,
		itemMap: itemMap,
	}, nil
}

// LoadDefaultCatalog loads the embedded tavern menu.
func LoadDefaultCatalog() (*Catalog, error) {
	var items []MenuItem
	if err := json.Unmarshal(defaultMenuJSON, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embedded tavern menu: %w", err)
	}
	return NewCatalog(items)
}

// Items returns a copy of all menu items.
func (c *Catalog) Items() []MenuItem {
	result := make([]MenuItem, len(c.items))
	copy(result, c.items)
	return result
}

// GetItem looks up a menu item by ID.
func (c *Catalog) GetItem(id string) (MenuItem, bool) {
	item, ok := c.itemMap[id]
	return item, ok
}
