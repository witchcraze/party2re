package gemstore

import "fmt"

func (c *Catalog) loadRecipes(recipes []Recipe, items ItemDefinitionProvider) error {
	c.allRecipes = make([]Recipe, 0, len(recipes))
	c.recipesByID = make(map[string]Recipe, len(recipes))
	for _, r := range recipes {
		result, ok := c.FindGemByID(r.ResultID)
		if !ok {
			return fmt.Errorf("recipe %q: invalid result gem ID %q", r.ID, r.ResultID)
		}
		r.ResultName = result.Name
		var names [2]string
		for i, id := range []string{r.Material1, r.Material2} {
			if gem, ok := c.FindGemByID(id); ok {
				names[i] = gem.Name
			} else {
				definition, err := items.FindByID(id)
				if err != nil {
					return fmt.Errorf("recipe %q: invalid material ID %q: %w", r.ID, id, err)
				}
				names[i] = definition.Name
			}
		}
		r.Material1Name, r.Material2Name = names[0], names[1]
		c.allRecipes = append(c.allRecipes, r)
		c.recipesByID[r.ID] = r
	}
	return nil
}
