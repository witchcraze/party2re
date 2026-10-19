package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// DefeatRewards composes the reward owner after native reset/equipment hooks.
// Validation is pure; Apply runs once per reached defeat, never on prevention.
type DefeatRewards struct {
	Validate func(*LifetimeState, TargetContext, string) error
	Apply    func(*LifetimeState, TargetContext, string) error
}

type DefeatContext struct {
	EquipmentResetContext
	ToolCategory ToolCategoryContext
	Rewards      *DefeatRewards
	FieldAtLoad  string
	// Fortress belongs to the field owner; the load snapshot and current anti
	// predicate are checked here only after the source reward reachability guard.
	Fortress *DefeatRewards
}

type DefeatResult struct {
	Prevented, Defeated, RewardsReached bool
	FinalStrike                         *EffectApplication
}

func (s *LifetimeState) ValidateDefeat(c *DefeatContext, targetID string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	return s.validateDefeat(c, targetID, rng, map[string]bool{})
}

func (s *LifetimeState) validateResetHP(c *EquipmentResetContext, id string, hp int, rng random.Generator) error {
	projected := *s
	projected.Actors = slices.Clone(s.Actors)
	if projected.actor(id) == nil {
		return ErrInvalidParticipant
	}
	projected.actor(id).HP = hp
	return projected.ValidateEquipmentReset(c, id, rng)
}

func (s *LifetimeState) validateDefeat(c *DefeatContext, id string, rng random.Generator, visiting map[string]bool) error {
	a := s.actor(id)
	if a == nil {
		return ErrInvalidParticipant
	}
	if visiting[id] {
		return ErrUnsupportedEffect
	} // A reachable recursive final strike has no finite closure.
	visiting[id] = true
	defer delete(visiting, id)
	auto := AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: func(string) error { return nil }}
	if err := s.ValidateAutomaticRevival(&auto, id, rng); err != nil {
		return err
	}
	hp := 0
	branch := automaticRevivalBranch(a)
	switch branch {
	case "temporary":
		hp = int(float64(a.MaxHP) * .1)
	case "pharaoh":
		hp = a.MaxHP
	case "half":
		hp = a.MaxHP / 2
	case "curse":
		hp = int(float64(a.MaxHP) * .3)
	}
	if err := s.validateResetHP(&c.EquipmentResetContext, id, hp, rng); err != nil {
		return err
	}
	if branch != "" {
		return nil
	}
	if hasItem(a.ItemDefinitionIDs, "item-212") {
		// Target selection uses the acting participant, not the fallen holder.
		// Reset can clear that actor's effects; admit both continuations.
		projected := *s
		projected.Actors = slices.Clone(s.Actors)
		fallen := projected.actor(id)
		fallen.HP = 0
		fallen.Status = ""
		fallen.Temporary = ""
		fallen.Extended = ""
		if fallen.Gear.AccessoryID == "item-143" {
			fallen.Status = StatusPhysicalSeal
		}
		if fallen.Gear.AccessoryID == "item-116" {
			fallen.Status = StatusDofuu
		}
		candidates, err := projected.defeatStrikeCandidates(c, id, rng)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			t := projected.actor(candidate.TargetID)
			if t.Temporary == TemporaryOneHitDefense {
				continue
			}
			if err := c.ToolCategory.validate(); err != nil {
				return err
			}
			if t.Gear == nil {
				return ErrUnsupportedEffect
			}
			if err := projected.validateDefeat(c, t.ID, rng, visiting); err != nil {
				return err
			}
		}
	}
	if s.actor(c.ActorID).NPC || id == c.ActorID {
		return nil
	}
	if c.Rewards == nil || c.Rewards.Validate == nil || c.Rewards.Apply == nil {
		return ErrUnsupportedEffect
	}
	if err := c.Rewards.Validate(s, c.TargetContext, id); err != nil {
		return err
	}
	if c.FieldAtLoad == "fortress" && s.Fields.AntiTurns == 0 {
		if c.Fortress == nil || c.Fortress.Validate == nil || c.Fortress.Apply == nil {
			return ErrUnsupportedEffect
		}
		return c.Fortress.Validate(s, c.TargetContext, id)
	}
	return nil
}

