package item

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var ErrDefinitionNotFound = errors.New("item definition not found")

// DefinitionProvider defines an interface for retrieving item definitions by ID.
type DefinitionProvider interface {
	FindByID(id string) (Definition, error)
}

// ItemDefinitionProvider is an alias for DefinitionProvider for packages preferring explicit naming.
type ItemDefinitionProvider = DefinitionProvider

type Catalog struct {
	definitions map[string]Definition
}

func NewCatalog(definitions []Definition) (*Catalog, error) {
	catalog := &Catalog{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		id := strings.TrimSpace(definition.ID)
		name := strings.TrimSpace(definition.Name)
		if id == "" || name == "" || definition.Price < 0 || !IsValidSlot(definition.Slot) || !IsValidUsageCategory(definition.UsageCategory) {
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

func (c *Catalog) FindByName(name string) (Definition, error) {
	if c == nil {
		return Definition{}, ErrDefinitionNotFound
	}
	cleanName := strings.TrimSpace(name)
	for _, definition := range c.definitions {
		if definition.Name == cleanName {
			return definition, nil
		}
	}
	return Definition{}, ErrDefinitionNotFound
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

var (
	defaultCatalogOnce sync.Once
	defaultCatalog     *Catalog
	defaultCatalogErr  error
)

// DefaultCatalog returns the shared default catalog containing all standard game item definitions.
func DefaultCatalog() (*Catalog, error) {
	defaultCatalogOnce.Do(func() {
		defaultCatalog, defaultCatalogErr = InitialCatalog()
	})
	return defaultCatalog, defaultCatalogErr
}

// IsStackableID returns whether the given item definition ID is stackable according to the default catalog.
// If the item definition is not found in the default catalog, it checks whether the ID indicates equipment.
func IsStackableID(definitionID string) bool {
	cat, err := DefaultCatalog()
	if err == nil {
		if def, err := cat.FindByID(definitionID); err == nil {
			return def.IsStackable()
		}
	}
	lower := strings.ToLower(definitionID)
	for _, prefix := range []string{"weapon", "armor", "shield", "acc", "wea", "arm", "shi", "sword", "axe", "bow", "wand", "staff", "helm", "plate", "robe"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return true
}
