package runhost

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/economy"
)

// StockLevelAction applies saved-EXP stock levels as an operation (system.cgi:49–67).
const StockLevelAction = "stock-level"

// Command serializes one explicit operation; observation never calls it.
// Transaction: WithRunForUpdate. Lock Order: runs(0) -> all characters(2, sorted) -> settler(3/4/5/8).
func (s *Service[R, P]) Command(ctx context.Context, cmd Command) (Receipt, error) {
	if err := s.authorize(ctx, cmd.PlayerID, cmd.ActorID); err != nil {
		return Receipt{}, err
	}
	if strings.TrimSpace(cmd.ID) == "" || len(cmd.ID) > 128 || cmd.RunID == "" || cmd.Action == "enroll" {
		return Receipt{}, ErrConflict
	}
	var result Receipt
	err := s.cfg.Store.WithRunForUpdate(ctx, cmd.RunID, false, func(ctx context.Context) error {
		old, err := s.cfg.Store.FindRunReceipt(ctx, cmd.RunID, cmd.ID)
		if err != nil {
			return err
		}
		if old != nil {
			if old.Command.PlayerID != cmd.PlayerID || old.Command.ActorID != cmd.ActorID {
				return ErrForbidden
			}
			if old.Command != cmd {
				return ErrConflict
			}
			result = *old
			return nil
		}
		run, err := s.cfg.Store.FindRun(ctx, cmd.RunID)
		if err != nil {
			return err
		}
		if run == nil {
			return ErrNotFound
		}
		r := P(run).Host()
		if r.Encoding != 1 || r.Kind != s.cfg.Kind {
			return ErrRecovery
		}
		if !slices.Contains(r.Roster, cmd.ActorID) || r.Recipients[cmd.ActorID].PlayerID != cmd.PlayerID {
			return ErrForbidden
		}
		if r.Version != cmd.Version {
			return ErrConflict
		}
		if r.TimedOut {
			return ErrRecovery
		}
		var ids []string
		var requester corecharacter.Character
		ctx = commandContext(ctx, r.ID)
		if err := economy.LockInOrder(ctx, r.Roster, func(ctx context.Context, id string) error {
			c, err := s.cfg.Characters.FindByIDForUpdate(ctx, id)
			if err != nil {
				return err
			}
			if c.PlayerID != r.Recipients[id].PlayerID {
				return ErrForbidden
			}
			if id == cmd.ActorID {
				requester = c
			}
			ids = append(ids, id)
			return nil
		}); err != nil {
			return err
		}
		now := s.cfg.Clock.Now()
		releaseStock := !s.cfg.Room.Conditional(run) || cmd.Action == "exit"
		effects := Effects{}
		// Releases credit EXP without growth; only an action tail levels (get_stock).
		request := battle.ApplyPostBattleRequest{CharacterIDs: ids, PreserveEarnedRewards: true, RecipientRewards: map[string]corebattle.Reward{}, DeferGrowth: true}
		var box *TreasureBox
		var tames []battle.MonsterTameResult
		tail := false
		switch {
		case cmd.Action == "cleanup":
			if !now.After(r.LastActivity.Add(1800 * time.Second)) {
				return ErrConflict
			}
			if err := s.timeout(ctx, run, now); err != nil {
				return err
			}
		case cmd.Action == StockLevelAction:
			if err := s.stockLevel(ctx, run, cmd, requester, now); err != nil {
				return err
			}
			effects.RoomActivity, effects.ActorActivity = true, true
		case cmd.Action == "chest" || cmd.Action == "exit" || slices.Contains(s.cfg.Room.Actions(), cmd.Action):
			if err := r.Ready(cmd.ActorID, now); err != nil {
				return err
			}
			switch cmd.Action {
			case "chest":
				var err error
				box, effects.Consumed, err = r.ClaimChest(ctx, s.cfg.Inventories, cmd.ActorID, cmd.TargetID)
				if err != nil {
					return err
				}
				request.RecipientDrops = map[string][]string{cmd.ActorID: {box.ItemID}}
				request.BattleResult.Outcome = corebattle.OutcomeWin
				tail = true
			case "exit":
				if err := s.prepareExit(run, cmd.ActorID, &request); err != nil {
					return err
				}
			default:
				var err error
				if effects, err = s.cfg.Room.Act(ctx, run, cmd); err != nil {
					return err
				}
				request.UnsealDemonKing = effects.UnsealDemonKing
				tail = true
			}
			effects.RoomActivity, effects.ActorActivity, effects.ReleaseStock = true, true, true
		default:
			var err error
			effects, err = s.executeCombat(ctx, run, cmd)
			if err != nil {
				return err
			}
			request.UnsealDemonKing = effects.UnsealDemonKing
			request.RecipientCharacterID, request.ExplicitDefeats = cmd.ActorID, effects.Defeats
		}
		if cmd.Action != "cleanup" && cmd.Action != StockLevelAction {
			releaseStock = (releaseStock && effects.ReleaseStock) || cmd.Action == "exit"
			if err := r.RecordEarnings(effects.Earnings, s.cfg.Room.Conditional(run)); err != nil {
				return err
			}
			var retained []RetainedEarnings
			if releaseStock {
				recipient := r.Recipients[cmd.ActorID]
				var due Earnings
				var err error
				retained, due, err = s.retainedDue(ctx, cmd.ActorID, recipient.Pending)
				if err != nil {
					return err
				}
				request.RecipientRewards[cmd.ActorID] = Reward(due)
			}
			if tail {
				if err := s.tailLevel(ctx, r, requester, request.RecipientRewards[cmd.ActorID].Experience, s.cfg.Room.Conditional(run)); err != nil {
					return err
				}
			}
			request.BattleResult.ConsumedItems = effects.Consumed
			// Consume concrete owned instances in this transaction, never at exit again.
			for id, entries := range effects.Consumed {
				if !slices.Contains(r.Roster, id) && id != cmd.ActorID {
					return ErrForbidden
				}
				for _, entry := range entries {
					if entry.InstanceID == "" || entry.Quantity <= 0 {
						return ErrConflict
					}
				}
			}
			response, err := s.cfg.Settler.ApplyPostBattleResult(ctx, request)
			if err != nil {
				return err
			}
			tames = response.MonsterTames[cmd.ActorID]
			if box != nil {
				boxes := []TreasureBox{*box}
				UpdateTreasureDeliveries(boxes, response)
				*box = boxes[0]
			}
			if releaseStock {
				recipient := r.Recipients[cmd.ActorID]
				recipient.Applied, err = AddEarnings(recipient.Applied, recipient.Pending)
				if err != nil {
					return err
				}
				recipient.Pending = Earnings{}
				r.Recipients[cmd.ActorID] = recipient
				if err := s.settleRetained(ctx, cmd.ActorID, retained); err != nil {
					return err
				}
			}
			if effects.Settled != nil {
				if err := effects.Settled(ctx); err != nil {
					return err
				}
			}
			if cmd.Action == "exit" {
				if err := s.finishExit(ctx, run, cmd.ActorID); err != nil {
					return err
				}
			}
		}
		r.PruneFallenNPCs(cmd.ActorID)
		if effects.RoomActivity && len(r.Roster) > 0 {
			r.LastActivity = now
		}
		if effects.ActorActivity {
			r.LastActions[cmd.ActorID] = now
		}
		r.Version++
		recipients := make([]string, 0, len(r.Recipients))
		for id := range r.Recipients {
			recipients = append(recipients, id)
		}
		slices.Sort(recipients)
		for _, id := range recipients {
			if err := s.cfg.Store.SaveRunRecipient(ctx, r.ID, id, r.Recipients[id]); err != nil {
				return err
			}
		}
		if err := s.cfg.Store.SaveRun(ctx, *run, cmd.Version); err != nil {
			return err
		}
		result = Receipt{Command: cmd, Version: r.Version, Round: r.Round, Box: box, Recipient: r.Recipients[cmd.ActorID], Disposition: r.Recipients[cmd.ActorID].Disposition, MonsterTames: tames}
		return s.cfg.Store.SaveRunReceipt(ctx, result)
	})
	result, err = s.resolveReceipt(ctx, cmd, result, err)
	if err == nil && cmd.Action == "cleanup" {
		err = s.RecoverTimeout(ctx, cmd.PlayerID, cmd.ActorID, cmd.RunID)
	}
	return result, err
}
