package battle

import "github.com/witchcraze/party2re/internal/core/random"

// DamageContext carries only this operation's facts. Reset/Defeat must be fully
// admitted by their owners, including all reachable descendants, before use.
// They share this state and cursor; nil retains the whole-effect gate.
type DamageContext struct {
	EffectContext
	End                    EndProcessing
	GuaranteedHit, Ordered bool
	Reset, Defeat          func(string) error
	Cursor                 TargetCursor
	ToolCategory           ToolCategoryContext
}

// ToolCategoryContext records the only checked package-category producers.
// A lexical physical/magic caller category is never valid provenance here.
type ToolCategoryContext struct {
	Source      string
	SavedAttack float64
}

func (c ToolCategoryContext) validate() error {
	if c.Source == "" && c.SavedAttack == 0 {
		return nil
	}
	if c.Source == "job77" && c.SavedAttack >= 0 && c.SavedAttack < 1e15 {
		return nil
	}
	return ErrUnsupportedEffect
}

func damageFlags(e TargetEffect) (direct, normal bool) {
	if len(e.Arguments) > 0 {
		direct = e.Arguments[0] == "1"
	}
	if len(e.Arguments) > 1 {
		normal = e.Arguments[1] == "1"
	}
	return
}

// ValidateDamage is inert. Catalog/outer composition owners must additionally
// admit every slot and indirect callback before costs or any preceding effect.
func (s *LifetimeState) ValidateDamage(c *DamageContext, e TargetEffect, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateRecovery(&c.EffectContext, e, rng); err != nil {
		return err
	}
	if len(e.Arguments) > 2 {
		return ErrUnsupportedEffect
	}
	for _, arg := range e.Arguments {
		if arg != "0" && arg != "1" && arg != "" {
			return ErrUnsupportedEffect
		}
	}
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return err
	}
	if len(c.EnemyIDs) == 0 || converted.Payload.Number == 0 {
		return nil
	}
	if err := c.End.Validate(); err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	for _, candidate := range s.damageCandidates(c, converted) {
		if candidate.Payload.Number == 0 {
			continue
		}
		t := s.actor(candidate.TargetID)
		if t.HP <= 0 {
			t = a
		}
		if t.Temporary == TemporaryOneHitDefense {
			continue
		}
		if !damageHasPower(a, t, candidate) {
			continue
		}
		if err := c.ToolCategory.validate(); err != nil {
			return err
		}
		if s.Fields.Current() == "cosmic" || s.Fields.Current() == "antlion" {
			return ErrUnsupportedEffect
		}
		if c.Defeat == nil || t.Gear == nil || a.Gear == nil || a.Agility < 0 || t.Agility < 0 {
			return ErrUnsupportedEffect
		}
		if (a.Extended == ExtendedPurification || hasAbility(a.Abilities, "seal_kuu")) && c.Reset == nil {
			return ErrUnsupportedEffect
		}
	}

	return nil
}

