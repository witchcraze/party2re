package adventure

import (
	"context"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/economy"
)

// WithJobAssets supplies requester facts to the enclosing, fully admitted combat
// operation. Call once inside Command, including NPC/delegated effects; the run
// lock/receipt and economy runner share the ambient SQL transaction.
// Transaction: ExecuteTransaction within Command. Lock Order: prelocked run(0) -> prelocked sorted characters(2) -> requester inventory(3).
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
