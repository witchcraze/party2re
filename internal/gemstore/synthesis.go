package gemstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// SynthesizeGem synthesizes two ingredient items/gems into an advanced gem stored in the Gem Box.
func (s *Service) SynthesizeGem(ctx context.Context, characterID, recipeID string) (SynthesizeResult, error) {
	characterID = strings.TrimSpace(characterID)
	recipeID = strings.TrimSpace(recipeID)
	if characterID == "" {
		return SynthesizeResult{}, ErrInvalidCharacterID
	}
	if recipeID == "" {
		return SynthesizeResult{}, ErrInvalidRecipeID
	}

	recipe, ok := s.catalog.FindRecipeByID(recipeID)
	if !ok {
		return SynthesizeResult{}, ErrRecipeNotFound
	}

	resultGem, ok := s.catalog.FindGemByName(recipe.ResultName)
	if !ok {
		resultGem, ok = s.catalog.FindGemByID(recipe.ResultName)
		if !ok {
			return SynthesizeResult{}, ErrGemNotFound
		}
	}

	var res SynthesizeResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		var dep depot.Depot
		if s.depots != nil {
			d, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
			if err != nil && !errors.Is(err, depot.ErrNotFound) {
				return err
			}
			if err == nil {
				dep = d
			}
		}

		box, err := s.getOrCreateGemBoxForUpdate(txCtx, char)
		if err != nil {
			return err
		}

		// Consume material 1 (try GemBox first, then inventory, then depot)
		if _, err := removeMaterialFromGemBox(&box, recipe.Material1, s.catalog, s.items); err != nil {
			mat1Query := depot.QueryByMatch(func(inst coreitem.Instance) bool {
				name := resolveItemName(inst.DefinitionID, s.catalog, s.items)
				return name == recipe.Material1 || inst.DefinitionID == recipe.Material1
			})
			if _, err := depot.ConsumeItem(&inv, &dep, mat1Query, depot.PriorityInventoryFirst, 1); err != nil {
				return fmt.Errorf("%w: missing %s", ErrInsufficientMaterials, recipe.Material1)
			}
		}

		// Consume material 2 (try GemBox first, then inventory, then depot)
		if _, err := removeMaterialFromGemBox(&box, recipe.Material2, s.catalog, s.items); err != nil {
			mat2Query := depot.QueryByMatch(func(inst coreitem.Instance) bool {
				name := resolveItemName(inst.DefinitionID, s.catalog, s.items)
				return name == recipe.Material2 || inst.DefinitionID == recipe.Material2
			})
			if _, err := depot.ConsumeItem(&inv, &dep, mat2Query, depot.PriorityInventoryFirst, 1); err != nil {
				return fmt.Errorf("%w: missing %s", ErrInsufficientMaterials, recipe.Material2)
			}
		}

		newInstance, err := coreitem.NewInstance(resultGem.ID, 1)
		if err != nil {
			return err
		}
		box.Deliver(newInstance) // gem_store.cgi:309 send_gem ignores capacity

		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}
		if s.depots != nil && dep.CharacterID != "" {
			if err := s.depots.Save(txCtx, dep); err != nil {
				return err
			}
		}
		if err := s.gemBoxes.Save(txCtx, box); err != nil {
			return err
		}

		res = SynthesizeResult{
			Character:    char,
			GemBox:       box,
			Inventory:    inv,
			CreatedGem:   resultGem,
			Recipe:       recipe,
			ItemInstance: newInstance,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return SynthesizeResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return SynthesizeResult{}, err
		}
	}

	return res, nil
}

// AppraiseItem appraises a Depot item: an unidentified orb is consumed and its gem enters the Gem Box; other items are only named.
func (s *Service) AppraiseItem(ctx context.Context, characterID, itemInstanceOrDefID string) (AppraiseResult, error) {
	characterID = strings.TrimSpace(characterID)
	itemInstanceOrDefID = strings.TrimSpace(itemInstanceOrDefID)
	if characterID == "" {
		return AppraiseResult{}, ErrInvalidCharacterID
	}
	if itemInstanceOrDefID == "" {
		return AppraiseResult{}, ErrItemNotOwned
	}

	var res AppraiseResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		// Legacy kantei reads only depot.cgi; hand slots are never appraised.
		var dep depot.Depot
		if s.depots != nil {
			d, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
			if err != nil && !errors.Is(err, depot.ErrNotFound) {
				return err
			}
			if err == nil {
				dep = d
			}
		}

		box, err := s.getOrCreateGemBoxForUpdate(txCtx, char)
		if err != nil {
			return err
		}

		targetQuery := depot.QueryByMatch(func(inst coreitem.Instance) bool {
			if inst.ID == itemInstanceOrDefID || inst.DefinitionID == itemInstanceOrDefID {
				return true
			}
			return resolveItemName(inst.DefinitionID, s.catalog, s.items) == itemInstanceOrDefID
		})
		resolved, err := depot.ResolveItem(nil, &dep, targetQuery, depot.PriorityDepotFirst)
		if err != nil {
			return ErrItemNotOwned
		}
		targetItem := resolved.Item

		itemName := resolveItemName(targetItem.DefinitionID, s.catalog, s.items)

		// Check if it's an unidentified orb that can be appraised into a gem
		if gem, isUnidentified, err := s.catalog.AppraiseUnidentifiedItem(itemName, s.randomSource); err != nil {
			return err
		} else if isUnidentified {
			if _, err := dep.Consume(targetItem.ID, 1); err != nil {
				return err
			}

			gemInstance, err := coreitem.NewInstance(gem.ID, 1)
			if err != nil {
				return err
			}

			box.Deliver(gemInstance) // gem_store.cgi:374 send_gem ignores capacity

			if err := s.depots.Save(txCtx, dep); err != nil {
				return err
			}
			if err := s.gemBoxes.Save(txCtx, box); err != nil {
				return err
			}

			res = AppraiseResult{
				Character:      char,
				GemBox:         box,
				Depot:          dep,
				IsGem:          true,
				IdentifiedGem:  &gem,
				IdentifiedName: gem.Name,
				Message:        fmt.Sprintf("これは… %sですね。%sさんの宝石箱に入れておきました", gem.Name, char.Name),
			}
			return nil
		}

		// Otherwise, it's standard identified equipment/item
		res = AppraiseResult{
			Character:      char,
			GemBox:         box,
			Depot:          dep,
			IsGem:          false,
			IdentifiedName: itemName,
			Message:        fmt.Sprintf("これは… %sですね", itemName),
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return AppraiseResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return AppraiseResult{}, err
		}
	}

	return res, nil
}

// -------------------------------------------------------------------
// Helper functions
// -------------------------------------------------------------------

func resolveItemName(defID string, catalog *Catalog, items ItemDefinitionProvider) string {
	if g, ok := catalog.FindGemByID(defID); ok {
		return g.Name
	}
	if items != nil {
		if d, err := items.FindByID(defID); err == nil {
			return d.Name
		}
	}
	return defID
}

func removeMaterialFromGemBox(
	box *GemBox,
	matName string,
	catalog *Catalog,
	items ItemDefinitionProvider,
) (coreitem.Instance, error) {
	for i, inst := range box.Items {
		name := resolveItemName(inst.DefinitionID, catalog, items)
		if inst.ID == matName || inst.DefinitionID == matName || name == matName {
			removed := inst
			box.Items = append(box.Items[:i], box.Items[i+1:]...)
			return removed, nil
		}
	}
	return coreitem.Instance{}, ErrItemNotOwned
}
