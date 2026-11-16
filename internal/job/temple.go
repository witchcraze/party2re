package job

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

// TempleStatus is what the job temple offers a character right now. Every
// list holds only choices the matching command would currently accept.
type TempleStatus struct {
	Changeable     []corejob.Definition
	Recallable     []corejob.Definition
	RecallItem     string // the held recall item ExchangeJob would use, or ""
	FutureMemories []corecharacter.FutureMemory
}

// TempleStatus reads the job temple choices without mutating anything; any
// required read failure fails the whole status.
func (s *Service) TempleStatus(ctx context.Context, characterID string) (TempleStatus, error) {
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return TempleStatus{}, err
	}
	state, err := s.loadState(ctx, char)
	if err != nil {
		return TempleStatus{}, err
	}
	var inv coreinventory.Inventory
	if s.inventories != nil {
		if inv, err = s.inventories.FindByCharacterID(ctx, characterID); err != nil {
			return TempleStatus{}, err
		}
	}
	var equip coreequipment.Equipment
	if s.equipment != nil {
		if equip, err = s.equipment.FindByCharacterID(ctx, characterID); err != nil {
			return TempleStatus{}, err
		}
	}
	memories, err := s.ListFutureMemories(ctx, characterID)
	if err != nil {
		return TempleStatus{}, err
	}
	status := TempleStatus{FutureMemories: memories}
	for _, id := range recallItemPreference {
		if inv.Quantity(id) > 0 {
			status.RecallItem = id
			break
		}
	}
	reqCtx := s.buildChangeContextNoItems(char, state)
	for _, def := range s.ListDefinitions() {
		if def.RequiredGender != "" && def.RequiredGender != char.Gender {
			continue
		}
		// Legacy lists every job_master.cgi entry except job 0 (job_change.cgi:402).
		if _, ok := state.SavedSP(def.ID); ok && state.HasRecordedJob(def.ID) && def.ID != "starter" &&
			!char.OverLevel && char.JobMemory == nil {
			status.Recallable = append(status.Recallable, def)
		}
		if char.JobMemory != nil || corejob.ValidateRequirements(def, reqCtx) != nil {
			continue
		}
		if corejob.RequiresItemPossession(def, char.JobID, char.OldJobID) && inv.Quantity(def.RequiredItem()) < 1 {
			continue
		}
		if corejob.IsArmorConsumed(def.ID, char.JobID, char.OldJobID) && !armorEquipped(equip, inv) {
			continue
		}
		status.Changeable = append(status.Changeable, def)
	}
	return status, nil
}

// armorEquipped reports whether armor-29, which job-84 consumes, is worn.
func armorEquipped(equip coreequipment.Equipment, inv coreinventory.Inventory) bool {
	instID, ok := equip.Equipped(coreitem.SlotBody)
	if !ok {
		return false
	}
	inst, found := inv.Find(instID)
	return found && inst.DefinitionID == "armor-29"
}
