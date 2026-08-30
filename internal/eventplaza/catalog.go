package eventplaza

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data/bazaar.json
var defaultBazaarData []byte

// LoadDefaultBazaarCatalog loads the default embedded traveling merchant bazaar catalog.
func LoadDefaultBazaarCatalog() ([]BazaarItem, error) {
	var items []BazaarItem
	if err := json.Unmarshal(defaultBazaarData, &items); err != nil {
		return nil, fmt.Errorf("failed to parse bazaar catalog: %w", err)
	}
	return items, nil
}
