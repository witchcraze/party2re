package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// EquipmentTailSnapshot is frozen before action/status callbacks, independently
// of later equipment loss, cures and temporary expiry. Persist it with the input.
type EquipmentTailSnapshot struct {
	Item206      bool `json:"item_206"`
	HadTemporary bool `json:"had_temporary"`
}

// EquipmentTailContext is rebuilt on reload. Consumers interleave these passive
// boundaries with field/player/extended owners, then finish counters once.
type EquipmentTailContext struct {
	DamageContext
	Completion LifetimeCompletion
	Snapshot   EquipmentTailSnapshot
	Defeat     *DefeatContext
	CureCalls  int
}

// EquipmentTailEntries certifies _battle.cgi:862–921,984–989,1007–1038,1130–1134
// and _npc_action.cgi:366–464. NPC202 has a separate formula from PlayerTails202.
func EquipmentTailEntries() []string {
	return []string{"item-164-operation-tail", "item-209-operation-tail",
		"item-206-operation-tail", "item-202-npc-tail", "item-208-combat-tail",
		"guardian-mp-tail", "item-246-combat-tail"}
}

func (s *LifetimeState) CaptureEquipmentTail(c TargetContext) (EquipmentTailSnapshot, error) {
	if s == nil {
		return EquipmentTailSnapshot{}, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c, random.Default()); err != nil {
		return EquipmentTailSnapshot{}, err
	}
	a := s.actor(c.ActorID)
	return EquipmentTailSnapshot{Item206: hasItem(a.ItemDefinitionIDs, "item-206") && a.Status != "", HadTemporary: a.Temporary != ""}, nil
}

func (s *LifetimeState) equipmentTailReached(c *EquipmentTailContext, id string) bool {
	a, f := s.actor(c.ActorID), c.Completion
	if !f.Accepted || (!a.NPC && f.Suppressed) {
		return false
	}
	switch id {
	case "item-164-operation-tail":
		return hasItem(a.ItemDefinitionIDs, "item-164") && a.Temporary != ""
	case "item-209-operation-tail":
		return hasItem(a.ItemDefinitionIDs, "item-209") && a.Temporary == TemporaryCovering
	case "item-206-operation-tail":
		return c.Snapshot.Item206 && a.Status == ""
	case "item-202-npc-tail":
		return a.NPC && hasItem(a.ItemDefinitionIDs, "item-202") && a.Temporary == ""
	}
	if !a.NPC && (!f.EnemiesRemain || f.NPCComment || f.LevelUp) {
		return false
	}
	switch id {
	case "item-208-combat-tail":
		return hasItem(a.ItemDefinitionIDs, "item-208")
	case "guardian-mp-tail":
		if !a.NPC && a.HP <= 0 {
			return false
		}
		for _, id := range c.PartyIDs {
			if s.actor(id).RosterIdentity == "guardian" {
				return true // Presence includes fallen helpers.
			}
		}
	case "item-246-combat-tail":
		return hasItem(a.ItemDefinitionIDs, "item-246") && !c.Snapshot.HadTemporary && a.HP > 0
	}
	return false
}

