package runhost

import (
	"context"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/economy"
)

// prepareExit is nigeru's settlement input (_battle.cgi:1184): the actor's own
// HP/MP, the room's exit bonus and fatigue.
func (s *Service[R, P]) prepareExit(run *R, actor string, request *battle.ApplyPostBattleRequest) error {
	r := P(run).Host()
	a := r.Actor(actor)
	if a == nil {
		return ErrNotParticipant
	}
	p := r.Recipients[actor]
	if p.Disposition != "" {
		return ErrConflict
	}
	// Ordinary exit forfeits only unfinished conditional stock; unconditional earnings survive.
	p.Conditional = Earnings{}
	bonus, fatigue, err := s.cfg.Room.Exit(run, actor)
	if err != nil {
		return err
	}
	p.Pending, err = AddEarnings(p.Pending, bonus)
	if err != nil {
		return err
	}
	p.Disposition = "exit"
	r.Recipients[actor] = p
	request.CharacterIDs = []string{actor}
	request.ExitFatigue = fatigue
	request.BattleResult.RemainingHP = map[string]int{actor: a.HP}
	request.BattleResult.RemainingMP = map[string]int{actor: a.MP}
	return nil
}

// finishExit runs the exit effects and record after settlement, then removes
// the actor and hands leadership to the next roster member.
func (s *Service[R, P]) finishExit(ctx context.Context, run *R, actor string) error {
	if s.cfg.ExitEffects == nil {
		return ErrUnavailable
	}
	if err := s.cfg.ExitEffects(ctx, actor); err != nil {
		return err
	}
	if err := s.cfg.Room.Record(ctx, run, actor, "exit"); err != nil {
		return err
	}
	r := P(run).Host()
	r.Roster = slices.DeleteFunc(r.Roster, func(id string) bool { return id == actor })
	r.Battle.Actors = slices.DeleteFunc(r.Battle.Actors, func(a corebattle.LifetimeActor) bool { return a.ID == actor })
	if r.LeaderID == actor {
		r.LeaderID = ""
		if len(r.Roster) > 0 {
			r.LeaderID = r.Roster[0]
		}
	}
	// The last exit's deletion check (_battle.cgi:1331); SaveRun disposes an unretained empty run.
	if len(r.Roster) == 0 {
		r.Retained = s.cfg.SystemRoom != nil && s.cfg.SystemRoom.Retain(run)
	}
	return nil
}

func (s *Service[R, P]) timeout(ctx context.Context, run *R, now time.Time) error {
	r := P(run).Host()
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
		if err := s.cfg.Room.Record(ctx, run, id, "timeout"); err != nil {
			return err
		}
	}
	// Idle deletion removes a room in any state (quest.cgi auto_delete_quest).
	r.TimedOut, r.Retained = true, false
	return nil
}

// RecoverTimeout projects fixed deadlines and releases only completed ownership.
// It never restarts a penalty or treats timeout as personal exit.
// Transaction: WithRunForUpdate. Lock Order: run(0) -> characters(2, sorted) -> recipients/claims(8).
func (s *Service[R, P]) RecoverTimeout(ctx context.Context, player, actor, runID string) error {
	if err := s.authorize(ctx, player, actor); err != nil {
		return err
	}
	return s.recoverTimeout(ctx, runID, func(r *State) error {
		if p, ok := r.Recipients[actor]; !ok || p.PlayerID != player {
			return ErrForbidden
		}
		return nil
	})
}

// recoverTimeout is RecoverTimeout after authorization; the idle sweep passes a nil owner check.
func (s *Service[R, P]) recoverTimeout(ctx context.Context, runID string, owner func(*State) error) error {
	return s.cfg.Store.WithRunForUpdate(ctx, runID, false, func(ctx context.Context) error {
		run, err := s.cfg.Store.FindRun(ctx, runID)
		if err != nil {
			return err
		}
		if run == nil {
			return nil
		}
		r := P(run).Host()
		if owner != nil {
			if err := owner(r); err != nil {
				return err
			}
		}
		if !r.TimedOut {
			return ErrConflict
		}
		var ids []string
		ctx = commandContext(ctx, r.ID)
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
		return s.cfg.Store.SaveRun(ctx, *run, version)
	})
}

func (s *Service[R, P]) GetActorPenalty(ctx context.Context, player, actor string) (*ActorRun, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return nil, err
	}
	facts, err := s.cfg.Store.FindPenalty(ctx, actor)
	if err != nil || facts == nil {
		return facts, err
	}
	if facts.Recipient.PlayerID != player {
		return nil, ErrForbidden
	}
	return facts, nil
}

// banish removes the members a room NPC banished in this command (vs_king.cgi
// dejon): no exit settlement and no deletion check. Unreleased earnings stay
// in the recipient ledger as retained earnings.
func (s *Service[R, P]) banish(ctx context.Context, run *R) ([]string, error) {
	r := P(run).Host()
	var gone []string
	for _, id := range r.Roster {
		if a := r.Actor(id); a != nil && a.Banished {
			gone = append(gone, id)
		}
	}
	if len(gone) == 0 {
		return nil, nil
	}
	if s.cfg.SystemRoom == nil {
		return nil, ErrConflict
	}
	for _, id := range gone {
		c, err := s.cfg.Characters.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := s.cfg.SystemRoom.Banish(run, &c); err != nil {
			return nil, err
		}
		if err := s.cfg.Characters.Update(ctx, c); err != nil {
			return nil, err
		}
		p := r.Recipients[id]
		p.Disposition, p.Conditional = "dejon", Earnings{}
		r.Recipients[id] = p
		delete(r.LastActions, id)
	}
	r.Roster = slices.DeleteFunc(r.Roster, func(id string) bool { return slices.Contains(gone, id) })
	r.Battle.Actors = slices.DeleteFunc(r.Battle.Actors, func(a corebattle.LifetimeActor) bool { return slices.Contains(gone, a.ID) })
	return gone, nil
}
