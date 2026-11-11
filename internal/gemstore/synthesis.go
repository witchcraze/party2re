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
// Transaction: RunInTx
// Lock Order: characters(2) -> character_depots(5) -> character_gem_boxes(8)
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

	resultGem, ok := s.catalog.FindGemByID(recipe.ResultID)
	if !ok {
		return SynthesizeResult{}, ErrGemNotFound
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

		// Legacy synthesis searches the Gem Box, then Depot; hand slots are excluded.
		for _, materialID := range []string{recipe.Material1, recipe.Material2} {
			if _, err := removeMaterialFromGemBox(&box, materialID); err != nil {
				if _, err := depot.ConsumeItem(nil, &dep, depot.QueryByDefinitionID(materialID), depot.PriorityDepotFirst, 1); err != nil {
					return &InsufficientMaterialsError{Recipe: recipe}
				}
			}
		}

		newInstance, err := coreitem.NewInstance(resultGem.ID, 1)
		if err != nil {
			return err
		}
		box.Deliver(newInstance) // gem_store.cgi:309 send_gem ignores capacity

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

func removeMaterialFromGemBox(box *GemBox, definitionID string) (coreitem.Instance, error) {
	for i, inst := range box.Items {
		if inst.DefinitionID == definitionID {
			removed := inst
			box.Items = append(box.Items[:i], box.Items[i+1:]...)
			return removed, nil
		}
	}
	return coreitem.Instance{}, ErrItemNotOwned
}
