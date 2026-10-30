package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// PlayerTailContext is rebuilt per operation. Completion carries source block
// eligibility; NormalAction means the normal block was reached, even on failure.
// Item210Fallen is frozen at entry. EffectContext.RequesterLevel belongs to the
// requester, independently of the acting member. Defeat owns reset/revival ports.
type PlayerTailContext struct {
	DamageContext
	Completion    LifetimeCompletion
	NormalAction  bool
	Item210Fallen []string
	DefeatContext *DefeatContext
}

// PlayerTailEntries are passive producer identities, never submitted commands,
// job tables, random skills or batch scalar payloads. Source: _battle.cgi:
// 615–622,829–832,923–992,1039–1069,1091–1095.
func PlayerTailEntries() []string {
	return []string{
		"item-189-normal-tail", "item-210-revival-tail", "item-183-combat-tail",
		"item-194-combat-tail", "item-202-combat-tail", "item-198-combat-tail",
		"weapon-35-armor-37-tail", "weapon-69-armor-54-tail", "weapon-70-armor-55-tail",
		"weapon-30-tail", "weapon-54-tail", "cmp-recharge-tail", "frenzy-tail", "crown-tail",
	}
}

// CaptureItem210 runs before action callbacks, preserving party order and fallen
// membership even if the holder loses equipment or an ally revives mid-operation.
func (s *LifetimeState) CaptureItem210(c TargetContext) ([]string, error) {
	if s == nil {
		return nil, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c, random.Default()); err != nil {
		return nil, err
	}
	if s.actor(c.ActorID).NPC || !hasItem(s.actor(c.ActorID).ItemDefinitionIDs, "item-210") {
		return nil, nil
	}
	var fallen []string
	for _, id := range c.PartyIDs {
		if s.actor(id).HP <= 0 {
			fallen = append(fallen, id)
		}
	}
	return fallen, nil
}

func (c *PlayerTailContext) combatTail() bool {
	f := c.Completion
	return f.EnemiesRemain && !f.Suppressed && !f.NPCComment && !f.LevelUp
}

func (s *LifetimeState) playerTailReached(c *PlayerTailContext, id string) bool {
	f, a := c.Completion, s.actor(c.ActorID)
	if !f.Accepted || f.NPC || c.ActorNPC || a.NPC {
		return false
	}
	switch id {
	case "item-189-normal-tail":
		return c.NormalAction && a.HP > 0 && hasItem(a.ItemDefinitionIDs, "item-189") && float64(a.MP) <= float64(a.MaxMP)*.3
	case "item-210-revival-tail":
		return !f.Suppressed && len(c.Item210Fallen) > 0
	}
	if !c.combatTail() {
		return false
	}
	switch id {
	case "cmp-recharge-tail":
		return true
	case "frenzy-tail":
		return a.Extended == ExtendedFrenzy && a.HP > 0
	case "crown-tail":
		return a.Extended == ExtendedCrown && c.DamageCalls > 0
	}
	if a.HP <= 0 {
		return false
	}
	switch id {
	case "item-183-combat-tail":
		return hasItem(a.ItemDefinitionIDs, "item-183") && a.Agility < a.BaseStats.Agility*3
	case "item-194-combat-tail":
		return hasItem(a.ItemDefinitionIDs, "item-194")
	case "item-202-combat-tail":
		return hasItem(a.ItemDefinitionIDs, "item-202") && a.HP <= a.MaxHP/2
	case "item-198-combat-tail":
		return hasItem(a.ItemDefinitionIDs, "item-198") && a.MP <= a.MaxMP/2
	case "weapon-35-armor-37-tail":
		return hasItem(a.ItemDefinitionIDs, "weapon-35") && hasItem(a.ItemDefinitionIDs, "armor-37") && float64(a.HP) <= float64(a.MaxHP)*.3
	case "weapon-69-armor-54-tail":
		return hasItem(a.ItemDefinitionIDs, "weapon-69") && hasItem(a.ItemDefinitionIDs, "armor-54")
	case "weapon-70-armor-55-tail":
		return hasItem(a.ItemDefinitionIDs, "weapon-70") && hasItem(a.ItemDefinitionIDs, "armor-55")
	case "weapon-30-tail", "weapon-54-tail":
		weapon := "weapon-30"
		if id == "weapon-54-tail" {
			weapon = "weapon-54"
		}
		return hasItem(a.ItemDefinitionIDs, weapon) && c.HealCalls == 0 && c.DamageCalls > 0
	}
	return false
}

