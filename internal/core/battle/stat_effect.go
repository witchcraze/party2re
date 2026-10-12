package battle

import "github.com/witchcraze/party2re/internal/core/random"

type BattleStat string

const (
	StatAttack  BattleStat = "attack"
	StatDefense BattleStat = "defense"
	StatAgility BattleStat = "agility"
	StatHP      BattleStat = "hp"
	StatHit     BattleStat = "hit"
)

func (k BattleStat) canRaise() bool { return k == StatAttack || k == StatDefense || k == StatAgility }

func (a *LifetimeActor) currentStat(k BattleStat) *int {
	switch k {
	case StatAttack:
		return &a.Attack
	case StatDefense:
		return &a.Defense
	case StatAgility:
		return &a.Agility
	case StatHP:
		return &a.HP
	case StatHit:
		return &a.Hit
	}
	return nil
}

func (a LifetimeActor) baseStat(k BattleStat) int {
	switch k {
	case StatAttack:
		return a.BaseStats.Attack
	case StatDefense:
		return a.BaseStats.Defense
	case StatAgility:
		return a.BaseStats.Agility
	case StatHP:
		return a.MaxHP
	}
	return 0 // Original mhit is absent; hit has its independent floor50.
}

func (t *LifetimeActor) statUpCap(a *LifetimeActor, stat BattleStat) float64 {
	multiplier := 2.5
	if t.ID == a.ID && (hasItem(a.ItemDefinitionIDs, "item-164") || (stat == StatAgility && hasItem(a.ItemDefinitionIDs, "item-183"))) {
		multiplier = 3
	}
	return float64(t.baseStat(stat)) * multiplier
}

func (t *LifetimeActor) statUpAmount(a *LifetimeActor, category CallbackCategory, stat BattleStat, v float64) int {
	if (hasItem(a.ItemDefinitionIDs, "item-148") && category == CategorySong) || hasItem(a.ItemDefinitionIDs, "item-159") {
		v *= 1.2
	}
	return int(float64(t.baseStat(stat)) * v * a.Tension)
}

func (s *LifetimeState) statUpTailPossible(c *EffectContext, e TargetEffect, stat BattleStat, powerMayChange bool) bool {
	a, t := s.actor(c.ActorID), s.actor(e.TargetID)
	if e.Payload.Number <= 0 || t.HP <= 0 || t.Status == StatusSabaku || float64(*t.currentStat(stat)) >= t.statUpCap(a, stat) {
		return false
	}
	v := s.finalEffectPower(c, s.initialEffectPower(c, e, ModifierStatUp))
	return v != 0 && (powerMayChange || t.statUpAmount(a, e.Category, stat, v) != 0)
}

type statUpAdmission struct {
	powerMayChange bool
	mayDouble      map[string]bool
}

func (s *LifetimeState) recordStatUp205(c *EffectContext, e TargetEffect, stat BattleStat, admission *statUpAdmission) {
	if stat == StatAttack || c.RequesterLevel == nil || *c.RequesterLevel < 0 {
		return
	}
	var changed []string
	for _, candidate := range s.partyTargetCandidates(c.TargetContext, e, admission.mayDouble) {
		if hasItem(s.actor(candidate.TargetID).ItemDefinitionIDs, "item-205") && s.statUpTailPossible(c, candidate, stat, admission.powerMayChange) {
			changed = append(changed, candidate.TargetID)
		}
	}
	for _, id := range changed {
		admission.mayDouble[id] = true
		admission.powerMayChange = true
	}
}

// ValidateStatUp admits possible equipment tails after the source's no-effect
// guards, before any cure/draw or mutation. The damage owner supplies a certified
// item201 executor; item205 uses requester level, never target level.
func (s *LifetimeState) ValidateStatUp(c *EffectContext, e TargetEffect, stat BattleStat, rng random.Generator) error {
	return s.validateStatUp(c, e, stat, rng, statUpAdmission{})
}

