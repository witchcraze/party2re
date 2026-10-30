package battle

import "github.com/witchcraze/party2re/internal/core/random"

// These direct writes deliberately bypass targeting, immunity and healing.
// One shared state draw applies to all living members (_skill.cgi:9721–9767).
func (s *LifetimeState) applyFiniteChaos(rng random.Generator) {
	branch := 5
	for i, width := range []float64{2, 2, 2, 3, 2} {
		if rng.Float64()*width < 1 {
			branch = i
			break
		}
	}
	tmp := TemporaryEffect("")
	status := ""
	if branch == 0 {
		tmp = []TemporaryEffect{TemporaryDouble, TemporaryReflectAttack, TemporaryReflectMagic, TemporaryDeflect}[int(rng.Float64()*4)]
	}
	if branch == 1 {
		status = []string{StatusConfusion, StatusSleep, StatusParalyze, StatusDeadlyPoison}[int(rng.Float64()*4)]
	}
	for i := range s.Actors {
		a := &s.Actors[i]
		switch branch {
		case 0:
			if a.HP > 0 {
				a.Temporary = tmp
			}
		case 1:
			if a.HP > 0 {
				a.Status, a.Tension = status, 1
			}
		case 2:
			if a.MaxHP <= 999 {
				a.HP = a.MaxHP
			}
		case 3:
			a.Agility = 0
		case 4:
			if a.HP > 0 && a.HP <= 999 {
				a.HP = 1
			}
		}
	}
}

func (s *LifetimeState) applyRainbow(c *NamedFieldContext, branch int, rng random.Generator) error {
	switch branch {
	case 0, 4, 5:
		stat := StatAttack
		if branch == 4 {
			stat = StatDefense
			for _, id := range c.PartyIDs {
				if a := s.actor(id); a.HP > 0 {
					a.Hit = 95
				}
			}
		}
		if branch == 5 {
			stat = StatAgility
			status := StatusContext{EffectContext: c.EffectContext, CureCalls: c.CureCalls}
			for _, id := range c.PartyIDs {
				if _, err := s.ApplyCure(&status, controlSymbol(id, s.actor(id).Status, CategoryUntyped), rng); err != nil {
					return err
				}
			}
			c.EffectContext, c.CureCalls = status.EffectContext, status.CureCalls
		}
		_, err := s.ApplyPartyStat(&c.EffectContext, 1, CategoryUntyped, stat, false, rng)
		return err
	case 1:
		for _, id := range c.PartyIDs {
			a := s.actor(id)
			if a.HP >= 0 {
				a.Temporary = TemporaryRevival
			}
		}
	case 2:
		for _, id := range c.PartyIDs {
			a := s.actor(id)
			if a.HP <= 0 {
				a.HP = a.MaxHP
			}
		}
	case 3:
		_, err := s.ApplyPartyHealing(&c.EffectContext, 500, CategoryUntyped, rng)
		return err
	case 6:
		c.PartyMPHealCalls++
		for _, id := range c.PartyIDs {
			if _, err := s.ApplyMPHealing(&c.EffectContext, controlEffect(id, 300, CategoryUntyped), rng); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *LifetimeState) validateRainbow(c *NamedFieldContext, rng random.Generator) error {
	for _, id := range c.PartyIDs {
		for _, e := range []TargetEffect{controlEffect(id, 500, CategoryUntyped), controlSymbol(id, s.actor(id).Status, CategoryUntyped)} {
			if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
				return err
			}
		}
	}
	for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
		p := s.callbackCopy()
		states := []LifetimeState{p}
		if stat == StatAgility {
			// Cures can remove binding/confusion on redirected recipients. Admit
			// both unchanged and cured eligibility before the party stat helper.
			cured := p.callbackCopy()
			for i := range cured.Actors {
				cured.Actors[i].Status = ""
			}
			states = append(states, cured)
		}
		for _, p := range states {
			admission := statUpAdmission{mayReplace: make(map[string]float64)}
			for _, id := range c.PartyIDs {
				e := controlEffect(id, 1, CategoryUntyped)
				if err := p.validateStatUp(&c.EffectContext, e, stat, rng, admission); err != nil {
					return err
				}
				p.recordStatUp205(&c.EffectContext, e, stat, &admission)
			}
		}
	}
	return nil
}
