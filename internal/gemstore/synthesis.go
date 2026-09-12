package gemstore

import (
	"context"
	"fmt"
	"strings"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		var dep depot.Depot
		if s.depots != nil {
			if d, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID); err == nil {
				dep = d
			}
		}

		box, err := s.getOrCreateGemBoxForUpdate(txCtx, char)
		if err != nil {
			return err
		}

		// Consume material 1 (try GemBox first, then inventory, then depot)
		consumedFromGemBox := 0
		if _, err := removeMaterialFromGemBox(&box, recipe.Material1, s.catalog, s.items); err == nil {
			consumedFromGemBox++
		} else if mat1Item, ok := findMaterialInInventory(inv, recipe.Material1, s.catalog, s.items); ok {
			if err := inv.Consume(mat1Item.ID, 1); err != nil {
				return err
			}
		} else if mat1Dep, ok := findMaterialInDepot(dep, recipe.Material1, s.catalog, s.items); ok {
			if _, err := dep.ConsumeOne(mat1Dep.ID); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%w: missing %s", ErrInsufficientMaterials, recipe.Material1)
		}

		// Consume material 2 (try GemBox first, then inventory, then depot)
		if _, err := removeMaterialFromGemBox(&box, recipe.Material2, s.catalog, s.items); err == nil {
			consumedFromGemBox++
		} else if mat2Item, ok := findMaterialInInventory(inv, recipe.Material2, s.catalog, s.items); ok {
			if err := inv.Consume(mat2Item.ID, 1); err != nil {
				return err
			}
		} else if mat2Dep, ok := findMaterialInDepot(dep, recipe.Material2, s.catalog, s.items); ok {
			if _, err := dep.ConsumeOne(mat2Dep.ID); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%w: missing %s", ErrInsufficientMaterials, recipe.Material2)
		}

		// Capacity check: if net gem box count increases
		if consumedFromGemBox == 0 && box.IsFull() {
			return ErrGemBoxFull
		}

		newInstance, err := coreitem.NewInstance(resultGem.ID, 1)
		if err != nil {
			return err
		}

		if err := box.AddItem(newInstance); err != nil {
			return err
		}

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

// AppraiseItem appraises an unidentified orb or equipment in character inventory, placing revealed gems into the Gem Box.
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

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		box, err := s.getOrCreateGemBoxForUpdate(txCtx, char)
		if err != nil {
			return err
		}

		targetItem, ok := findItemInInventory(inv, itemInstanceOrDefID, s.catalog, s.items)
		if !ok {
			return ErrItemNotOwned
		}

		itemName := resolveItemName(targetItem.DefinitionID, s.catalog, s.items)

		// Check if it's an unidentified orb that can be appraised into a gem
		if gem, isUnidentified, err := s.catalog.AppraiseUnidentifiedItem(itemName, s.randomSource); err != nil {
			return err
		} else if isUnidentified {
			if box.IsFull() {
				return ErrGemBoxFull
			}

			if err := inv.Consume(targetItem.ID, 1); err != nil {
				return err
			}

			gemInstance, err := coreitem.NewInstance(gem.ID, 1)
			if err != nil {
				return err
			}

			if err := box.AddItem(gemInstance); err != nil {
				return err
			}

			if err := s.inventories.Save(txCtx, inv); err != nil {
				return err
			}
			if err := s.gemBoxes.Save(txCtx, box); err != nil {
				return err
			}

			res = AppraiseResult{
				Character:      char,
				GemBox:         box,
				Inventory:      inv,
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
			Inventory:      inv,
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

func findItemInInventory(
	inv coreinventory.Inventory,
	target string,
	catalog *Catalog,
	items ItemDefinitionProvider,
) (coreitem.Instance, bool) {
	for _, inst := range inv.Items {
		if inst.ID == target || inst.DefinitionID == target {
			return inst, true
		}
		name := resolveItemName(inst.DefinitionID, catalog, items)
		if name == target {
			return inst, true
		}
	}
	return coreitem.Instance{}, false
}

func findMaterialInInventory(
	inv coreinventory.Inventory,
	matName string,
	catalog *Catalog,
	items ItemDefinitionProvider,
) (coreitem.Instance, bool) {
	for _, inst := range inv.Items {
		name := resolveItemName(inst.DefinitionID, catalog, items)
		if name == matName || inst.DefinitionID == matName {
			return inst, true
		}
	}
	return coreitem.Instance{}, false
}

func findMaterialInDepot(
	dep depot.Depot,
	matName string,
	catalog *Catalog,
	items ItemDefinitionProvider,
) (coreitem.Instance, bool) {
	for _, inst := range dep.Items {
		name := resolveItemName(inst.DefinitionID, catalog, items)
		if name == matName || inst.DefinitionID == matName {
			return inst, true
		}
	}
	return coreitem.Instance{}, false
}

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