func (s *LifetimeState) validateStatUp(c *EffectContext, e TargetEffect, stat BattleStat, rng random.Generator, admission statUpAdmission) error {
	if err := s.validateRecovery(c, e, rng); err != nil {
		return err
	}
	if !stat.canRaise() {
		return ErrUnsupportedEffect
	}
	if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
		return err
	}
	for _, a := range s.Actors {
		if a.baseStat(stat) < 0 {
			return ErrInvalidParticipant
		}
	}
	if e.Payload.Number == 0 {
		return nil
	}
	for _, candidate := range s.partyTargetCandidates(c.TargetContext, e, admission.mayDouble) {
		a := s.actor(candidate.TargetID)
		if !s.statUpTailPossible(c, candidate, stat, admission.powerMayChange) {
			continue
		}
		if stat == StatAgility && hasItem(a.ItemDefinitionIDs, "item-201") && c.Damage == nil {
			return ErrUnsupportedEffect
		}
		if stat != StatAttack && hasItem(a.ItemDefinitionIDs, "item-205") && (c.RequesterLevel == nil || *c.RequesterLevel < 0) {
			return ErrUnsupportedEffect
		}
	}
	return nil
}

// ApplyStatUp uses base stats, retaining returned-but-unapplied cap/binding
// outcomes and item186/201/205 ordering (_skill.cgi:9307–9354).
func (s *LifetimeState) ApplyStatUp(c *EffectContext, e TargetEffect, stat BattleStat, rng random.Generator) (EffectApplication, error) {
	r := EffectApplication{}
	if err := s.ValidateStatUp(c, e, stat, rng); err != nil {
		return r, err
	}
	target, err := s.ResolvePartyTarget(c.TargetContext, e, rng)
	r.TargetResolution = target
	if err != nil || !target.Resolved {
		return r, err
	}
	t, a := s.actor(target.TargetID), s.actor(c.ActorID)
	v := target.Payload.Number
	if t.HP <= 0 || v <= 0 {
		return r, nil
	}
	c.StatUpCalls++
	cap := t.statUpCap(a, stat)
	value := t.currentStat(stat)
	if float64(*value) >= cap {
		r.Returned = !c.AdditionalEffect
		return r, nil
	}
	if t.Status == StatusSabaku {
		r.Returned = true
		return r, nil
	}
	v, r.IncidentalHealing, err = s.ModifyEffect(c, target.TargetEffect, ModifierStatUp, stat, rng)
	if err != nil {
		return r, err
	}
	r.Amount = t.statUpAmount(a, target.Category, stat, v)
	if r.Amount == 0 {
		return r, nil
	}
	a.Tension = 1
	before := *value
	*value = min(int(cap), *value+r.Amount)
	r.Returned, r.Delta, r.Applied = true, *value-before, *value != before
	if stat == StatAgility && hasItem(t.ItemDefinitionIDs, "item-201") {
		// Execute here: defeat/reset during this hit precedes the item205 test.
		hit := TargetEffect{Payload: CallbackPayload{Number: float64(t.Agility) * .5}, Arguments: []string{"1"}}
		if err := c.Damage(c.TargetContext, hit, rng); err != nil {
			return r, err
		}
		t = s.actor(target.TargetID)
	}
	if stat != StatAttack && hasItem(t.ItemDefinitionIDs, "item-205") {
		t.Attack = min(999, t.Attack+r.Amount)
		if err := s.statTension(c, t, false, rng); err != nil {
			return r, err
		}
		t.Temporary = TemporaryDouble
	}
	return r, nil
}

func (s *LifetimeState) statTension(c *EffectContext, t *LifetimeActor, recursive bool, rng random.Generator) error {
	if t.Status == StatusSabaku {
		return nil
	}
	if err := t.AdvanceTension(*c.RequesterLevel, rng); err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	if hasItem(a.ItemDefinitionIDs, "item-145") && !recursive && rng.Float64() < 1.0/6 && t.ID == a.ID && !c.ActorNPC {
		if err := s.statTension(c, a, true, rng); err != nil {
			return err
		}
	}
	if hasItem(t.ItemDefinitionIDs, "item-161") && rng.Float64() < .25 && t.Temporary != TemporaryDefense && t.Temporary != TemporaryStrongDefense && t.Temporary != TemporaryOneHitDefense {
		t.Temporary = TemporaryDefense
		if rng.Float64() < 1.0/6 && t.Tension >= 5 {
			t.Temporary = TemporaryStrongDefense
		}
	}
	return nil
}

