package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateNPCJob405 admits each ordered body on a private projection. Repeated
// areas share one projection; admission never calls external Apply hooks or RNG.
func (s *LifetimeState) ValidateNPCJob405(c *NPCJob405Context, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	damage := p.nativeDamageContext(child.DamageContext, child.Death, rng)
	switch id {
	case "npc-job-405-position-0", "npc-job-405-position-0-wounded":
		hits := 4
		if id == "npc-job-405-position-0-wounded" {
			hits = 5
		}
		// Enemy seals change no damage recipient, so they need no branches.
		for range hits {
			if err := p.admitAreaDamage(&damage, dualAgilityEffect("", 110, CategoryMagic, true), rng); err != nil {
				return err
			}
		}
		return nil
	case "npc-job-405-position-1":
		return p.admitNPCPartyStats(&child.EffectContext, .6, rng)
	case "npc-job-405-position-2":
		targets := []string{child.shift(), child.shift()}
		return p.validateNPCHits(&child.DamageContext, child.Death, 2, func(a *LifetimeActor, hit int) TargetEffect {
			if hit == 1 {
				return dualAgilityEffect(targets[1], float64(a.Attack)*1.3, CategoryPhysical, false)
			}
			return dualAgilityEffect(targets[0], float64(a.Defense)*.6, CategoryMagic, true)
		}, rng)
	case "npc-job-405-position-3":
		return p.validateNPC405Renewal(&child, damage, rng)
	case "npc-job-405-position-4":
		for _, target := range child.EnemyIDs {
			npc405Cleave(p.actor(target))
		}
		return p.admitAreaDamage(&damage, dualAgilityEffect("", 300, CategoryUntyped, true), rng)
	case "npc-job-405-position-4-wounded":
		// Each cleave precedes its own hit, so later enemies keep their
		// covers and reflections for earlier hits.
		for _, target := range child.EnemyIDs {
			npc405Cleave(p.actor(target))
			if err := p.admitDamageSequence(&damage, []TargetEffect{dualAgilityEffect(target, 799, CategoryUntyped, true)}, rng); err != nil {
				return err
			}
		}
		return nil
	case "npc-job-405-position-5":
		a := p.actor(child.ActorID)
		power := float64(a.Attack+a.Defense+a.Agility)*.7 + float64(200*p.npc405Members(child.ActorID))
		states, err := p.projectCallbackDamage(&child.DamageContext, child.Death, dualAgilityEffect(child.shift(), power, CategoryUntyped, false), power, true, rng)
		if err != nil {
			return err
		}
		for _, next := range states {
			if err := next.State.ValidateStatDown(&next.Context.EffectContext, controlEffect(next.TargetID, .3, CategoryUntyped), StatCMP, rng); err != nil {
				return err
			}
		}
		return nil
	case "npc-job-405-position-5-wounded":
		// Unspecified-target candidates cover every living enemy the chain can draw.
		for range max(1, len(child.EnemyIDs)) {
			if err := p.admitDamageSequence(&damage, []TargetEffect{dualAgilityEffect("", 1, CategoryUntyped, false)}, rng); err != nil {
				return err
			}
			if err := p.ValidateStatDown(&damage.EffectContext, controlEffect("", .5, CategoryUntyped), StatCMP, rng); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrUnsupportedEffect
}

func (p *LifetimeState) validateNPC405Renewal(c *NPCJob405Context, damage DamageContext, rng random.Generator) error {
	if err := p.validatePartyTarget(c.TargetContext, controlEffect(c.ActorID, 1000, CategoryUntyped), rng); err != nil {
		return err
	}
	swapped := npc405Opponents(c.EffectContext)
	for _, id := range swapped.PartyIDs {
		if err := p.validatePartyTarget(swapped.TargetContext, controlEffect(id, 150, CategoryMagic), rng); err != nil {
			return err
		}
	}
	if _, err := p.gemTensionCeiling(&c.EffectContext, p.actor(c.ActorID), 1); err != nil {
		return err
	}
	p.actor(c.ActorID).Extended = ExtendedPurification
	// The field draw either keeps the current field or installs an anti-field.
	anti := p.callbackCopy()
	anti.Fields.CreateAnti(1)
	for _, state := range []*LifetimeState{p, &anti} {
		context := damage
		for _, id := range c.PartyIDs {
			if id == c.ActorID {
				continue
			}
			if err := state.admitAreaDamage(&context, dualAgilityEffect("", float64(state.actor(id).HP)*1.5, CategoryUntyped, true), rng); err != nil {
				return err
			}
			state.actor(id).HP = 0
		}
	}
	return nil
}
