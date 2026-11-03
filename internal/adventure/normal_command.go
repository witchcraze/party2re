package adventure

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Command serializes one explicit operation; observation never calls it.
// Transaction: WithRunForUpdate. Lock Order: normal_adventure_runs(0) -> all characters(2, sorted) -> settler(3/4/5/8).
func (s *NormalRunService) Command(ctx context.Context, cmd RunCommand) (RunReceipt, error) {
	if err := s.authorize(ctx, cmd.PlayerID, cmd.ActorID); err != nil {
		return RunReceipt{}, err
	}
	if strings.TrimSpace(cmd.ID) == "" || len(cmd.ID) > 128 || cmd.RunID == "" || cmd.Action == "enroll" {
		return RunReceipt{}, ErrRunConflict
	}
	var result RunReceipt
	err := s.cfg.Store.WithRunForUpdate(ctx, cmd.RunID, false, func(ctx context.Context) error {
		old, err := s.cfg.Store.FindRunReceipt(ctx, cmd.RunID, cmd.ID)
		if err != nil {
			return err
		}
		if old != nil {
			if old.Command.PlayerID != cmd.PlayerID || old.Command.ActorID != cmd.ActorID {
				return ErrRunForbidden
			}
			if old.Command != cmd {
				return ErrRunConflict
			}
			result = *old
			return nil
		}
		r, err := s.cfg.Store.FindRun(ctx, cmd.RunID)
		if err != nil {
			return err
		}
		if r == nil {
			return ErrNotFound
		}
		if r.Encoding != 1 {
			return ErrRunRecovery
		}
		if !slices.Contains(r.Roster, cmd.ActorID) || r.Recipients[cmd.ActorID].PlayerID != cmd.PlayerID {
			return ErrRunForbidden
		}
		if r.Version != cmd.Version {
			return ErrRunConflict
		}
		if r.TimedOut {
			return ErrRunRecovery
		}
		ids := slices.Clone(r.Roster)
		ctx = context.WithValue(ctx, normalMutationContext{}, r.ID)
		slices.Sort(ids)
		for _, id := range ids {
			c, err := s.cfg.Characters.FindByIDForUpdate(ctx, id)
			if err != nil {
				return err
			}
			if c.PlayerID != r.Recipients[id].PlayerID {
				return ErrRunForbidden
			}
		}
		now := s.cfg.Clock.Now()
		releaseStock := r.Mode != CrawlModeTry || cmd.Action == "exit"
		effects := RunActionEffects{}
		request := battle.ApplyPostBattleRequest{CharacterIDs: ids, PreserveEarnedRewards: true, RecipientRewards: map[string]corebattle.Reward{}}
		var box *TreasureBox
		switch cmd.Action {
		case "advance", "chest", "exit":
			if now.Before(r.LastActions[cmd.ActorID].Add(time.Duration(r.Admission.Speed) * time.Second)) {
				return ErrRunCooldown
			}
			effects.RoomActivity, effects.ActorActivity, effects.ReleaseStock = true, true, true
			switch cmd.Action {
			case "advance":
				if err := s.advance(ctx, r, cmd.ActorID); err != nil {
					return err
				}
			case "chest":
				var err error
				box, effects.Consumed, err = s.claimChest(ctx, r, cmd.ActorID, cmd.TargetID)
				if err != nil {
					return err
				}
				request.RecipientDrops = map[string][]string{cmd.ActorID: {box.ItemID}}
				request.BattleResult.Outcome = corebattle.OutcomeWin
			case "exit":
				if err := s.prepareExit(r, cmd.ActorID, &request); err != nil {
					return err
				}
			}
		case "cleanup":
			if !now.After(r.LastActivity.Add(1800 * time.Second)) {
				return ErrRunConflict
			}
			if err := s.timeout(ctx, r, now); err != nil {
				return err
			}
		default:
			if s.cfg.Combat == nil {
				return ErrRunUnavailable
			}
			var err error
			effects, err = s.cfg.Combat.ExecuteRunAction(ctx, r, cmd)
			if err != nil {
				return err
			}
			if effects.EncounterDefeated {
				if r.livingEnemies() || r.EncounterFloor < 1 || r.EncounterFloor > BossFloor {
					return ErrRunConflict
				}
				r.FloorsCleared = max(r.FloorsCleared, r.EncounterFloor)
				if r.EncounterFloor == BossFloor {
					r.StageCleared = true
				}
			}
		}
		if cmd.Action != "cleanup" {
			releaseStock = (releaseStock && effects.ReleaseStock) || cmd.Action == "exit"
			if err := r.recordEarnings(effects.Earnings); err != nil {
				return err
			}
			if releaseStock {
				recipient := r.Recipients[cmd.ActorID]
				request.RecipientRewards[cmd.ActorID] = runReward(recipient.Pending)
			}
			request.BattleResult.ConsumedItems = effects.Consumed
			// Consume concrete owned instances in this transaction, never at exit again.
			for id, entries := range effects.Consumed {
				if !slices.Contains(r.Roster, id) && id != cmd.ActorID {
					return ErrRunForbidden
				}
				for _, entry := range entries {
					if entry.InstanceID == "" || entry.Quantity <= 0 {
						return ErrRunConflict
					}
				}
			}
			response, err := s.cfg.Settler.ApplyPostBattleResult(ctx, request)
			if err != nil {
				return err
			}
			if box != nil {
				boxes := []TreasureBox{*box}
				updateTreasureDeliveries(boxes, response)
				*box = boxes[0]
			}
			if releaseStock {
				recipient := r.Recipients[cmd.ActorID]
				recipient.Applied, err = addRunEarnings(recipient.Applied, recipient.Pending)
				if err != nil {
					return err
				}
				recipient.Pending = RunEarnings{}
				r.Recipients[cmd.ActorID] = recipient
			}
			if cmd.Action == "exit" {
				if s.cfg.ExitEffects == nil {
					return ErrRunUnavailable
				}
				if err := s.cfg.ExitEffects(ctx, cmd.ActorID); err != nil {
					return err
				}
				if err := s.writeHistory(ctx, r, cmd.ActorID, "exit"); err != nil {
					return err
				}
				r.Roster = slices.DeleteFunc(r.Roster, func(id string) bool { return id == cmd.ActorID })
				if r.LeaderID == cmd.ActorID {
					r.LeaderID = ""
					if len(r.Roster) > 0 {
						r.LeaderID = r.Roster[0]
					}
				}
			}
		}
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
		if err := s.cfg.Store.SaveRun(ctx, *r, cmd.Version); err != nil {
			return err
		}
		result = RunReceipt{Command: cmd, Version: r.Version, Floor: r.Floor, StageID: r.Stage.ID, Box: box, Recipient: r.Recipients[cmd.ActorID], Disposition: r.Recipients[cmd.ActorID].Disposition}
		return s.cfg.Store.SaveRunReceipt(ctx, result)
	})
	result, err = s.resolveReceipt(ctx, cmd, result, err)
	if err == nil && cmd.Action == "cleanup" {
		err = s.RecoverTimeout(ctx, cmd.PlayerID, cmd.ActorID, cmd.RunID)
	}
	return result, err
}