func (s *LifetimeState) defeatStrikeCandidates(c *DefeatContext, id string, rng random.Generator) ([]TargetEffect, error) {
	e := TargetEffect{Payload: CallbackPayload{Symbol: "last_strike"}}
	if slices.Contains(c.EnemyIDs, id) {
		if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
			return nil, err
		}
		return s.partyTargetCandidates(c.TargetContext, e, nil), nil
	}
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return nil, err
	}
	// Source still calls tool damage after a missing helper result. The
	// resulting empty-record defeat is not a represented participant.
	if len(c.EnemyIDs) == 0 {
		return nil, ErrUnsupportedEffect
	}
	if s.actor(c.ActorID).Status != StatusConfusion {
		for _, enemy := range c.EnemyIDs {
			if s.actor(enemy).HP <= 0 {
				return nil, ErrUnsupportedEffect
			}
		}
	}
	return s.damageCandidates(&DamageContext{EffectContext: EffectContext{TargetContext: c.TargetContext}}, converted), nil
}

// ApplyDefeat composes the existing automatic revival before the one reset
// path, then final strike/inheritance, NPC/self guard, rewards and fortress.
// The caller's transaction owns rollback on an external callback failure.
func (s *LifetimeState) ApplyDefeat(c *DefeatContext, targetID string, rng random.Generator) (DefeatResult, error) {
	if err := s.ValidateDefeat(c, targetID, rng); err != nil {
		return DefeatResult{}, err
	}
	return s.applyDefeat(c, targetID, rng)
}

func (s *LifetimeState) applyDefeat(c *DefeatContext, id string, rng random.Generator) (DefeatResult, error) {
	r := DefeatResult{}
	auto := AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: func(id string) error { return s.ApplyEquipmentReset(&c.EquipmentResetContext, id, rng) }}
	revival, err := s.ApplyAutomaticRevival(&auto, id, rng)
	if err != nil || revival.Prevented {
		r.Prevented = revival.Prevented
		return r, err
	}
	a := s.actor(id)
	tmp := a.Temporary
	a.HP = 0
	if err := s.ApplyEquipmentReset(&c.EquipmentResetContext, id, rng); err != nil {
		return r, err
	}
	a = s.actor(id)
	if tmp == TemporaryEffect("death_mark") {
		a.Temporary = tmp
	}
	r.Defeated = true
	if hasItem(a.ItemDefinitionIDs, "item-212") {
		e := TargetEffect{Payload: CallbackPayload{Symbol: "last_strike"}}
		var target TargetResolution
		if slices.Contains(c.PartyIDs, id) {
			target, err = s.ResolveEnemyTarget(c.TargetContext, e, rng)
		} else if slices.Contains(c.EnemyIDs, id) {
			target, err = s.ResolvePartyTarget(c.TargetContext, e, rng)
		}
		if err != nil {
			return r, err
		}
		if !target.Resolved {
			return r, ErrUnsupportedEffect
		}
		applyStatusResources(s, target)
		if target.Resolved {
			damage := DamageContext{EffectContext: EffectContext{TargetContext: c.TargetContext}, ToolCategory: c.ToolCategory, Defeat: func(id string) error { _, err := s.applyDefeat(c, id, rng); return err }}
			strike, err := s.ApplyToolDamage(&damage, target.TargetID, float64(s.actor(id).Attack)*1.5, false, false, rng)
			if err != nil {
				return r, err
			}
			r.FinalStrike = &strike
		}
	}
	for side, ids := range [][]string{c.PartyIDs, c.EnemyIDs} {
		if len(ids) == 0 || s.actor(id).TeamID != s.actor(ids[0]).TeamID {
			continue
		}
		for _, other := range ids {
			member := s.actor(other)
			if other == id || !hasItem(member.ItemDefinitionIDs, "item-212") {
				continue
			}
			member.Attack += 100
			member.Defense += 100
			member.Agility += 100
			if side == 0 {
				member.Attack = min(999, member.Attack)
				member.Defense = min(999, member.Defense)
				member.Agility = min(999, member.Agility)
			}
		}
	}
	if s.actor(c.ActorID).NPC || id == c.ActorID {
		return r, nil
	}
	if err := c.Rewards.Apply(s, c.TargetContext, id); err != nil {
		return r, err
	}
	r.RewardsReached = true
	if c.FieldAtLoad == "fortress" && s.Fields.AntiTurns == 0 {
		if err := c.Fortress.Apply(s, c.TargetContext, id); err != nil {
			return r, err
		}
	}
	return r, nil
}
