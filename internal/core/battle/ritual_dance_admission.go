package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateRitualDance admits complete descendants on private facts before any
// costs, arguments, counters, external hooks or committed RNG can change.
func (s *LifetimeState) ValidateRitualDance(c *RitualDanceContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	control := ControlClimaxContext{DamageContext: child.DamageContext}
	switch id {
	case "job-75-entry-0":
		return p.validateControlStatuses(&control, []string{StatusMagicSeal, StatusPhysicalSeal, StatusKinju}, CategoryMagic, 75, rng)
	case "job-75-entry-1":
		_, err := p.controlTargets(&control, controlEffect(child.shift(), 0, CategoryUntyped), false)
		return err
	case "job-75-entry-2":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "accuracy", CategoryUntyped), rng)
	case "job-75-entry-3":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "cure", CategoryMagic), rng)
	case "job-75-entry-4":
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 1, CategoryUntyped), rng)
	case "job-75-entry-6":
		_, err := p.controlTargets(&control, controlSymbol(child.shift(), "forbidden", CategoryMagic), false)
		return err
	case "job-76-entry-0":
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&control, controlEffect(target, .2, CategoryMagic), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-76-entry-1":
		return p.ValidateAreaStatus(&StatusContext{EffectContext: child.EffectContext}, controlSymbol("", StatusSleep, CategorySong), 50, false, false, rng)
	case "job-76-entry-2":
		target := child.shift()
		for _, power := range []float64{150, 350} {
			if err := p.ValidateDamage(&child.DamageContext, dualAgilityEffect(target, power, CategoryMagic, true), rng); err != nil {
				return err
			}
		}
		// A successful first category cure and target195 activation precede
		// the second party-target helper. Its explicit self target is retained.
		if p.actor(child.ActorID).categorySealed(CategoryMagic) {
			p.actor(child.ActorID).Status = ""
		}
		child.Item195Activated = true
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 1, CategoryMagic), rng)
	case "job-76-entry-4":
		e := controlSymbol(child.shift(), "cure", CategoryUntyped)
		if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
			return err
		}
		for _, candidate := range p.partyTargetCandidates(child.TargetContext, e, nil) {
			if err := p.validatePartyTarget(child.TargetContext, controlSymbol(candidate.TargetID, "cure", CategoryUntyped), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-76-entry-5":
		return p.validateGroupDance(&child, p.actor(child.shift()) != nil, rng)
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateGroupDance(c *RitualDanceContext, supplied bool, rng random.Generator) error {
	// The target owner retains original and reset alternatives, including loss
	// of the first covering member and selection of a later covering member.
	mayReset := map[string]float64{}
	mayRevive := false
	for _, target := range c.PartyIDs {
		a := s.actor(target)
		if a.HP <= 0 && !mayRevive {
			continue
		}
		a.HP = max(1, a.HP)
		c.ActorID, c.ActorNPC = target, a.NPC
		tension := a.Tension
		if !supplied {
			a.Tension = 1
		}
		reset, blocked := map[string]bool{}, map[string]bool{}
		activate := false
		for _, direct := range []bool{true, false} {
			e := groupDanceEffect(a, supplied, direct)
			if err := s.ValidateDamage(&c.DamageContext, e, rng); err != nil {
				return err
			}
			converted, _, err := a.ConvertEnemyCategory(c.TargetContext, e)
			if err != nil {
				return err
			}
			if len(c.EnemyIDs) == 0 || converted.Payload.Number == 0 {
				continue
			}
			q := s.callbackCopy()
			for id := range mayReset {
				q.actor(id).Temporary, q.actor(id).Status = "", ""
			}
			candidates := s.damageCandidatesAfter(&c.DamageContext, converted, mayReset)
			// A reset of the current actor can remove phantom/confusion/seals
			// before this helper's category and target checks.
			candidates = append(candidates, q.damageCandidatesAfter(&c.DamageContext, converted, mayReset)...)
			if err := q.validateDamageRecipients(&c.DamageContext, candidates); err != nil {
				return err
			}
			for fallen := range mayReset {
				p := q.callbackCopy()
				p.actor(fallen).HP = 0
				if err := p.validateDamageRecipients(&c.DamageContext, candidates); err != nil {
					return err
				}
			}
			for _, candidate := range candidates {
				t := q.actor(candidate.TargetID)
				if t.HP <= 0 {
					t = q.actor(c.ActorID)
				}
				if candidate.Payload.Number == 0 {
					continue
				}
				if t.Temporary == TemporaryOneHitDefense {
					blocked[t.ID] = true
				} else if damageHasPower(q.actor(c.ActorID), t, candidate) {
					if q.groupDanceMayReset(&c.DamageContext, t, candidate) {
						reset[t.ID] = true
					}
					activate = activate || hasItem(t.ItemDefinitionIDs, "item-195")
					if mayReset[t.ID] > 0 || t.Extended == ExtendedInterception && t.ID != c.ActorID {
						reset[c.ActorID] = true // A fallen recipient redirects damage to the actor.
					}
				}
			}
		}
		// Direct/ordinary are alternatives, with one helper per member. Carry
		// possible defense consumption and activation only to the next helper.
		e := groupDanceEffect(a, supplied, false)
		if err := s.admitDamageSequence(&c.DamageContext, []TargetEffect{e}, rng); err != nil {
			return err
		}
		for id := range blocked {
			s.actor(id).Temporary = ""
		}
		for id := range reset {
			mayReset[id], mayRevive = 1, true
		}
		c.Item195Activated = c.Item195Activated || activate
		if !supplied {
			a.Tension = tension
		}
	}
	return nil
}

func (s *LifetimeState) groupDanceMayReset(c *DamageContext, t *LifetimeActor, e TargetEffect) bool {
	bounded := *s
	bounded.Actors = slices.Clone(s.Actors)
	s = &bounded
	a := s.actor(c.ActorID)
	if a.Extended == ExtendedPurification || hasAbility(a.Abilities, "seal_kuu") {
		return true
	}
	child := *c
	child.DamageCalls = max(4, child.DamageCalls+1) // Upper bound for239 after preceding helpers.
	if hasItem(a.ItemDefinitionIDs, "weapon-68") && a.MaxHP > 0 {
		a.HP = 1 // An earlier hit can enable the low-HP weapon multiplier.
	}
	direct, normal := damageFlags(e)
	minus := toolDefense(t)
	if hasItem(a.ItemDefinitionIDs, "item-187") {
		minus -= float64(t.Defense-t.BaseStats.Defense) * .3
	}
	rollDirect := direct || e.Category == CategoryDance && hasItem(a.ItemDefinitionIDs, "item-190")
	// The damage modifier has at most four optional draws; their endpoint
	// combinations bound every boost/reduction without sampling committed RNG.
	for mask := range 16 {
		bits := mask
		draw := func() float64 { v := float64(bits&1) * math.Nextafter(1, 0); bits >>= 1; return v }
		power := s.modifyDamage(&child, t, e.Payload.Number, e.Category, direct, normal, draw)
		if !hasItem(a.ItemDefinitionIDs, "item-149") {
			power *= a.Tension
		}
		if t.HP <= max(2, rollDamage(power, minus, rollDirect, t.BaseStats.Defense, resetBoundary(math.Nextafter(1, 0)))) {
			return true
		}
	}
	return false
}
