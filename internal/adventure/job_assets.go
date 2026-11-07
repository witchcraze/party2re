package adventure

import (
	"context"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

// WithJobAssets supplies requester facts to the enclosing, fully admitted combat
// operation. Call once inside Command, including NPC/delegated effects; the run
// lock/receipt and economy runner share the ambient SQL transaction. The
// command's TargetID and Input are job50's outer stored-tool selector and target.
// A selection error commits no assets; stored-tool history stays in the run.
// Transaction: ExecuteTransaction within Command. Lock Order: prelocked run(0) -> prelocked sorted characters(2) -> requester inventory(3) -> requester depot(5).
func (s *NormalRunService) WithJobAssets(ctx context.Context, r *NormalRun, cmd RunCommand, apply func(*corebattle.JobAssets) error) error {
	if r == nil || ctx.Value(normalMutationContext{}) != r.ID || cmd.RunID != r.ID ||
		!slices.Contains(r.Roster, cmd.ActorID) || r.Recipients[cmd.ActorID].PlayerID != cmd.PlayerID {
		return ErrRunForbidden
	}
	if s.cfg.JobAssetRunner == nil || apply == nil {
		return corebattle.ErrUnsupportedEffect
	}
	_, err := s.cfg.JobAssetRunner.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: cmd.ActorID, LockInventory: true}, func(tc *economy.TxContext) error {
		if tc.Character.PlayerID != cmd.PlayerID {
			return ErrRunForbidden
		}
		assets := corebattle.JobAssets{RequesterID: cmd.ActorID, Wallet: tc.Character.Money, AddWallet: economy.SafeAdd}
		assets.HeldToolID = tc.Inventory.HeldToolID()
		if s.cfg.JobAssetHelpers != nil {
			tools, err := s.cfg.JobAssetHelpers(ctx)
			if err != nil {
				return err
			}
			assets.HelperToolIDs = append([]int{}, tools...)
		}
		assets.StoredItem, assets.StoredTarget = cmd.TargetID, cmd.Input
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
		if err := apply(&assets); err != nil {
			return err
		}
		if assets.RequesterID != cmd.ActorID || assets.Wallet < 0 {
			return ErrRunConflict
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
		if assets.HeldToolID == nil || *assets.HeldToolID == previousHeld {
			return nil
		}
		switch {
		case previousHeld == "":
			tc.AddGrant(economy.ResourceGrant{ItemDefinitionID: *assets.HeldToolID, ItemQuantity: 1})
		case *assets.HeldToolID == "":
			// A known held hand is exactly one inventory row.
			return tc.Inventory.ConsumeOne(tc.Inventory.Items[0].ID)
		default:
			return corebattle.ErrUnsupportedEffect
		}
		return nil
	})
	return err
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
		return held, ErrRunConflict
	}
	taken, err := d.ConsumeOne(instanceID)
	if err != nil {
		return held, err
	}
	whole := index >= len(d.Items) || d.Items[index].ID != instanceID
	if held != "" {
		previous, err := tc.Inventory.ConsumeOneItem(tc.Inventory.Items[0].ID)
		if err != nil {
			return held, err
		}
		d.InsertDelivery(index, previous)
	}
	if whole {
		err = tc.Inventory.Add(taken)
	} else {
		tc.AddGrant(economy.ResourceGrant{ItemDefinitionID: taken.DefinitionID, ItemQuantity: 1})
	}
	return taken.DefinitionID, err
}
