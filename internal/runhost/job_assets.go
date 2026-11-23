package runhost

import (
	"context"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
)

// withJobAssets supplies requester facts to the enclosing, fully admitted combat
// operation. Call once inside Command, including NPC/delegated effects; the run
// lock/receipt and economy runner share the ambient SQL transaction. Tail growth
// updates the same locked requester that the runner uses to settle assets. The
// command's TargetID and Input are job50's outer stored-tool selector and target.
// Job86 withdrawals swap in place; returns deliver even over capacity.
// A selection error commits no assets; stored-tool history stays in the run.
// Transaction: ExecuteTransaction within Command. Lock Order: prelocked run(0) -> prelocked sorted characters(2) -> requester inventory(3) -> requester equipment(3) -> requester depot(5).
func (s *Service[R, P]) withJobAssets(ctx context.Context, r *State, cmd Command, apply func(*corebattle.JobAssets, *corecharacter.Character) error) error {
	if r == nil || !inCommand(ctx, r.ID) || cmd.RunID != r.ID ||
		!slices.Contains(r.Roster, cmd.ActorID) || r.Recipients[cmd.ActorID].PlayerID != cmd.PlayerID {
		return ErrForbidden
	}
	if s.cfg.JobAssetRunner == nil || apply == nil {
		return corebattle.ErrUnsupportedEffect
	}
	_, err := s.cfg.JobAssetRunner.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: cmd.ActorID, LockInventory: true}, func(tc *economy.TxContext) error {
		if tc.Character.PlayerID != cmd.PlayerID {
			return ErrForbidden
		}
		assets := corebattle.JobAssets{RequesterID: cmd.ActorID, Wallet: tc.Character.Money, AddWallet: economy.SafeAdd}
		assets.HeldToolID = tc.Inventory.HeldToolID()
		heldBefore, _ := tc.Inventory.HeldTool()
		if s.cfg.JobAssetHelpers != nil {
			tools, err := s.cfg.JobAssetHelpers(ctx)
			if err != nil {
				return err
			}
			assets.HelperToolIDs = append([]int{}, tools...)
		}
		assets.StoredItem, assets.StoredTarget = cmd.TargetID, cmd.Input
		var eq *coreequipment.Equipment
		if s.cfg.JobAssetEquipment != nil {
			value, err := s.cfg.JobAssetEquipment.FindByCharacterIDForUpdate(tc, cmd.ActorID)
			if err != nil {
				return err
			}
			eq = &value
		}
		var stored *depot.Depot
		if s.cfg.JobAssetDepots != nil {
			value, err := depot.FindOrCreate(tc, s.cfg.JobAssetDepots, tc.Character)
			if err != nil {
				return err
			}
			stored, assets.StoredTools = &value, storedTools(value)
		}
		previousHeld := ""
		if assets.HeldToolID != nil {
			previousHeld = *assets.HeldToolID
		}
		if err := apply(&assets, &tc.Character); err != nil {
			return err
		}
		if assets.RequesterID != cmd.ActorID || assets.Wallet < 0 {
			return ErrConflict
		}
		if assets.SelectionError {
			return nil
		}
		if len(assets.StoredOutcomes) > 0 {
			if stored == nil {
				return corebattle.ErrUnsupportedEffect
			}
			for _, o := range assets.StoredOutcomes {
				var err error
				switch o.Result {
				case corebattle.StoredToolConsumed:
					_, err = stored.ConsumeOne(o.InstanceID)
				case corebattle.StoredToolExchanged:
					previousHeld, err = exchangeStoredTool(tc, stored, o.InstanceID, previousHeld)
				case corebattle.BookReturned:
					previousHeld, err = returnHeldTool(tc, stored, previousHeld)
				}
				if err != nil {
					return err
				}
			}
			if err := s.cfg.JobAssetDepots.Save(tc, *stored); err != nil {
				return err
			}
		}
		if assets.Wallet < tc.Character.Money {
			if err := tc.Character.DeductMoney(tc.Character.Money - assets.Wallet); err != nil {
				return err
			}
		} else {
			tc.AddGrant(economy.ResourceGrant{Gold: assets.Wallet - tc.Character.Money})
		}
		if assets.HeldToolID != nil && *assets.HeldToolID != previousHeld {
			switch {
			case previousHeld == "":
				inst, err := item.NewInstance(*assets.HeldToolID, 1)
				if err != nil {
					return err
				}
				if err := tc.Inventory.Add(inst); err != nil {
					return err
				}
			case *assets.HeldToolID == "":
				if _, err := takeHeld(tc); err != nil {
					return err
				}
			default:
				return corebattle.ErrUnsupportedEffect
			}
		}
		return s.saveHeldGear(tc, eq, heldBefore.ID)
	})
	return err
}

