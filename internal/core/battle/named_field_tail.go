package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// GraveSnapshot is captured before each operation, not when the field starts.
// Only the named helper is snapshotted; its current agility is intentional.
type GraveSnapshot struct {
	Name, Icon, TeamID string
	Stats              [5]float64
}

func (s *LifetimeState) SnapshotGrave() *GraveSnapshot {
	a := s.rosterIdentity(callingIdentity(64))
	if a == nil {
		return nil
	}
	return &GraveSnapshot{Name: a.Name, Icon: a.Icon, TeamID: a.TeamID, Stats: [5]float64{float64(a.MaxHP), float64(a.MaxMP), float64(a.BaseStats.Attack), float64(a.BaseStats.Defense), float64(a.Agility)}}
}

// NamedFieldTail runs at the explicit source boundary before lifetime ticks.
// NPC burn/grave are outside its normal-action block; library is player-only.
type NamedFieldTail struct {
	Before                           string
	NPC, ResponseEnabled, Suppressed bool
	Grave                            *GraveSnapshot
}

func (s *LifetimeState) ValidateNamedFieldTail(c *NamedFieldContext, tail NamedFieldTail, rng random.Generator) error {
	if tail.NPC {
		tail.ResponseEnabled, tail.Suppressed = true, false
	}
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if tail.Grave != nil && s.Fields.Exercises("grave", tail.Before, tail.ResponseEnabled, tail.Suppressed) {
		a := s.rosterIdentity(callingIdentity(64))
		if a == nil {
			if c.Roster == nil {
				return ErrUnsupportedEffect
			}
			roster := *c.Roster
			roster.TargetContext = c.TargetContext
			p := HelperPayload{Identity: callingIdentity(64), Name: tail.Grave.Name, Stats: tail.Grave.Stats}
			if err := s.validateHelper(&roster, p, rng); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *LifetimeState) ApplyNamedFieldTail(c *NamedFieldContext, tail NamedFieldTail, rng random.Generator) error {
	if tail.NPC {
		tail.ResponseEnabled, tail.Suppressed = true, false
	}
	if err := s.ValidateNamedFieldTail(c, tail, rng); err != nil {
		return err
	}
	if s.Fields.Exercises("burn", tail.Before, tail.ResponseEnabled, tail.Suppressed) {
		a := s.actor(c.ActorID)
		if a.HP > 0 {
			a.HP = max(1, a.HP-min(200, a.MaxHP/10))
			a.Status, a.Temporary, a.Extended, a.ExtendedTurns = "", "", "", 0
		}
	}
	if g := tail.Grave; g != nil && s.Fields.Exercises("grave", tail.Before, tail.ResponseEnabled, tail.Suppressed) {
		a := s.rosterIdentity(callingIdentity(64))
		if a == nil {
			roster := *c.Roster
			roster.TargetContext = c.TargetContext
			r, err := s.AddHelper(&roster, HelperPayload{Identity: callingIdentity(64), Name: g.Name, Stats: g.Stats}, rng)
			if err != nil {
				return err
			}
			s.actor(r.HelperID).Icon = g.Icon
			c.TargetContext = roster.TargetContext
		} else if a.HP <= 0 {
			a.HP, a.TeamID = int(g.Stats[0]), g.TeamID
			if !tail.NPC {
				a.MP = int(g.Stats[1])
			}
		}
	}
	if !tail.NPC && s.Fields.Exercises("library", tail.Before, tail.ResponseEnabled, tail.Suppressed) {
		a := &s.Actors[int(rng.Float64()*float64(len(s.Actors)))]
		if a.HP > 0 {
			a.MP = min(a.MaxMP, a.MP+int(float64(a.MaxMP)*.03*(.9+rng.Float64()*.3)))
		}
	}
	return nil
}

// ApplyIncenseHealing runs before TickIncense/CompleteOperation. The holder's
// original tension is restored even after the party helper consumes it.
func (s *LifetimeState) ApplyIncenseHealing(c *EffectContext, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	holder := ""
	for _, id := range c.PartyIDs {
		if s.actor(id).Extended == ExtendedIncense {
			holder = id
			break
		}
	}
	if holder == "" {
		return nil
	}
	child := *c
	child.ActorID = holder
	child.targetResources = func(r TargetResolution) { applyStatusResources(s, r) }
	for _, id := range c.PartyIDs {
		if err := s.validatePartyTarget(child.TargetContext, controlEffect(id, 100, CategoryUntyped), rng); err != nil {
			return err
		}
	}
	a := s.actor(holder)
	tension := a.Tension
	a.Tension = 1
	defer func() { s.actor(holder).Tension = tension }()
	_, err := s.ApplyPartyHealing(&child, 100, CategoryUntyped, rng)
	c.HealCalls, c.PartyHealCalls = child.HealCalls, child.PartyHealCalls
	return err
}

// FortressDefeat supplies the existing defeat owner with a pure conservative
// projection. Tool healing may LOWER a living participant already above MaxHP.
func FortressDefeat(rng random.Generator) *DefeatRewards {
	validate := func(s *LifetimeState, c TargetContext, id string) error {
		if err := s.validateTargetContext(c, rng); err != nil {
			return err
		}
		if s.actor(id) == nil {
			return ErrInvalidParticipant
		}
		return nil
	}
	side := func(c TargetContext, id string) []string {
		if slices.Contains(c.PartyIDs, id) {
			return c.PartyIDs
		}
		if slices.Contains(c.EnemyIDs, id) {
			return c.EnemyIDs
		}
		return nil
	}
	return &DefeatRewards{
		Validate: validate,
		Apply: func(s *LifetimeState, c TargetContext, id string) error {
			if err := validate(s, c, id); err != nil {
				return err
			}
			power := float64(s.actor(id).MaxHP) * .2
			for _, target := range side(c, id) {
				if _, err := s.ApplyToolHealing(c, target, power, false, rng); err != nil {
					return err
				}
			}
			return nil
		},
		Project: func(s *LifetimeState, c TargetContext, id string) ([]LifetimeState, error) {
			if err := validate(s, c, id); err != nil {
				return nil, err
			}
			power := float64(s.actor(id).MaxHP) * .2
			minimum := int(power * .9)
			maximum := int(power * (.9 + .3*math.Nextafter(1, 0)))
			states := []LifetimeState{s.callbackCopy()}
			for _, target := range side(c, id) {
				if s.actor(target).HP <= 0 {
					continue
				}
				var next []LifetimeState
				for _, state := range states {
					a := state.actor(target)
					low, high := min(a.MaxHP, a.HP+minimum), min(a.MaxHP, a.HP+maximum)
					// Independent tool rolls can straddle both survival and the
					// HP999 immunity boundary; a cap can also lower over-MaxHP.
					for _, hp := range []int{low, high, 999, 1000} {
						if hp < low || hp > high {
							continue
						}
						p := state.callbackCopy()
						p.actor(target).HP = hp
						next = appendDefeatState(next, p)
					}
				}
				states = next
			}
			return states, nil
		},
	}
}
