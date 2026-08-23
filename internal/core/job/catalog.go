package job

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

var ErrDefinitionNotFound = errors.New("job definition not found")

type DefinitionProvider interface {
	FindByID(id string) (Definition, error)
}

type Catalog struct {
	definitions map[string]Definition
}

func NewCatalog(definitions []Definition) (*Catalog, error) {
	catalog := &Catalog{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		if definition.ID == "" || definition.Name == "" || definition.HPGrowth < 0 ||
			definition.MPGrowth < 0 || definition.AttackGrowth < 0 ||
			definition.DefenseGrowth < 0 || definition.AgilityGrowth < 0 ||
			definition.MinLevel < 1 {
			return nil, ErrInvalidDefinition
		}
		if _, exists := catalog.definitions[definition.ID]; exists {
			return nil, ErrInvalidDefinition
		}
		catalog.definitions[definition.ID] = definition
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

//go:embed data/jobs.json
var initialCatalogData []byte

func InitialCatalog() (*Catalog, error) {
	var data []Definition
	if err := json.Unmarshal(initialCatalogData, &data); err != nil {
		return nil, fmt.Errorf("decode job catalog: %w", err)
	}
	return NewCatalog(data)
}