// ApplyStatDown is for HP/AT/DF/AG/hit. MP drain remains the resource owner's
// separate operation; unknown stats reject before enemy conversion or RNG.
func (s *LifetimeState) ApplyStatDown(c *EffectContext, e TargetEffect, stat BattleStat, rng random.Generator) (EffectApplication, error) {
	r := EffectApplication{}
	if err := s.validateRecovery(c, e, rng); err != nil {
		return r, err
	}
	if !stat.canRaise() && stat != StatHP && stat != StatHit {
		return r, ErrUnsupportedEffect
	}
	target, err := s.ResolveEnemyTarget(c.TargetContext, e, rng)
	r.TargetResolution = target
	if err != nil || !target.Resolved {
		return r, err
	}
	t, a := s.actor(target.TargetID), s.actor(c.ActorID)
	if t.HP <= 0 || target.Payload.Number == 0 {
		return r, nil
	}
	v, _, err := s.ModifyEffect(c, target.TargetEffect, ModifierStatDown, stat, rng)
	if err != nil || v == 0 {
		return r, err
	}
	c.StatDownCalls++
	if t.BaseStats.Defense > 999 || ((stat == StatHP || stat == StatAttack) && t.HP > 999) || hasItem(t.ItemDefinitionIDs, "item-159") || (stat == StatHit && hasItem(t.ItemDefinitionIDs, "item-197")) {
		return r, nil
	}
	value := t.currentStat(stat)
	floor := float64(t.baseStat(stat)) * .2
	if float64(*value) <= floor || (stat == StatHit && *value <= 50) {
		return r, nil
	}
	r.Amount = int(float64(*value) * v * a.Tension)
	a.Tension = 1
	before := *value
	*value -= r.Amount
	if float64(*value) < floor {
		*value = int(floor)
	}
	if stat == StatHit {
		*value = max(50, *value)
	}
	r.Returned, r.Delta, r.Applied = true, *value-before, *value != before
	return r, nil
}

func (s *LifetimeState) ApplyPartyStat(c *EffectContext, coefficient float64, category CallbackCategory, stat BattleStat, down bool, rng random.Generator) ([]EffectApplication, error) {
	e := TargetEffect{Category: category, Payload: CallbackPayload{Number: coefficient}}
	if err := s.validateRecovery(c, e, rng); err != nil {
		return nil, err
	}
	if down {
		if !stat.canRaise() && stat != StatHP && stat != StatHit {
			return nil, ErrUnsupportedEffect
		}
	} else {
		admission := statUpAdmission{mayDouble: make(map[string]bool)}
		for _, id := range c.PartyIDs {
			e.TargetID = id
			if err := s.validateStatUp(c, e, stat, rng, admission); err != nil {
				return nil, err
			}
			s.recordStatUp205(c, e, stat, &admission)
		}
	}
	allowed, err := s.actor(c.ActorID).CheckCategorySeal(category, rng)
	if err != nil || !allowed {
		return nil, err
	}
	ids := c.PartyIDs
	if down {
		c.PartyStatDownCalls++
		ids = c.EnemyIDs
	} else {
		c.PartyStatUpCalls++
	}
	var results []EffectApplication
	for _, id := range ids {
		e.TargetID = id
		var r EffectApplication
		if down {
			r, err = s.ApplyStatDown(c, e, stat, rng)
		} else {
			r, err = s.ApplyStatUp(c, e, stat, rng)
		}
		if err != nil {
			return results, err
		}
		results = append(results, r)
	}
	return results, nil
}