// ApplyPlayerTail runs one source boundary after complete native admission.
// Consumers admit the enclosing operation before charging and compose other
// owners at their original positions. Save results with a receipt; never replay
// these callbacks to reload/retry. Counters are finished once by CompleteOperation.
func (s *LifetimeState) ApplyPlayerTail(c *PlayerTailContext, id string, rng random.Generator) error {
	if err := s.ValidatePlayerTail(c, id, rng); err != nil {
		return err
	}
	if !s.playerTailReached(c, id) {
		return nil
	}
	a := s.actor(c.ActorID)
	switch id {
	case "item-189-normal-tail":
		// The source draws before testing suppression, even when it skips healing.
		if rng.Float64() < .2 && !c.Completion.Suppressed {
			power := min(1000, int(float64(a.MaxMP)*(.15+rng.Float64()*.1)))
			_, err := s.ApplyToolMPHealing(c.TargetContext, a.ID, float64(power), false, rng)
			return err
		}
	case "item-210-revival-tail":
		for _, target := range c.Item210Fallen {
			t := s.actor(target)
			var err error
			if t.HP <= t.MP || t.MP <= 0 {
				_, err = s.ApplyToolHealing(c.TargetContext, target, 300, false, rng)
			} else {
				_, err = s.ApplyToolMPHealing(c.TargetContext, target, 300, false, rng)
			}
			if err != nil {
				return err
			}
		}
	case "item-183-combat-tail":
		a.Agility += int(float64(a.BaseStats.Agility) * (.2 + rng.Float64()*.3))
	case "item-194-combat-tail":
		return s.applyResetTail(c, rng)
	case "item-202-combat-tail":
		_, err := s.ApplyToolHealing(c.TargetContext, a.ID, float64(a.MaxHP)*.04, true, rng)
		return err
	case "item-198-combat-tail":
		_, err := s.ApplyToolMPHealing(c.TargetContext, a.ID, float64(a.MaxMP)*.02, true, rng)
		return err
	case "weapon-35-armor-37-tail":
		_, err := s.ApplyToolHealing(c.TargetContext, a.ID, float64(a.MaxHP)*.1, false, rng)
		return err
	case "weapon-69-armor-54-tail", "weapon-70-armor-55-tail":
		if rng.Float64() < 1.0/6 && a.Tension < 2 {
			return s.statTension(&c.EffectContext, a, false, rng)
		}
	case "weapon-30-tail", "weapon-54-tail":
		_, err := s.ApplyToolHealing(c.TargetContext, a.ID, float64(a.MaxHP)*(.02+rng.Float64()*.02), false, rng)
		return err
	case "cmp-recharge-tail":
		if a.CMPCharge >= a.CMPNeedCharge {
			amount := int(rng.Float64()*float64(max(1, min(99, *c.RequesterLevel)))*.4) + 1
			if hasItem(a.ItemDefinitionIDs, "item-238") {
				amount = int(float64(amount) * 1.5)
			}
			c.Completion.CMPAmount, c.Completion.NextCMPThreshold = amount, int(rng.Float64()*7)+3
		}
	case "frenzy-tail":
		a.Attack = min(a.BaseStats.Attack*5, a.Attack+int(float64(a.BaseStats.Attack)*.6))
		if c.DefeatContext != nil {
			c.DefeatContext.Cursor = c.Cursor
		}
		damage := s.nativeDamageContext(c.DamageContext, c.DefeatContext, rng)
		_, err := s.ApplyToolDamage(&damage, s.randomMember(rng), float64(a.Attack), false, false, rng)
		c.End = damage.End
		if c.DefeatContext != nil {
			c.Cursor = c.DefeatContext.Cursor
		}
		return err
	case "crown-tail":
		if float64(c.DamageCalls) > rng.Float64()*float64(c.DamageCalls+4) {
			_, err := s.ApplyToolHealing(c.TargetContext, a.ID, 80+rng.Float64()*50, false, rng)
			return err
		}
	}
	return nil
}

func (s *LifetimeState) applyResetTail(c *PlayerTailContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	before := *a
	reset := c.DefeatContext.EquipmentResetContext
	reset.TargetContext, reset.Cursor = c.TargetContext, c.Cursor
	if err := s.ApplyEquipmentReset(&reset, a.ID, rng); err != nil {
		return err
	}
	c.Cursor, c.DefeatContext.Cursor = reset.Cursor, reset.Cursor
	difference := a.Attack + a.Defense + a.Agility - before.Attack - before.Defense - before.Agility
	defer func() {
		a := s.actor(c.ActorID)
		a.Tension, a.Status, a.Temporary, a.Hit = before.Tension, before.Status, before.Temporary, before.Hit
	}()
	if difference > 0 {
		damage := s.nativeDamageContext(c.DamageContext, c.DefeatContext, rng)
		damage.targetResources = func(r TargetResolution) { applyStatusResources(s, r) }
		_, err := s.ApplyDamage(&damage, dualAgilityEffect("", float64(difference)*.5, CategoryUntyped, true), rng)
		c.EffectContext, c.End = damage.EffectContext, damage.End
		c.Cursor = c.DefeatContext.Cursor
		return err
	}
	if difference < 0 {
		previous := c.EffectContext.targetResources
		c.EffectContext.targetResources = func(r TargetResolution) { applyStatusResources(s, r) }
		defer func() { c.EffectContext.targetResources = previous }()
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(a.ID, float64(-difference)*.5, CategoryUntyped), rng)
		return err
	}
	return nil
}

// ValidatePlayerTail is also the discovery gate. Unknown identities reject even
// in skipped blocks; supported ineligible/no-effect entries remain accepted.
func (s *LifetimeState) ValidatePlayerTail(c *PlayerTailContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if !slices.Contains(PlayerTailEntries(), id) {
		return ErrUnsupportedEffect
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if c.Completion.ActorID != c.ActorID {
		return ErrInvalidRequest
	}
	if !s.playerTailReached(c, id) {
		return nil
	}
	return s.validateReachedPlayerTail(c, id, rng)
}
