package battle

import "github.com/witchcraze/party2re/internal/core/random"

// This producer's unchecked tuple write is supported only when every reached
// hit has a participant result. Empty-record continuations stay wholly gated.
func (s *LifetimeState) validateDoubleTail(c *RosterContext, rng random.Generator) error {
	if s.actor(c.ActorID).Status == StatusConfusion || c.Item195Activated {
		return ErrUnsupportedEffect
	}
	var effects []TargetEffect
	for _, id := range c.EnemyIDs {
		a := s.actor(id)
		if a.HP <= 0 || hasItem(a.ItemDefinitionIDs, "item-195") {
			return ErrUnsupportedEffect
		}
		e := rosterEffect(id, 150, CategoryBreath, true)
		for _, target := range s.damageCandidates(&c.DamageContext, e) {
			t := s.actor(target.TargetID)
			if target.Payload.Number == 0 || t.Temporary == TemporaryOneHitDefense {
				return ErrUnsupportedEffect
			}
		}
		// Earlier double writes can remove covering/reflection hooks. Without a
		// transition projection those target changes remain uncertified here.
		if a.Temporary == TemporaryCover || a.Temporary == TemporaryCovering || a.Temporary == TemporaryReflectBreath || a.Temporary == TemporaryPhantom {
			return ErrUnsupportedEffect
		}
		effects = append(effects, e)
	}
	return s.validateDamageSequence(&c.DamageContext, effects, rng)
}

func (s *LifetimeState) applyDoubleTail(c *RosterContext, rng random.Generator) (RosterResult, error) {
	r := RosterResult{}
	c.AdditionalEffect = true
	for _, id := range c.EnemyIDs {
		hit, err := s.ApplyDamage(&c.DamageContext, rosterEffect(id, 150, CategoryBreath, true), rng)
		if err != nil {
			return r, err
		}
		r.Effects = append(r.Effects, hit)
		if rng.Float64()*2 >= 1 {
			if hit.TargetID == "" {
				return r, ErrUnsupportedEffect
			}
			s.actor(hit.TargetID).Temporary = TemporaryDouble
		}
	}
	r.Applied = len(r.Effects) > 0
	return r, nil
}

func venomRepeats(a *LifetimeActor) int {
	if a.RosterIdentity == callingIdentity(87) && a.SummonForm >= 1 {
		return 2
	}
	return 1
}

func (s *LifetimeState) validateVenom(c *RosterContext, helperID string, rng random.Generator) error {
	var effects []TargetEffect
	for range venomRepeats(s.actor(helperID)) {
		for _, id := range c.EnemyIDs {
			e := rosterEffect(id, 120, CategoryBreath, true)
			effects = append(effects, e)
			if s.actor(id).HP <= 0 {
				continue
			}
			if s.actor(id).MaxHP > 999 || s.actor(id).BaseStats.Defense > 999 {
				continue
			}
			if c.Status == nil || c.Status.Validate == nil || c.Status.Execute == nil {
				return ErrUnsupportedEffect
			}
			for _, status := range []string{StatusDeadlyPoison, StatusParalyze} {
				child := TargetEffect{TargetID: id, Category: CategoryBreath, Payload: CallbackPayload{Symbol: status}}
				if err := c.Status.Validate(s, c.TargetContext, child, .75); err != nil {
					return err
				}
			}
		}
	}
	return s.validateDamageSequence(&c.DamageContext, effects, rng)
}

func (s *LifetimeState) applyVenom(c *RosterContext, helperID string, rng random.Generator) (RosterResult, error) {
	r := RosterResult{HelperID: helperID}
	h := s.actor(helperID)
	repeats := 1
	if venomRepeats(h) == 2 {
		chance := 4.0
		if h.SummonForm == 2 {
			chance = 2
		}
		if rng.Float64()*chance < 1 {
			repeats = 2
		}
	}
	for range repeats {
		for _, id := range c.EnemyIDs {
			hit, err := s.ApplyDamage(&c.DamageContext, rosterEffect(id, 120, CategoryBreath, true), rng)
			if err != nil {
				return r, err
			}
			r.Effects = append(r.Effects, hit)
			if id != hit.TargetID {
				continue
			}
			t := s.actor(id)
			if t.MaxHP > 999 || t.BaseStats.Defense > 999 {
				if rng.Float64()*3 < 1 {
					t.Status = StatusPoison
				}
			} else {
				status := []string{StatusDeadlyPoison, StatusParalyze}[int(rng.Float64()*2)]
				child := TargetEffect{TargetID: id, Category: CategoryBreath, Payload: CallbackPayload{Symbol: status}}
				if err := c.Status.Execute(s, c.TargetContext, child, .75, rng); err != nil {
					return r, err
				}
			}
		}
	}
	s.growHelper(c, helperID, 5, rng)
	r.Applied = len(r.Effects) > 0
	return r, nil
}
