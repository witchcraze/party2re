package battle

import (
	"github.com/witchcraze/party2re/internal/core/random"
)

func (s *LifetimeState) validateNPCPoison(c *EquipmentTailContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	damage := a.MaxHP / 10
	if damage > 999 {
		damage = 1049
	}
	if a.HP <= 0 || a.HP > damage {
		return nil
	}
	if c.Defeat == nil {
		return ErrUnsupportedEffect
	}
	projection := s.callbackCopy()
	projection.actor(a.ID).HP = 0
	return projection.ValidateDefeat(c.Defeat, a.ID, rng)
}

// BeginNPCAction keeps NPC cures separate from the player extension/tail path.
// Capture equipment/field facts before calling. The enclosing consumer admits
// the action and its continuations before this first state mutation.
func (s *LifetimeState) BeginNPCAction(c *EquipmentTailContext, rng random.Generator) (StatusDecision, error) {
	if c == nil || rng == nil {
		return StatusDecision{}, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return StatusDecision{}, err
	}
	a := s.actor(c.ActorID)
	if err := a.ValidateOrdinaryNPC(); err != nil {
		return StatusDecision{}, err
	}
	if a.Status == StatusKinju {
		if err := s.validateNPCPoison(c, rng); err != nil {
			return StatusDecision{}, err
		}
	}
	if a.Status != "" && ((hasItem(a.ItemDefinitionIDs, "item-246") && rng.Float64() < .5) || a.Extended == ExtendedPurification) {
		a.Status = ""
	}
	decision, err := a.CheckIncapacitation(IncapacitationContext{NPC: true, Bypass: hasItem(a.ItemDefinitionIDs, "item-204"), SpecialBossField: s.SpecialBossField, PhantomField: s.Fields.Current() == "phantom"}, rng)
	if err != nil {
		return StatusDecision{}, err
	}
	if decision.NeedsDefeat {
		if c.Defeat == nil {
			return StatusDecision{}, ErrUnsupportedEffect
		}
		if _, err := s.ApplyDefeat(c.Defeat, a.ID, rng); err != nil {
			return StatusDecision{}, err
		}
	}
	if decision.NormalAction {
		if err := s.actor(c.ActorID).BeginNormalAction(true, false, false, rng); err != nil {
			return StatusDecision{}, err
		}
	}
	return decision, nil
}

// FinishNPCNormalAction is reached only after a normal callback, including an
// accepted no-effect. Deadly poison precedes regeneration; standard poison
// follows it. Incapacitation/cure-only operations skip this entire block.
func (s *LifetimeState) FinishNPCNormalAction(c *EquipmentTailContext, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	if err := a.ValidateOrdinaryNPC(); err != nil {
		return err
	}
	if a.Status == StatusPoison || a.Status == StatusDeadlyPoison || a.Status == StatusVirulentPoison {
		if err := s.validateNPCPoison(c, rng); err != nil {
			return err
		}
	}
	poison := func() error {
		dead, err := s.actor(c.ActorID).ApplyPoison(rng)
		if err != nil || !dead {
			return err
		}
		if c.Defeat == nil {
			return ErrUnsupportedEffect
		}
		_, err = s.ApplyDefeat(c.Defeat, c.ActorID, rng)
		return err
	}
	if a.Status == StatusDeadlyPoison || a.Status == StatusVirulentPoison {
		if err := poison(); err != nil {
			return err
		}
	}
	a = s.actor(c.ActorID)
	if a.HP > 0 && a.Temporary == TemporaryRegeneration {
		amount := 0
		if a.MaxHP > 999 {
			amount = int(rng.Float64() * 100)
		} else {
			amount = int(float64(a.MaxHP) * (.1 + rng.Float64()*.1))
		}
		a.HP = min(a.MaxHP, a.HP+amount)
	}
	if a.Status == StatusPoison {
		if err := poison(); err != nil {
			return err
		}
	}
	a = s.actor(c.ActorID)
	if a.HP > 0 && a.Temporary == TemporaryRage {
		return s.statTension(&c.EffectContext, a, false, rng)
	}
	return nil
}

// FinishNPCTails interleaves the shared effect owners in NPC source order,
// outside the normal-action block. Field lifetime is not ticked a second time.
func (s *LifetimeState) FinishNPCTails(c *EquipmentTailContext, field *NamedFieldContext, tail NamedFieldTail, rng random.Generator) error {
	if c == nil || field == nil || !c.Completion.Accepted || !c.Completion.NPC {
		return ErrInvalidRequest
	}
	if a := s.actor(c.ActorID); a != nil {
		if err := a.ValidateOrdinaryNPC(); err != nil {
			return err
		}
	}
	for _, id := range []string{"item-164-operation-tail", "item-209-operation-tail"} {
		if err := s.ApplyEquipmentTail(c, id, rng); err != nil {
			return err
		}
	}
	field.TargetContext = c.TargetContext
	tail.NPC = true
	if err := s.ApplyNamedFieldTail(field, tail, rng); err != nil {
		return err
	}
	c.TargetContext = field.TargetContext
	for _, id := range []string{"item-206-operation-tail", "item-202-npc-tail", "item-208-combat-tail", "guardian-mp-tail", "item-246-combat-tail"} {
		if err := s.ApplyEquipmentTail(c, id, rng); err != nil {
			return err
		}
	}
	c.Completion.End = c.End
	_, err := s.CompleteOperation(c.Completion, c.PartyIDs)
	return err
}

// ApplySpecialBossField caps party baselines, then uses the shared reset owner.
// Ordinary stage24 floor10 invokes this; EX default fields have another owner.
func (s *LifetimeState) ApplySpecialBossField(c *EquipmentResetContext, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	capParty := func(state *LifetimeState) {
		state.SpecialBossField = true
		for _, id := range c.PartyIDs {
			a := state.actor(id)
			rate := 1
			if a.OverLevel {
				rate = 2
			}
			a.MaxHP = min(a.MaxHP, 999*rate)
			a.MaxMP = min(a.MaxMP, 999*rate)
			a.BaseStats.Attack = min(a.BaseStats.Attack, 255*rate)
			a.BaseStats.Defense = min(a.BaseStats.Defense, 255*rate)
			a.BaseStats.Agility = min(a.BaseStats.Agility, 255*rate)
		}
	}
	projection := s.callbackCopy()
	capParty(&projection)
	for _, id := range c.PartyIDs {
		if err := projection.ValidateEquipmentReset(c, id, rng); err != nil {
			return err
		}
	}
	capParty(s)
	for _, id := range c.PartyIDs {
		if err := s.ApplyEquipmentReset(c, id, rng); err != nil {
			return err
		}
	}
	return nil
}
