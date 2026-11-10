package adventure

import (
	"context"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/economy"
)

func (s *NormalRunService) prepareExit(r *NormalRun, actor string, request *battle.ApplyPostBattleRequest) error {
	a := r.actor(actor)
	if a == nil {
		return ErrNotParticipant
	}
	p := r.Recipients[actor]
	if p.Disposition != "" {
		return ErrRunConflict
	}
	// Ordinary exit forfeits only unfinished Try stock; unconditional earnings survive.
	p.Conditional = RunEarnings{}
	bonus := RunEarnings{}
	if a.Gear != nil && slices.Contains(a.ItemDefinitionIDs, a.Gear.AccessoryID) {
		switch a.Gear.AccessoryID {
		case "item-105":
			var err error
			bonus.EXP, err = economy.SafeMultiply(r.Floor, 20)
			if err != nil {
				return err
			}
		case "item-106":
			bonus.Gold = int(float64(r.Floor) * s.cfg.RNG.Float64() * 77)
		}
	}
	var err error
	p.Pending, err = addRunEarnings(p.Pending, bonus)
	if err != nil {
		return err
	}
	p.Disposition = "exit"
	r.Recipients[actor] = p
	request.CharacterIDs = []string{actor}
	request.ExitFatigue = CalculateExitFatigue(r.Floor, r.Mode)
	request.BattleResult.RemainingHP = map[string]int{actor: a.HP}
	request.BattleResult.RemainingMP = map[string]int{actor: a.MP}
	return nil
}

func (s *NormalRunService) writeHistory(ctx context.Context, r *NormalRun, actor, disposition string) error {
	p := r.Recipients[actor]
	outcome := corebattle.OutcomeDraw
	winner := ""
	if disposition == "timeout" {
		outcome = corebattle.Outcome("timeout")
	} else if a := r.actor(actor); a != nil && a.HP <= 0 {
		outcome = corebattle.OutcomeDefeat
	} else if r.StageCleared {
		outcome, winner = corebattle.OutcomeWin, actor
	}
	return s.cfg.History.Save(ctx, Adventure{ID: normalRunID(r.ID + ":" + actor), CharacterID: actor, Type: r.Admission.StageID, StageID: r.Admission.StageID, Season: r.RecordSeason, SeasonalSubID: SeasonToSubID(r.RecordSeason), StartedAt: r.StartedAt, FloorsCleared: r.FloorsCleared, IsCleared: r.StageCleared, PartySize: len(r.Admission.CharacterIDs), ExperienceReward: p.Applied.EXP, Resolved: true, BattleResult: corebattle.Result{Outcome: outcome, WinnerID: winner, Reward: runReward(p.Applied)}})
}

func (s *NormalRunService) timeout(ctx context.Context, r *NormalRun, now time.Time) error {
	// Command already holds the character locks; preserve history write order.
	ids := slices.Clone(r.Roster)
	slices.Sort(ids)
	for _, id := range ids {
		p := r.Recipients[id]
		p.Disposition, p.TimerPending, p.PenaltyUntil = "timeout", true, now.Add(time.Hour)
		r.Recipients[id] = p
		c, err := s.cfg.Characters.FindByID(ctx, id)
		if err != nil {
			return err
		}
		c.PendingWake = true
		if err := s.cfg.Characters.Update(ctx, c); err != nil {
			return err
		}
		if err := s.writeHistory(ctx, r, id, "timeout"); err != nil {
			return err
		}
	}
	r.TimedOut = true
	return nil
}

// RecoverTimeout projects fixed deadlines and releases only completed ownership.
// It never restarts a penalty or treats timeout as personal exit.
// Transaction: WithRunForUpdate. Lock Order: run(0) -> characters(2, sorted) -> recipients/claims(8).
func (s *NormalRunService) RecoverTimeout(ctx context.Context, player, actor, runID string) error {
	if err := s.authorize(ctx, player, actor); err != nil {
		return err
	}
	return s.cfg.Store.WithRunForUpdate(ctx, runID, false, func(ctx context.Context) error {
		r, err := s.cfg.Store.FindRun(ctx, runID)
		if err != nil {
			return err
		}
		if r == nil {
			return nil
		}
		p, ok := r.Recipients[actor]
		if !ok || p.PlayerID != player {
			return ErrRunForbidden
		}
		if !r.TimedOut {
			return ErrRunConflict
		}
		var ids []string
		ctx = context.WithValue(ctx, normalMutationContext{}, r.ID)
		if err := economy.LockInOrder(ctx, r.Roster, func(ctx context.Context, id string) error {
			if _, err := s.cfg.Characters.FindByIDForUpdate(ctx, id); err != nil {
				return err
			}
			ids = append(ids, id)
			return nil
		}); err != nil {
			return err
		}
		for _, id := range ids {
			p := r.Recipients[id]
			if !p.TimerPending {
				continue
			}
			if err := s.cfg.Penalty.ProjectRunPenalty(ctx, id, p.PenaltyUntil); err != nil {
				return err
			}
			p.TimerPending = false
			r.Recipients[id] = p
			if err := s.cfg.Store.SaveRunRecipient(ctx, r.ID, id, p); err != nil {
				return err
			}
		}
		version := r.Version
		r.Version++
		r.Roster, r.LeaderID = nil, ""
		return s.cfg.Store.SaveRun(ctx, *r, version)
	})
}

func (s *NormalRunService) GetActorPenalty(ctx context.Context, player, actor string) (*ActorRun, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return nil, err
	}
	facts, err := s.cfg.Store.FindPenalty(ctx, actor)
	if err != nil || facts == nil {
		return facts, err
	}
	if facts.Recipient.PlayerID != player {
		return nil, ErrRunForbidden
	}
	return facts, nil
}
