package battle

import "github.com/witchcraze/party2re/internal/core/random"

// WeaponSealEntries are the player-only additions to the previous-job loop.
// _data.cgi:2217–2219; _skill.cgi:219–224,260–283. Costs here are unmodified.
func WeaponSealEntries() []CallbackEntry {
	return []CallbackEntry{
		{ID: "seal-7-breath", Name: "seal-7-breath", RequiredSP: 90, MPCost: 40},
		{ID: "seal-8-magic", Name: "seal-8-magic", RequiredSP: 90, MPCost: 27},
		{ID: "seal-9-magic", Name: "seal-9-magic", RequiredSP: 90, MPCost: 40},
	}
}

func weaponSealEffect(id string) TargetEffect {
	e := TargetEffect{Category: CategoryMagic, Payload: CallbackPayload{Number: 180}, Arguments: []string{"1"}}
	if id == "seal-7-breath" {
		e.Category = CategoryBreath
	} else if id == "seal-8-magic" {
		e.Payload.Number = 160
	}
	return e
}

// ValidateWeaponSeal shares discovery/submission eligibility and whole-area
// admission. The returned MP cost is the outer debit, applied AFTER the effect.
func (s *LifetimeState) ValidateWeaponSeal(c *DamageContext, id string, rng random.Generator) (CallbackEntry, error) {
	if s == nil || c == nil {
		return CallbackEntry{}, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return CallbackEntry{}, err
	}
	a := s.actor(c.ActorID)
	for i, entry := range WeaponSealEntries() {
		if entry.ID != id {
			continue
		}
		if a.NPC || c.ActorNPC || a.Gear == nil || a.Gear.WeaponID == "" || a.WeaponSeal != i+7 || len(c.EnemyIDs) == 0 || hasAbility(a.Abilities, "cursed") || a.Jobs.OldSP < entry.RequiredSP {
			return CallbackEntry{}, ErrUnsupportedEffect
		}
		// All three source names are absent from _is_magician_skill's list
		// (_battle.cgi:1804–1825); magic category alone never halves their cost.
		if hasItem(a.ItemDefinitionIDs, "item-249") {
			entry.MPCost *= 2
		}
		if a.MP < entry.MPCost {
			return CallbackEntry{}, ErrUnsupportedEffect
		}
		if err := s.validateAreaDamage(c, weaponSealEffect(id), rng); err != nil {
			return CallbackEntry{}, err
		}
		return entry, nil
	}
	return CallbackEntry{}, ErrUnsupportedEffect
}

// ApplyWeaponSeal uses the shared ordered target/modifier/reflection/tail
// pipeline. Ordinary MP charging belongs to the consumer (_battle.cgi:761–778).
// Target MP recovery is already applied before damage; do not replay its facts.
func (s *LifetimeState) ApplyWeaponSeal(c *DamageContext, id string, rng random.Generator) ([]EffectApplication, error) {
	if _, err := s.ValidateWeaponSeal(c, id, rng); err != nil {
		return nil, err
	}
	previous := c.targetResources
	c.targetResources = func(r TargetResolution) {
		if r.MPRecoveryTarget != "" {
			t := s.actor(r.MPRecoveryTarget)
			t.MP = min(t.MaxMP, t.MP+r.MPRecovery)
		}
	}
	defer func() { c.targetResources = previous }()
	e := weaponSealEffect(id)
	return s.ApplyAreaDamage(c, e.Payload.Number, e.Category, true, rng)
}
