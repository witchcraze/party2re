package shop

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/depot"
)

const HiyakuItemDefinitionID = "item-180"

type SynthesisResult struct {
	Recipe             SynthesisRecipe `json:"recipe"`
	Success            bool            `json:"success"`
	UsedHiyaku         bool            `json:"used_hiyaku"`
	CreatedItem        *item.Instance  `json:"created_item,omitempty"`
	ConsumedMaterials  []item.Instance `json:"consumed_materials"`
	ConsumedHiyakuItem *item.Instance  `json:"consumed_hiyaku_item,omitempty"`
	Depot              depot.Depot     `json:"depot"`
}

// Synthesize executes an accessory synthesis from materials in depot.
// If the player holds item-180 (合成の秘薬) in inventory, it is consumed and guarantees 100% success.
// Otherwise, the recipe's success rate is rolled. Materials are consumed from depot on both success and failure.
// Finished items are delivered directly to the depot.
func (s *Service) Synthesize(ctx context.Context, characterID string, recipeTarget string) (SynthesisResult, error) {
	if characterID == "" {
		return SynthesisResult{}, corecharacter.ErrNotFound
	}
	if s.depots == nil {
		return SynthesisResult{}, ErrDepotNotConfigured
	}

	recipe, err := FindSynthesisRecipe(recipeTarget)
	if err != nil {
		return SynthesisResult{}, ErrRecipeNotFound
	}

	if _, err := s.catalog.FindByID(recipe.ProductDefinitionID); err != nil {
		return SynthesisResult{}, ErrItemNotFound
	}

	var result SynthesisResult
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character (Rank 2)
		char, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return corecharacter.ErrNotFound
		}

		// 2. Lock Inventory (Rank 3)
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		// Check for Hiyaku (item-180) in character inventory
		hasHiyaku := false
		var hiyakuInstanceID string
		for _, inst := range inv.Items {
			if inst.DefinitionID == HiyakuItemDefinitionID {
				hasHiyaku = true
				hiyakuInstanceID = inst.ID
				break
			}
		}

		// 3. Lock Depot (Rank 5) using depot.FindOrCreate
		dep, err := depot.FindOrCreate(txCtx, s.depots, char)
		if err != nil {
			return err
		}

		// Find Material 1 and Material 2 in depot
		var mat1Inst, mat2Inst item.Instance
		found1, found2 := false, false

		for _, inst := range dep.Items {
			if !found1 && inst.DefinitionID == recipe.Material1DefinitionID && inst.Quantity >= 1 {
				mat1Inst = inst
				found1 = true
				continue
			}
			if !found2 && inst.DefinitionID == recipe.Material2DefinitionID && inst.Quantity >= 1 {
				mat2Inst = inst
				found2 = true
			}
		}
		if !found1 || !found2 {
			return ErrMaterialsNotFound
		}

		// Consume Material 1 (1 quantity) from depot
		consumed1, err := dep.Consume(mat1Inst.ID, 1)
		if err != nil {
			return err
		}
		// Consume Material 2 (1 quantity) from depot
		consumed2, err := dep.Consume(mat2Inst.ID, 1)
		if err != nil {
			return err
		}

		consumedMaterials := []item.Instance{consumed1, consumed2}

		var consumedHiyaku *item.Instance
		success := false

		if hasHiyaku {
			// Item 180 guarantees 100% success
			success = true
			consumedHiyakuItem, err := inv.ConsumeItem(hiyakuInstanceID, 1)
			if err != nil {
				return err
			}
			if err := s.inventories.Save(txCtx, inv); err != nil {
				return err
			}
			consumedHiyaku = &consumedHiyakuItem
		} else {
			// Rate-based success check: rand(100) > comp fails in legacy CGI
			roll := random.Intn(100)
			if roll < recipe.SuccessRate {
				success = true
			}
		}

		var createdInstance *item.Instance
		if success {
			inst, err := item.NewInstance(recipe.ProductDefinitionID, 1)
			if err != nil {
				return err
			}
			if err := dep.AddItem(inst); err != nil {
				return err
			}
			createdInstance = &inst
		}

		if err := s.depots.Save(txCtx, dep); err != nil {
			return err
		}

		result = SynthesisResult{
			Recipe:             recipe,
			Success:            success,
			UsedHiyaku:         hasHiyaku,
			CreatedItem:        createdInstance,
			ConsumedMaterials:  consumedMaterials,
			ConsumedHiyakuItem: consumedHiyaku,
			Depot:              dep,
		}
		return nil
	})
	if err != nil {
		return SynthesisResult{}, err
	}
	return result, nil
}