// takeHeld removes the durable held unit: the inventory's tool-hand row.
func takeHeld(tc *economy.TxContext) (item.Instance, error) {
	held, ok := tc.Inventory.HeldTool()
	if !ok || held.ID == "" {
		return item.Instance{}, ErrConflict
	}
	return tc.Inventory.ConsumeOneItem(held.ID)
}

// saveHeldGear keeps equipment_slots on the tool hand as Depot withdrawal does:
// the leaving row is unlinked before the runner deletes it, and gear entering
// the hand is equipped once its inventory row exists.
func (s *Service[R, P]) saveHeldGear(tc *economy.TxContext, eq *coreequipment.Equipment, before string) error {
	held, _ := tc.Inventory.HeldTool()
	if eq == nil || held.ID == before {
		return nil
	}
	if before != "" {
		unlinked, err := eq.Detach(&tc.Character, before)
		if err != nil {
			return err
		}
		if unlinked {
			if err := s.cfg.JobAssetEquipment.Save(tc, *eq); err != nil {
				return err
			}
		}
	}
	if held.ID == "" {
		return nil
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return err
	}
	def, err := catalog.FindByID(held.DefinitionID)
	if err != nil || def.Slot == item.SlotNone {
		return err
	}
	if err := s.cfg.Inventories.Save(tc, tc.Inventory); err != nil {
		return err
	}
	if _, err := eq.Equip(&tc.Inventory, def, held.ID); err != nil {
		return err
	}
	return s.cfg.JobAssetEquipment.Save(tc, *eq)
}

// storedTools lists depot tool rows in order with their usage category.
func storedTools(d depot.Depot) []corebattle.StoredTool {
	tools := []corebattle.StoredTool{}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return nil // Unknown categories leave job50 unsupported.
	}
	for _, inst := range d.Items {
		if def, err := catalog.FindByID(inst.DefinitionID); err == nil && def.IsTool() {
			tools = append(tools, corebattle.StoredTool{InstanceID: inst.ID, DefinitionID: inst.DefinitionID, Category: def.UsageCategory})
		}
	}
	return tools
}

// exchangeStoredTool puts the durable held tool, if any, at the selected row's
// position, capacity-unchecked like the source rewrite, and equips one unit of
// that row. A remaining stack keeps its identity; the split unit is new.
func exchangeStoredTool(tc *economy.TxContext, d *depot.Depot, instanceID, held string) (string, error) {
	index := slices.IndexFunc(d.Items, func(i item.Instance) bool { return i.ID == instanceID })
	if index < 0 {
		return held, ErrConflict
	}
	taken, err := d.ConsumeOne(instanceID)
	if err != nil {
		return held, err
	}
	if index < len(d.Items) && d.Items[index].ID == instanceID {
		taken.ID = id.New() // The remaining stack keeps its identity.
	}
	if held != "" {
		previous, err := takeHeld(tc)
		if err != nil {
			return held, err
		}
		d.InsertDelivery(index, previous)
	}
	return taken.DefinitionID, tc.Inventory.Add(taken)
}

// returnHeldTool delivers the durable held unit to the requester depot: it
// merges into an eligible stack or appends, even at or over capacity.
func returnHeldTool(tc *economy.TxContext, d *depot.Depot, held string) (string, error) {
	if held == "" {
		return held, ErrConflict
	}
	unit, err := takeHeld(tc)
	if err != nil {
		return held, err
	}
	d.Deliver(unit)
	return "", nil
}
