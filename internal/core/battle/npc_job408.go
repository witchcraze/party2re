package battle

import "github.com/witchcraze/party2re/internal/core/random"

// NPCJob408Definition binds an intrinsic NPC body to the response's existing
// damage and reset ports. The HP branch is read at execution (_skill.cgi:8318–8373).
func NPCJob408Definition(id string) CallbackDefinition {
	return CallbackDefinition{
		Validate: func(s *LifetimeState, f *CallbackContext) error {
			return s.validateNPC408(f, id, random.Default())
		},
		Execute: func(s *LifetimeState, f *CallbackContext, rng random.Generator) error {
			return s.applyNPC408(f, id, rng)
		},
	}
}

func npc408Wounded(a *LifetimeActor) bool {
	return a.HP < a.MaxHP-a.HP
}

func npc408Target(f *CallbackContext) string {
	if len(f.Arguments) > 0 {
		return f.Arguments[0]
	}
	return ""
}

var npc408Stats = [...]BattleStat{StatAttack, StatDefense, StatAgility}

func (s *LifetimeState) validateNPC408(f *CallbackContext, id string, rng random.Generator) error {
	if f == nil || f.Damage == nil || !f.Damage.ActorNPC {
		return ErrUnsupportedEffect
	}
	if err := s.validateTargetContext(f.TargetContext, rng); err != nil {
		return err
	}
	a := s.actor(f.ActorID)
	if !a.NPC {
		return ErrUnsupportedEffect
	}
	damage := *f.Damage
	damage.TargetContext = f.TargetContext
	switch id {
	case "npc-job-408-position-0":
		if !npc408Wounded(a) {
			e := dualAgilityEffect(npc408Target(f), 350, CategoryUntyped, true)
			_, err := s.projectCallbackDamage(&damage, f.Defeat, e, 350, false, rng)
			return err
		}
		p := s.callbackCopy()
		damage = p.nativeDamageContext(damage, f.Defeat, rng)
		for _, target := range f.EnemyIDs {
			e := dualAgilityEffect(target, 400, CategoryUntyped, true)
			if err := p.admitNPC405Hit(&damage, f.Defeat, e, 400, rng); err != nil {
				return err
			}
			t := p.actor(target)
			t.MaxHP -= t.MaxHP / 5
		}
	case "npc-job-408-position-1":
		hits := 5
		if npc408Wounded(a) {
			hits++
		}
		p := s.callbackCopy()
		damage = p.nativeDamageContext(damage, f.Defeat, rng)
		for range hits {
			if err := p.admitNPC405Area(&damage, f.Defeat, dualAgilityEffect("", 180, CategoryBreath, true), rng); err != nil {
				return err
			}
		}
	case "npc-job-408-position-2":
		if f.Defeat == nil && len(f.EnemyIDs) > 0 {
			return ErrUnsupportedEffect
		}
		for _, target := range f.EnemyIDs {
			reset := f.Defeat.EquipmentResetContext
			reset.TargetContext = f.TargetContext
			if err := s.ValidateEquipmentReset(&reset, target, rng); err != nil {
				return err
			}
			// Untyped stat-down admission depends on the frame and effect, which the reset preserves.
			for _, stat := range npc408Stats {
				if err := s.ValidateStatDown(&damage.EffectContext, controlEffect(target, .5, CategoryUntyped), stat, rng); err != nil {
					return err
				}
			}
		}
	default:
		return ErrUnsupportedEffect
	}
	return nil
}

func (s *LifetimeState) applyNPC408(f *CallbackContext, id string, rng random.Generator) error {
	damage := f.Damage
	damage.TargetContext = f.TargetContext
	previous := *damage
	native := s.nativeDamageContext(*damage, f.Defeat, rng)
	damage.Reset, damage.Defeat, damage.responseDefeat = native.Reset, native.Defeat, f.Defeat
	resources := func(r TargetResolution) {
		a := s.actor(f.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	damage.targetResources, damage.EffectContext.targetResources = resources, resources
	defer func() {
		damage.Reset, damage.Defeat, damage.responseDefeat = previous.Reset, previous.Defeat, previous.responseDefeat
		damage.targetResources, damage.EffectContext.targetResources = previous.targetResources, previous.EffectContext.targetResources
	}()
	wounded := npc408Wounded(s.actor(f.ActorID))
	switch id {
	case "npc-job-408-position-0":
		if !wounded {
			r, err := s.ApplyDamage(damage, dualAgilityEffect(npc408Target(f), 350, CategoryUntyped, true), rng)
			if err == nil && r.Applied {
				target := s.actor(r.TargetID)
				if target.TeamID != s.actor(f.ActorID).TeamID {
					target.MaxHP -= target.MaxHP / 5
				}
			}
			return err
		}
		for _, target := range f.EnemyIDs {
			if _, err := s.ApplyDamage(damage, dualAgilityEffect(target, 400, CategoryUntyped, true), rng); err != nil {
				return err
			}
			a := s.actor(target)
			a.MaxHP -= a.MaxHP / 5
		}
	case "npc-job-408-position-1":
		hits := 2 + int(rng.Float64()*4)
		if wounded {
			hits++
		}
		for range hits {
			if s.actor(f.ActorID).HP <= 0 {
				break
			}
			if _, err := s.ApplyAreaDamage(damage, 180, CategoryBreath, true, rng); err != nil {
				return err
			}
		}
	case "npc-job-408-position-2":
		for _, target := range f.EnemyIDs {
			if err := s.applyDamageReset(damage, target, rng); err != nil {
				return err
			}
			damage.Cursor = f.Defeat.Cursor
			if rng.Float64() >= .5 {
				continue
			}
			s.actor(target).Status = StatusParalyze
			for _, stat := range npc408Stats {
				if _, err := s.ApplyStatDown(&damage.EffectContext, controlEffect(target, .5, CategoryUntyped), stat, rng); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