// ApplyDamage consumes source base power, never actor AT plus a scalar DTO.
// Target/resource facts remain visible even when the helper returns no tuple.
func (s *LifetimeState) ApplyDamage(c *DamageContext, e TargetEffect, rng random.Generator) (EffectApplication, error) {
	r := EffectApplication{}
	if err := s.ValidateDamage(c, e, rng); err != nil {
		return r, err
	}
	target, err := s.ResolveEnemyTarget(c.TargetContext, e, rng)
	r.TargetResolution = target
	if err != nil || !target.Resolved || target.Payload.Number == 0 {
		return r, err
	}
	c.DamageCalls++
	a, t := s.actor(c.ActorID), s.actor(target.TargetID)
	if t.HP <= 0 {
		t = a
		r.TargetID = a.ID
	}
	if t.ConsumeOneHitDefense() {
		r.TargetID = ""
		r.Returned = true
		return r, nil
	}
	if target.Category == CategoryPhysical && !c.GuaranteedHit {
		miss := float64(a.Hit) < rng.Float64()*100 || s.exceedAg(c, t, a, rng)
		if miss && t.Status != StatusKinju && t.Status != StatusSabaku && !hasItem(a.ItemDefinitionIDs, "weapon-63") {
			a.Tension = 1
			r.TargetID = ""
			r.Returned = true
			return r, nil
		}
	}
	direct, normal := damageFlags(target.TargetEffect)
	v := s.modifyDamage(c, t, target.Payload.Number, target.Category, direct, normal, rng)
	if v == 0 {
		return r, nil
	}
	if a.Extended == ExtendedPurification {
		status, tension := t.Status, t.Tension
		if err := c.Reset(t.ID); err != nil {
			return r, err
		}
		a, t = s.actor(c.ActorID), s.actor(r.TargetID)
		t.Status, t.Tension = status, tension
	}
	if (hasItem(a.ItemDefinitionIDs, "item-190") && target.Category == CategoryDance && rng.Float64() < 1.0/6) ||
		(a.Extended == ExtendedFlame && target.Category == CategoryPhysical && v <= float64(a.Attack)) {
		direct = true
	}
	minus := float64(t.Defense+t.Gear.ArmorDefense) * .3
	defense := t.Defense
	if hasItem(a.ItemDefinitionIDs, "item-181") && target.Category == CategoryPhysical {
		defense = int(float64(defense) * .5)
	}
	if hasItem(t.ItemDefinitionIDs, "item-162") {
		minus = (float64(defense)*1.5 + float64(t.Gear.ArmorDefense)) * .3
	}
	if hasItem(a.ItemDefinitionIDs, "item-187") {
		minus -= float64(t.Defense-t.BaseStats.Defense) * .3
	}
	retain := hasItem(a.ItemDefinitionIDs, "item-149")
	if !retain {
		v *= a.Tension
	}
	if hasItem(t.ItemDefinitionIDs, "item-162") && target.Category == CategoryPhysical {
		direct = false
	}
	r.Amount = rollDamage(v, minus, direct, t.BaseStats.Defense, rng)
	before := t.HP
	t.HP -= r.Amount
	if !retain {
		a.Tension = 1
	}
	if hasItem(t.ItemDefinitionIDs, "item-185") {
		c.Item185Activated = true
	}
	if hasItem(t.ItemDefinitionIDs, "item-195") {
		c.Item195Activated = true
	}
	if t.HP <= 0 {
		t.HP = 0
		if err := c.Defeat(t.ID); err != nil {
			return r, err
		}
	} else if hasItem(t.ItemDefinitionIDs, "item-162") && target.Category == CategoryPhysical {
		t.Defense = min(int(float64(t.BaseStats.Defense)*2.5), t.Defense+int(float64(t.BaseStats.Defense)*.2))
	} else if t.Extended == ExtendedInterception && t.ID != a.ID {
		if _, err := s.ApplyToolDamage(c, a.ID, float64(t.Attack)*.2+5, true, true, rng); err != nil {
			return r, err
		}
		t = s.actor(r.TargetID)
		t.ExtendedTurns--
		if t.ExtendedTurns <= 0 {
			t.Extended = ""
		}
	}
	a, t = s.actor(c.ActorID), s.actor(r.TargetID)
	if s.Fields.Current() == "darkness" && a.HP > 0 {
		c.End.RecordDarkness(r.Amount)
	}
	if t.HP > 0 && hasAbility(a.Abilities, "seal_kuu") && rng.Float64() < .125 {
		tension := t.Tension
		if err := c.Reset(t.ID); err != nil {
			return r, err
		}
		t = s.actor(r.TargetID)
		t.Tension = tension
	}
	r.Returned, r.Applied, r.Delta = true, true, t.HP-before
	return r, nil
}

func rollDamage(power, minus float64, direct bool, baseDefense int, rng random.Generator) int {
	if !direct || baseDefense > 999 {
		power = power*.5 - minus
	}
	v := int(power * (.9 + rng.Float64()*.3))
	if v < 1 {
		v = int(rng.Float64()*2) + 1
	}
	return v
}

// _is_exceed_ag reads item147 from the effect actor even during evasion.
// Zero agility still draws: Perl rand(0) is rand(1).
func (s *LifetimeState) exceedAg(c *DamageContext, m, y *LifetimeActor, rng random.Generator) bool {
	mult := 3.0
	if hasItem(s.actor(c.ActorID).ItemDefinitionIDs, "item-147") {
		if rng.Float64() >= .5 {
			return false
		}
		mult = 1
	} else if m.Extended == ExtendedPurification {
		return rng.Float64()*float64(max(1, m.Agility*3)) >= rng.Float64()*float64(max(1, y.Agility))
	} else if rng.Float64() >= 1.0/3 {
		return false
	}
	return rng.Float64()*float64(max(1, m.Agility)) >= rng.Float64()*float64(max(1, int(float64(y.Agility)*mult)))
}

// ApplyAreaDamage pre-evaluates modifiers before recording the area count.
// It retains every invocation, including zero/miss, and source roster order.
func (s *LifetimeState) ApplyAreaDamage(c *DamageContext, power float64, category CallbackCategory, direct bool, rng random.Generator) ([]EffectApplication, error) {
	e := TargetEffect{Category: category, Payload: CallbackPayload{Number: power}}
	if direct {
		e.Arguments = []string{"1"}
	}
	if err := s.validateAreaDamage(c, e, rng); err != nil {
		return nil, err
	}
	allowed, err := s.actor(c.ActorID).CheckCategorySeal(category, rng)
	if err != nil || !allowed {
		return nil, err
	}
	initial := s.modifyDamage(c, nil, power, category, direct, false, rng)
	c.AreaDamageCalls++
	if s.actor(c.ActorID).Extended == ExtendedAdditional && category == CategoryMagic {
		power *= 1.5
	}
	var results []EffectApplication
	for _, id := range c.EnemyIDs {
		if s.actor(c.ActorID).HP <= 0 {
			break
		}
		e.TargetID, e.Payload.Number = id, power
		r, err := s.ApplyDamage(c, e, rng)
		if err != nil {
			return results, err
		}
		results = append(results, r)
		if !hasItem(s.actor(c.ActorID).ItemDefinitionIDs, "item-144") || initial > 300 {
			power *= .85
		}
	}
	return results, nil
}