// ValidateEquipmentTail shares discovery's gate. Unknown identities reject even
// in skipped blocks; accepted no-effect entries consume no committed RNG.
func (s *LifetimeState) ValidateEquipmentTail(c *EquipmentTailContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if !slices.Contains(EquipmentTailEntries(), id) {
		return ErrUnsupportedEffect
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if c.Completion.ActorID != c.ActorID || c.Completion.NPC != s.actor(c.ActorID).NPC || c.ActorNPC != c.Completion.NPC {
		return ErrInvalidRequest
	}
	if !s.equipmentTailReached(c, id) {
		return nil
	}
	return s.validateReachedEquipmentTail(c, id, rng)
}

// ApplyEquipmentTail runs exactly one source position after whole-body admission.
// It never runs poison/regeneration, ticks a field or completes an operation.
// The consumer admits enclosing continuations before cost and saves a receipt;
// reload/retry returns saved facts instead of executing this callback again.
func (s *LifetimeState) ApplyEquipmentTail(c *EquipmentTailContext, id string, rng random.Generator) error {
	if err := s.ValidateEquipmentTail(c, id, rng); err != nil {
		return err
	}
	if !s.equipmentTailReached(c, id) {
		return nil
	}
	a := s.actor(c.ActorID)
	switch id {
	case "item-164-operation-tail":
		a.Temporary = ""
		if rng.Float64() < .25 {
			return s.statTension(&c.EffectContext, a, false, rng)
		}
	case "item-209-operation-tail":
		_, err := s.ApplyToolHealing(c.TargetContext, a.ID, float64(a.MaxHP)*.03, false, rng)
		return err
	case "item-206-operation-tail":
		tension := a.Tension
		a.Tension = 1
		defer func() { s.actor(c.ActorID).Tension = tension }()
		child := c.EffectContext
		previous := child.targetResources
		child.targetResources = func(r TargetResolution) { applyStatusResources(s, r) }
		_, err := s.ApplyHealing(&child, controlEffect(a.ID, float64(a.MaxHP)*.3, CategoryUntyped), rng)
		c.EffectContext = child
		c.EffectContext.targetResources = previous
		if err != nil {
			return err
		}
		if rng.Float64() < .5 {
			return s.equipmentTailHit(c, float64(s.actor(c.ActorID).Attack), rng)
		}
	case "item-202-npc-tail":
		_, err := s.ApplyToolHealing(c.TargetContext, a.ID, float64(a.MaxHP)*(.03+rng.Float64()*.02), false, rng)
		return err
	case "item-208-combat-tail":
		return s.applyEquipment208(c, rng)
	case "guardian-mp-tail":
		_, err := s.ApplyToolMPHealing(c.TargetContext, a.ID, float64(a.MaxMP)*(.005+rng.Float64()*.03), false, rng)
		return err
	case "item-246-combat-tail":
		kinds := [...]TemporaryEffect{TemporaryReflectAttack, TemporaryReflectMagic, TemporaryReflectDance, TemporaryReflectBreath}
		a.Temporary = kinds[int(rng.Float64()*float64(len(kinds)))]
	}
	return nil
}

func (s *LifetimeState) equipmentTailHit(c *EquipmentTailContext, power float64, rng random.Generator) error {
	if c.Defeat != nil {
		c.Defeat.Cursor = c.Cursor
	}
	damage := s.nativeDamageContext(c.DamageContext, c.Defeat, rng)
	damage.targetResources = func(r TargetResolution) { applyStatusResources(s, r) }
	_, err := s.ApplyDamage(&damage, dualAgilityEffect("", power, CategoryUntyped, true), rng)
	c.EffectContext, c.End = damage.EffectContext, damage.End
	if c.Defeat != nil {
		c.Cursor = c.Defeat.Cursor
	}
	return err
}

func (s *LifetimeState) applyEquipment208(c *EquipmentTailContext, rng random.Generator) error {
	count := len(c.PartyIDs)
	if count >= 3 {
		for _, id := range c.PartyIDs {
			t := s.actor(id)
			if hasItem(t.ItemDefinitionIDs, "item-208") && t.Status != "" {
				child := StatusContext{EffectContext: c.EffectContext, CureCalls: c.CureCalls}
				previous := child.targetResources
				child.targetResources = func(r TargetResolution) { applyStatusResources(s, r) }
				_, err := s.ApplyCure(&child, controlSymbol(id, t.Status, CategoryUntyped), rng)
				c.EffectContext, c.CureCalls = child.EffectContext, child.CureCalls
				c.EffectContext.targetResources = previous
				if err != nil {
					return err
				}
			}
			s.actor(id).Hit = max(95, s.actor(id).Hit)
		}
	}
	if count >= 4 {
		tension := s.actor(c.ActorID).Tension
		err := func() error {
			s.actor(c.ActorID).Tension = 1
			defer func() { s.actor(c.ActorID).Tension = tension }()
			for range count {
				if err := s.equipmentTailHit(c, float64(s.actor(c.ActorID).Attack)*.1, rng); err != nil {
					return err
				}
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	if count >= 5 {
		for _, id := range c.PartyIDs {
			if _, err := s.ApplyToolHealing(c.TargetContext, id, min(float64(s.actor(id).MaxHP)*.1, 200), false, rng); err != nil {
				return err
			}
		}
	}
	if count >= 6 {
		for _, id := range c.PartyIDs {
			if rng.Float64() < 1.0/3 {
				if err := s.statTension(&c.EffectContext, s.actor(id), false, rng); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
