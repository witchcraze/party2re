package battle

import (
	"math"

	"github.com/witchcraze/party2re/internal/core/random"
)

// A strict total-damage bound avoids enumerating HP permutations when every
// recipient must survive. Stateful reset/interception/confusion paths still use
// ordered projection.208's remaining heal/tension admission needs no exact HP.
func (s *LifetimeState) equipment208Nonlethal(c *EquipmentTailContext, count int, rng random.Generator) (bool, error) {
	a := s.actor(c.ActorID)
	if a.Extended == ExtendedPurification || hasAbility(a.Abilities, "seal_kuu") || hasItem(a.ItemDefinitionIDs, "weapon-68") || c.Item195Activated {
		return false, nil
	}
	for _, target := range s.Actors {
		if target.Extended == ExtendedInterception || hasItem(target.ItemDefinitionIDs, "item-195") {
			return false, nil
		}
	}
	p := s.callbackCopy()
	// Removing every one-use shield is an upper bound, not an execution path.
	for i := range p.Actors {
		p.Actors[i].ConsumeOneHitDefense()
	}
	damage := p.nativeDamageContext(c.DamageContext, c.Defeat, rng)
	totals := make(map[string]int)
	for hit := range count {
		damage.DamageCalls = c.DamageCalls + hit
		e := dualAgilityEffect("", float64(a.Attack)*.1, CategoryUntyped, true)
		if err := p.ValidateDamage(&damage, e, rng); err != nil {
			return false, err
		}
		converted, _, err := a.ConvertEnemyCategory(c.TargetContext, e)
		if err != nil {
			return false, err
		}
		damage.DamageCalls++
		maximum := make(map[string]int)
		for _, candidate := range p.damageCandidates(&damage, converted) {
			if candidate.Payload.Number == 0 {
				continue
			}
			t := p.actor(candidate.TargetID)
			if t.HP <= 0 || t.Gear == nil {
				return false, nil
			}
			_, high := p.callbackPowerBounds(&damage, t, candidate, candidate.Payload.Number)
			if high == 0 {
				continue
			}
			minus := toolDefense(t)
			if hasItem(a.ItemDefinitionIDs, "item-187") {
				minus -= float64(t.Defense-t.BaseStats.Defense) * .3
			}
			direct, _ := damageFlags(candidate)
			amount := max(2, rollDamage(high, minus, direct, t.BaseStats.Defense, resetBoundary(math.Nextafter(1, 0))))
			maximum[t.ID] = max(maximum[t.ID], amount)
		}
		for id, amount := range maximum {
			// Compare before addition so a large power cannot overflow the bound.
			if p.actor(id).HP-totals[id] <= amount {
				return false, nil
			}
			totals[id] += amount
		}
	}
	return true, nil
}
