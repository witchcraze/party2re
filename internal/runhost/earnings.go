package runhost

import (
	"context"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/economy"
)

func AddEarnings(a, b Earnings) (Earnings, error) {
	if b.EXP < 0 || b.Gold < 0 || b.Crystals < 0 {
		return Earnings{}, ErrConflict
	}
	var err error
	a.EXP, err = economy.SafeAdd(a.EXP, b.EXP)
	if err != nil {
		return Earnings{}, err
	}
	a.Gold, err = economy.SafeAdd(a.Gold, b.Gold)
	if err != nil {
		return Earnings{}, err
	}
	a.Crystals, err = economy.SafeAdd(a.Crystals, b.Crystals)
	return a, err
}

// RecordEarnings credits new earnings to the roster, conditionally or pending.
func (r *State) RecordEarnings(earned map[string]Earnings, conditional bool) error {
	for id, e := range earned {
		if !slices.Contains(r.Roster, id) {
			return ErrForbidden
		}
		p := r.Recipients[id]
		var err error
		if conditional {
			p.Conditional, err = AddEarnings(p.Conditional, e)
		} else {
			p.Pending, err = AddEarnings(p.Pending, e)
		}
		if err != nil {
			return err
		}
		r.Recipients[id] = p
	}
	return nil
}

// RetainedDue adds the actor's disposed-timeout balances to the stock being
// released by the current command. Idle cleanup retains unpaid earnings instead
// of paying them, so the next eligible stock release is their only settlement.
func (s *Service[R, P]) RetainedDue(ctx context.Context, actor string, pending Earnings) ([]RetainedEarnings, Earnings, error) {
	retained, err := s.cfg.Store.FindRetainedEarnings(ctx, actor)
	if err != nil {
		return nil, Earnings{}, err
	}
	due := pending
	for _, entry := range retained {
		if due, err = AddEarnings(due, entry.Recipient.Pending); err != nil {
			return nil, Earnings{}, err
		}
	}
	return retained, due, nil
}

// settleRetained moves paid balances to Applied in the settlement transaction,
// after the settler (Rank 8 follows Ranks 3/4/5) and before commit.
// Conditional stock stays in Conditional, and the committed penalty is not rewritten.
func (s *Service[R, P]) settleRetained(ctx context.Context, actor string, retained []RetainedEarnings) error {
	for _, entry := range retained {
		p := entry.Recipient
		credited := p.Pending
		var err error
		if p.Applied, err = AddEarnings(p.Applied, p.Pending); err != nil {
			return err
		}
		p.Pending = Earnings{}
		if err := s.cfg.Store.SaveRetainedRecipient(ctx, entry.RunID, actor, credited, p); err != nil {
			return err
		}
	}
	return nil
}

// LevelUpDue is the player step's branch input. released is any already
// pending (including retained) stock the operation releases; earned is the
// operation's own unconditional EXP. Conditional stock never counts.
func LevelUpDue(s *corebattle.LifetimeState, requester corecharacter.Character, conditional bool, released, earned int) (bool, error) {
	threshold, err := progression.ExperienceForNextLevelWithMax(requester.Level, progression.MaxLevelForCharacter(&requester))
	if s.SpecialBossField || err != nil {
		return false, nil
	}
	exp := requester.Experience
	if !conditional {
		exp, err = economy.SafeAdd(exp, released)
		if err != nil {
			return false, err
		}
		exp, err = economy.SafeAdd(exp, earned)
	}
	return exp >= threshold && err == nil, err
}

// tailLevel is the action tail's lv_up reached by room actions and treasure
// inspection: at most one level from saved and released EXP (_battle.cgi:912,935–937).
func (s *Service[R, P]) tailLevel(ctx context.Context, r *State, requester corecharacter.Character, released int, conditional bool) error {
	due, err := LevelUpDue(&r.Battle, requester, conditional, released, 0)
	if err != nil || !due {
		return err
	}
	if s.cfg.Jobs == nil {
		return ErrUnavailable
	}
	def, err := s.cfg.Jobs.FindByID(requester.JobID)
	if err != nil {
		return err
	}
	grown, err := GrowRequester(&r.Battle, requester.ID, &requester, func(c *corecharacter.Character, held string) ([]corecharacter.Stats, error) {
		return progression.ApplyTailLevel(c, def, s.cfg.RNG, held, released)
	})
	if err != nil || !grown {
		return err
	}
	return s.cfg.Characters.Update(ctx, requester)
}

// GrowRequester applies saved-EXP stock levels to the requester and mirrors the
// uncapped growth onto the battle actor (system.cgi:49–67).
func GrowRequester(b *corebattle.LifetimeState, actorID string, requester *corecharacter.Character, apply func(*corecharacter.Character, string) ([]corecharacter.Stats, error)) (bool, error) {
	i := slices.IndexFunc(b.Actors, func(a corebattle.LifetimeActor) bool { return a.ID == actorID })
	if i < 0 {
		return false, ErrConflict
	}
	a := &b.Actors[i]
	held := ""
	if a.HeldToolID != nil {
		held = *a.HeldToolID
	} else if a.Gear != nil {
		held = a.Gear.AccessoryID
	}
	startingLevel, startingSP := requester.Level, requester.SP
	growth, err := apply(requester, held)
	if err != nil {
		return false, err
	}
	cappedLevels := 0
	if requester.OverLevel {
		cappedLevels = max(0, progression.MaxLevel-startingLevel)
	}
	a.ApplyLevelGrowth(growth, requester.SP-startingSP, cappedLevels)
	return len(growth) > 0, nil
}
