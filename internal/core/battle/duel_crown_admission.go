package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateDuelCrown admits complete descendants on private state, including
// later hits after defense consumption and the duel's temporarily swapped sides.
func (s *LifetimeState) ValidateDuelCrown(c *DuelCrownContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for _, entry := range DuelCrownEntries() {
		known = known || entry.ID == id
	}
	if !known {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	switch id {
	case "job-74-entry-0":
		for i := range p.Actors {
			t := &p.Actors[i]
			if t.HP <= 0 || t.ID == c.ActorID {
				continue
			}
			if _, err := p.gemTensionCeiling(&child.EffectContext, t, 1); err != nil {
				return err
			}
		}
	case "job-74-entry-1", "job-74-entry-4":
		return nil // Direct writes bypass ordinary target/status/revival guards.
	case "job-74-entry-2":
		return p.validateDuel(&child, rng)
	case "job-74-entry-3":
		return p.validateAreaDamage(&child.DamageContext, TargetEffect{Payload: CallbackPayload{Number: 150}, Arguments: []string{"1"}}, rng)
	case "job-74-entry-5":
		return p.validateCrownFlurry(&child, rng)
	}
	return nil
}

func (s *LifetimeState) duelCandidates(c *DuelCrownContext, target string) ([]string, error) {
	e, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, TargetEffect{TargetID: target, Payload: CallbackPayload{Symbol: "duel"}})
	if err != nil {
		return nil, err
	}
	var ids []string
	if len(c.EnemyIDs) > 0 {
		for _, candidate := range s.damageCandidates(&c.DamageContext, e) {
			if !slices.Contains(ids, candidate.TargetID) {
				ids = append(ids, candidate.TargetID)
			}
		}
	}
	// Fallen random selections and an empty enemy roster reach the member fallback.
	fallback := len(c.EnemyIDs) == 0
	a, t := s.actor(c.ActorID), s.actor(target)
	if a.Status != StatusConfusion {
		if t != nil && t.TeamID != a.TeamID {
			fallback = fallback || t.HP <= 0
		} else {
			for _, id := range c.EnemyIDs {
				fallback = fallback || s.actor(id).HP <= 0
			}
		}
	}
	if fallback {
		for i := range max(1, len(c.EnemyIDs)) {
			if !slices.Contains(ids, s.Actors[i].ID) {
				ids = append(ids, s.Actors[i].ID)
			}
		}
	}
	return ids, nil
}

func (s *LifetimeState) validateDuel(c *DuelCrownContext, rng random.Generator) error {
	targets, err := s.duelCandidates(c, c.shift())
	if err != nil {
		return err
	}
	for _, target := range targets {
		p, child := s.callbackCopy(), *c
		actor := child.ActorID
		p.actor(actor).Attack = int(float64(p.actor(actor).Attack) * 1.1)
		for range 2 {
			if err := p.admitDuelStrike(&child.DamageContext, target, rng); err != nil {
				return err
			}
			if p.actor(actor).HP <= 0 || p.actor(target).HP <= 0 {
				break
			}
			original, npc := child.TargetContext, child.ActorNPC
			child.TargetContext, child.ActorNPC = duelReplyContext(original, target), p.actor(target).NPC
			err := p.admitDuelStrike(&child.DamageContext, actor, rng)
			original.Item195Activated = child.Item195Activated
			child.TargetContext, child.ActorNPC = original, npc
			if err != nil {
				return err
			}
		}
		p.actor(actor).Attack = s.actor(actor).Attack
		if p.actor(actor).HP > 0 {
			if err := p.ValidateStatUp(&child.EffectContext, TargetEffect{TargetID: actor, Payload: CallbackPayload{Number: .2}}, StatAttack, rng); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *LifetimeState) admitDuelStrike(c *DamageContext, target string, rng random.Generator) error {
	if len(c.EnemyIDs) == 0 {
		return nil
	}
	a := s.actor(c.ActorID)
	if t := s.actor(target); t == nil || t.TeamID == a.TeamID {
		target = ""
	}
	hits := 1
	if hasItem(a.ItemDefinitionIDs, "item-147") && !c.ActorNPC {
		for _, id := range s.damageInitialTargetIDs(c, TargetEffect{TargetID: target}) {
			if s.actor(id).HP > 0 {
				hits = 2
				break
			}
		}
	}
	e := TargetEffect{TargetID: target, Category: CategoryPhysical, Payload: CallbackPayload{Number: float64(a.Attack)}, Arguments: []string{"0", "1"}}
	for i := range hits {
		if i > 0 {
			e.Arguments = nil
		}
		if err := s.admitDamageSequence(c, []TargetEffect{e}, rng); err != nil {
			return err
		}
	}
	return nil
}

func (s *LifetimeState) validateCrownFlurry(c *DuelCrownContext, rng random.Generator) error {
	// Recoil's optional cure can change both attack power and target selection.
	// Keep the uncured path as well as the earliest reachable cured path.
	for _, cure := range []bool{false, true} {
		p, child := s.callbackCopy(), *c
		if err := p.admitCrownFlurry(&child, cure, rng); err != nil {
			return err
		}
		if a := s.actor(c.ActorID); !hasItem(a.ItemDefinitionIDs, "item-160") || a.Status == "" {
			break
		}
	}
	return nil
}

func (s *LifetimeState) crownHitCanReturn(c *DuelCrownContext, e TargetEffect) (bool, error) {
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return false, err
	}
	if len(c.EnemyIDs) == 0 {
		return false, nil
	}
	for _, candidate := range s.damageCandidates(&c.DamageContext, converted) {
		t := s.actor(candidate.TargetID)
		if t.HP <= 0 {
			t = s.actor(c.ActorID)
		}
		if candidate.Payload.Number != 0 && t.Temporary != TemporaryOneHitDefense && damageHasPower(s.actor(c.ActorID), t, candidate) {
			return true, nil
		}
	}
	return false, nil
}

func (s *LifetimeState) admitCrownFlurry(c *DuelCrownContext, cure bool, rng random.Generator) error {
	a := s.actor(c.ActorID)
	if a.HP <= 0 {
		return nil
	}
	target := ""
	hits := 6
	if a.Extended == ExtendedCrown {
		target = c.shift()
		// A defeat/reset can remove the crown before the first three tails.
		hits = 9
	}
	coefficient := .9
	for hit := 0; hit < hits; hit++ {
		e := TargetEffect{TargetID: target, Category: CategoryPhysical, Payload: CallbackPayload{Number: float64(a.Attack) * coefficient}}
		returned, err := s.crownHitCanReturn(c, e)
		if err != nil {
			return err
		}
		if err := s.admitDamageSequence(&c.DamageContext, []TargetEffect{e}, rng); err != nil {
			return err
		}
		if returned {
			p, child := s.callbackCopy(), *c
			if _, err := p.ApplyRecoil(&child.EffectContext, &child.End, float64(a.Attack)*.075*coefficient*1.1, resetBoundary(math.Nextafter(1, 0))); err != nil {
				return err
			}
			// A successful item160 cure can raise AT and unlock later damage.
			if cure && hasItem(a.ItemDefinitionIDs, "item-160") && a.Status != "" {
				a.Status = ""
				a.Attack = max(a.Attack, min(999, a.Attack+int(float64(a.BaseStats.Attack)*.7)))
			}
		}
		coefficient *= 1.1
	}
	return nil
}
