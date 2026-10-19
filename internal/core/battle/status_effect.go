package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

type StatusContext struct {
	EffectContext
	Defeat                                                  *DefeatContext
	StatusCalls, AreaStatusCalls, CureCalls, PartyCureCalls int
	DeathCalls, AreaDeathCalls                              int
}

type StatusApplication struct {
	TargetResolution
	Returned, Applied bool
}

func statusImmune(t *LifetimeActor, bypass, death bool, field string) bool {
	if death {
		return t.HP > 999 || t.BaseStats.Defense > 999 || (hasItem(t.ItemDefinitionIDs, "item-161") && t.Tension >= 5)
	}
	if t.NPC {
		return t.MaxHP > 999 || t.BaseStats.Defense > 999
	}
	return !bypass && field != "den" && (t.HP > 999 || t.BaseStats.Defense > 999)
}

// ValidateStatus admits the entire reachable defeat closure before category
// cures, target replacement, incidental MP, counters or probability draws.
func (s *LifetimeState) ValidateStatus(c *StatusContext, e TargetEffect, chance float64, bypass, death bool, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if err := e.validate(); err != nil {
		return err
	}
	if math.IsNaN(chance) || math.IsInf(chance, 0) || chance < 0 || chance > 1e12 {
		return ErrUnsupportedEffect
	}
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return err
	}
	if len(c.EnemyIDs) == 0 || converted.Payload.Symbol == "" {
		return nil
	}
	dc := DamageContext{EffectContext: c.EffectContext}
	// A successful category cure can enable item195's confusion activation.
	projected := *s
	projected.Actors = slices.Clone(s.Actors)
	if projected.actor(c.ActorID).categorySealed(converted.Category) {
		projected.actor(c.ActorID).Status = ""
	}
	dc.TargetContext = c.TargetContext
	for _, candidate := range projected.damageCandidates(&dc, converted) {
		t := projected.actor(candidate.TargetID)
		if t.HP <= 0 || candidate.Payload.Symbol == "" || statusImmune(t, bypass, death, projected.Fields.Current()) {
			continue
		}
		probability := TargetEffect{Category: candidate.Category, Payload: CallbackPayload{Number: chance}}
		if !death && projected.finalEffectPower(&c.EffectContext, projected.initialEffectPower(&c.EffectContext, probability, ModifierStatus)) <= 0 {
			continue
		}
		if t.Gear == nil {
			return ErrUnsupportedEffect
		}
		if !death {
			continue
		}
		if c.Defeat == nil {
			return ErrUnsupportedEffect
		}
		child := *c.Defeat
		child.TargetContext = c.TargetContext
		if err := projected.ValidateDefeat(&child, t.ID, rng); err != nil {
			return err
		}
	}
	return nil
}

// ApplyStatus retains a returned status tuple even on failure/immunity. Death
// has no tuple, calls defeat directly and uses >=; status uses strict >.
func (s *LifetimeState) ApplyStatus(c *StatusContext, e TargetEffect, chance float64, bypass, death bool, rng random.Generator) (StatusApplication, error) {
	r := StatusApplication{}
	if err := s.ValidateStatus(c, e, chance, bypass, death, rng); err != nil {
		return r, err
	}
	target, err := s.ResolveEnemyTarget(c.TargetContext, e, rng)
	r.TargetResolution = target
	if err == nil && c.targetResources != nil {
		c.targetResources(target)
	}
	if err != nil || !target.Resolved || target.Payload.Symbol == "" {
		return r, err
	}
	t := s.actor(target.TargetID)
	if t.HP <= 0 {
		return r, nil
	}
	modifier := ModifierStatus
	if death {
		modifier = ModifierInstantDeath
	}
	probability := target.TargetEffect
	probability.Payload = CallbackPayload{Number: chance}
	chance, _, err = s.ModifyEffect(&c.EffectContext, probability, modifier, "", rng)
	if err != nil {
		return r, err
	}
	if death {
		c.DeathCalls++
	} else {
		c.StatusCalls++
		r.Returned = true
	}
	if !death && s.Fields.Current() == "den" {
		chance = math.Trunc(chance * 1.2)
	}
	if statusImmune(t, bypass, death, s.Fields.Current()) {
		if c.AdditionalEffect {
			r.Returned = false
		}
		return r, nil
	}
	draw := rng.Float64() * 100
	success := chance > draw
	if death {
		success = chance >= draw
	}
	if success && t.Gear != nil && t.Gear.WeaponID == "weapon-68" && t.Gear.ArmorID == "armor-53" {
		success = rng.Float64() < .5
	}
	if !success {
		return r, nil
	}
	if !death {
		if hasItem(t.ItemDefinitionIDs, "item-161") && t.Tension >= 5 {
			return r, nil
		}
		t.Status, t.Tension = target.Payload.Symbol, 1
	} else {
		c.Defeat.TargetContext = c.TargetContext
		if _, err := s.ApplyDefeat(c.Defeat, t.ID, rng); err != nil {
			return r, err
		}
	}
	r.Applied = true
	return r, nil
}

