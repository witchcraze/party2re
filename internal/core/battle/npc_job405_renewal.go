package battle

import "github.com/witchcraze/party2re/internal/core/random"

// npc405Opponents heals the opposing side through the party helpers' own target
// rules: each call draws a random opponent and a fallen draw heals the caster.
func npc405Opponents(c EffectContext) EffectContext {
	c.PartyIDs, c.EnemyIDs = c.EnemyIDs, c.PartyIDs
	c.sideSwapOriginID = c.ActorID
	return c
}

func (s *LifetimeState) applyNPC405Renewal(c *NPCJob405Context, rng random.Generator) error {
	if _, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, 1000, CategoryUntyped), rng); err != nil {
		return err
	}
	swapped, origin := npc405Opponents(c.EffectContext), c.sideSwapOriginID
	restore := func() {
		swapped.PartyIDs, swapped.EnemyIDs, swapped.sideSwapOriginID = swapped.EnemyIDs, swapped.PartyIDs, origin
		c.EffectContext = swapped
	}
	if _, err := s.ApplyPartyHealing(&swapped, 150, CategoryMagic, rng); err != nil {
		restore()
		return err
	}
	// The party MP helper checks the seal once before its per-target calls.
	allowed, err := s.actor(c.ActorID).CheckCategorySeal(CategoryMagic, rng)
	if err == nil && allowed {
		for _, id := range swapped.PartyIDs {
			if _, err = s.ApplyMPHealing(&swapped, controlEffect(id, 100, CategoryMagic), rng); err != nil {
				break
			}
		}
	}
	restore()
	if err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	a.Extended, a.ExtendedTurns = ExtendedPurification, 7
	if rng.Float64() < .5 {
		if err := s.statTension(&c.EffectContext, a, false, rng); err != nil {
			return err
		}
	}
	s.npc405Field(6 + int(rng.Float64()*8))
	for _, id := range c.PartyIDs {
		ally := s.actor(id)
		if id == c.ActorID {
			continue
		}
		if _, err := s.ApplyAreaDamage(&c.DamageContext, float64(ally.HP)*1.5, CategoryUntyped, true, rng); err != nil {
			return err
		}
		// HP only: no defeat, reward or roster removal.
		s.actor(id).HP = 0
	}
	return nil
}

// npc405Field compares the draw with the temporary field's remaining turns
// (a default field reads 0). An equal draw ticks once instead of clearing,
// and a smaller draw never reaches the field's expiry.
func (s *LifetimeState) npc405Field(draw int) {
	turns := 0
	if s.Fields.Temporary != "" {
		turns = s.Fields.Turns
	}
	switch {
	case draw > turns:
		s.Fields.CreateAnti(draw - turns)
	case draw == turns:
		s.Fields.Turns--
	default:
		s.Fields.Turns -= draw
	}
}
