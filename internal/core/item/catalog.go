package item

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrDefinitionNotFound = errors.New("item definition not found")

type DefinitionProvider interface {
	FindByID(id string) (Definition, error)
}

type Catalog struct {
	definitions map[string]Definition
}

func NewCatalog(definitions []Definition) (*Catalog, error) {
	catalog := &Catalog{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		id := strings.TrimSpace(definition.ID)
		name := strings.TrimSpace(definition.Name)
		if id == "" || name == "" || definition.Price < 0 || !IsValidSlot(definition.Slot) {
			return nil, ErrInvalidDefinition
		}
		if _, exists := catalog.definitions[id]; exists {
			return nil, ErrInvalidDefinition
		}
		definition.ID = id
		definition.Name = name
		catalog.definitions[id] = definition
	}
	return catalog, nil
}

func (c *Catalog) FindByID(id string) (Definition, error) {
	if c == nil {
		return Definition{}, ErrDefinitionNotFound
	}
	value, ok := c.definitions[id]
	if !ok {
		return Definition{}, ErrDefinitionNotFound
	}
	return value, nil
}

// Definitions returns all catalog entries in stable ID order.
func (c *Catalog) Definitions() []Definition {
	if c == nil {
		return nil
	}
	values := make([]Definition, 0, len(c.definitions))
	for _, definition := range c.definitions {
		values = append(values, definition)
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].ID < values[j].ID
	})
	return values
}

//go:embed data/weapons.json
var weaponsCatalogData []byte

//go:embed data/armors.json
var armorsCatalogData []byte

//go:embed data/shields.json
var shieldsCatalogData []byte

//go:embed data/accessories.json
var accessoriesCatalogData []byte

//go:embed data/consumables.json
var consumablesCatalogData []byte

func InitialCatalog() (*Catalog, error) {
	var all []Definition
	for _, source := range [][]byte{
		weaponsCatalogData,
		armorsCatalogData,
		shieldsCatalogData,
		accessoriesCatalogData,
		consumablesCatalogData,
	} {
		var data []Definition
		if err := json.Unmarshal(source, &data); err != nil {
			return nil, fmt.Errorf("decode item catalog: %w", err)
		}
		all = append(all, data...)
	}
	return NewCatalog(all)
}