func (s *LifetimeState) ValidateAreaStatus(c *StatusContext, e TargetEffect, chance float64, bypass, death bool, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if err := s.ValidateStatus(c, e, chance, bypass, death, rng); err != nil {
		return err
	}
	if death {
		return s.validateAreaDeath(c, e, chance, bypass, rng)
	}
	projected := *s
	projected.Actors = slices.Clone(s.Actors)
	child := *c
	if projected.actor(c.ActorID).categorySealed(e.Category) {
		projected.actor(c.ActorID).Status = ""
	}
	// Earlier status applications may confuse the actor via reflection/cover.
	// Admit that ordered continuation as well as the original target paths.
	confusionStates := []bool{false}
	if !death && e.Payload.Symbol == StatusConfusion {
		confusionStates = append(confusionStates, true)
	}
	for _, confused := range confusionStates {
		if confused {
			projected.actor(c.ActorID).Status = StatusConfusion
		}
		for _, id := range c.EnemyIDs {
			e.TargetID = id
			if err := s.ValidateStatus(&child, e, chance, bypass, death, rng); err != nil {
				return err
			}
			if err := projected.ValidateStatus(&child, e, chance, bypass, death, rng); err != nil {
				return err
			}
		}
	}
	return nil
}

// ApplyAreaStatus does not stop on actor death; the original plural status and
// instant-death loops visit every ordered enemy, unlike area damage.
func (s *LifetimeState) ApplyAreaStatus(c *StatusContext, e TargetEffect, chance float64, bypass, death bool, rng random.Generator) ([]StatusApplication, error) {
	if err := s.ValidateAreaStatus(c, e, chance, bypass, death, rng); err != nil {
		return nil, err
	}
	allowed, err := s.actor(c.ActorID).CheckCategorySeal(e.Category, rng)
	if err != nil || !allowed {
		return nil, err
	}
	if death {
		c.AreaDeathCalls++
	} else {
		c.AreaStatusCalls++
	}
	var results []StatusApplication
	for _, id := range c.EnemyIDs {
		e.TargetID = id
		r, err := s.ApplyStatus(c, e, chance, bypass, death, rng)
		if err != nil {
			return results, err
		}
		results = append(results, r)
	}
	return results, nil
}

func (s *LifetimeState) ApplyCure(c *StatusContext, e TargetEffect, rng random.Generator) (StatusApplication, error) {
	r := StatusApplication{}
	if c == nil {
		return r, ErrInvalidRequest
	}
	target, err := s.ResolvePartyTarget(c.TargetContext, e, rng)
	r.TargetResolution = target
	if err == nil && c.targetResources != nil {
		c.targetResources(target)
	}
	if err != nil || !target.Resolved || target.Payload.Symbol == "" {
		return r, err
	}
	t := s.actor(target.TargetID)
	if t.HP <= 0 {
		return r, nil
	}
	c.CureCalls++
	r.Returned = true
	if t.Status == target.Payload.Symbol || (target.Payload.Symbol == StatusDeadlyPoison && (t.Status == StatusPoison || t.Status == StatusVirulentPoison)) {
		t.Status = ""
		r.Applied = true
	}
	return r, nil
}

func (s *LifetimeState) ApplyPartyCure(c *StatusContext, e TargetEffect, rng random.Generator) ([]StatusApplication, error) {
	if c == nil {
		return nil, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return nil, err
	}
	for _, id := range c.PartyIDs {
		e.TargetID = id
		if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
			return nil, err
		}
	}
	allowed, err := s.actor(c.ActorID).CheckCategorySeal(e.Category, rng)
	if err != nil || !allowed {
		return nil, err
	}
	c.PartyCureCalls++
	c.AdditionalEffect = true
	var results []StatusApplication
	for _, id := range c.PartyIDs {
		e.TargetID = id
		r, err := s.ApplyCure(c, e, rng)
		if err != nil {
			return results, err
		}
		results = append(results, r)
	}
	return results, nil
}

func applyStatusResources(s *LifetimeState, r TargetResolution) {
	if r.MPRecoveryTarget != "" {
		t := s.actor(r.MPRecoveryTarget)
		t.MP = min(t.MaxMP, t.MP+r.MPRecovery)
	}
}

// ApplyAbilityCure runs before CheckIncapacitation for players and NPCs. Map17
// ability production is deliberately absent from ordinary-stage admission.
func (a *LifetimeActor) ApplyAbilityCure(rng random.Generator) error {
	if rng == nil {
		return ErrInvalidRequest
	}
	specific := ""
	switch a.Status {
	case StatusPoison, StatusDeadlyPoison, StatusVirulentPoison:
		specific = "poison1"
	case StatusParalyze:
		specific = "paralysis1"
	case StatusSleep:
		specific = "sleep1"
	case StatusConfusion:
		specific = "confusion1"
	case StatusDofuu:
		specific = "sealed1"
	case StatusPhysicalSeal:
		specific = "attack_sealed1"
	case StatusMagicSeal:
		specific = "magic_sealed1"
	}
	if specific != "" && hasAbility(a.Abilities, specific) && rng.Float64() < .5 {
		a.Status = ""
	}
	if a.Status != "" && hasAbility(a.Abilities, "state1") && rng.Float64() < .2 {
		a.Status = ""
	}
	return nil
}
