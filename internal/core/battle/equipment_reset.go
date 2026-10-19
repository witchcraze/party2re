package battle

import (
	"fmt"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
)

// WeaponSealTable holds source table-load facts, shared by all resets in the
// request. Seal3 uses the requester's table weapon; seal6 draws once at load.
type WeaponSealTable struct {
	DragonAttack, PhoenixWeight int
}

type EquipmentResetContext struct {
	TargetContext
	RequesterLevel *int
	Seals          *WeaponSealTable
	Cursor         TargetCursor
}

// ValidateEquipmentReset admits the current target, including fallen targets.
// Enclosing effects must also admit their reachable HP/context transitions.
func (s *LifetimeState) ValidateEquipmentReset(c *EquipmentResetContext, targetID string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	a := s.actor(targetID)
	if a == nil {
		return ErrInvalidParticipant
	}
	if a.Gear == nil || (a.Gear.WeaponID != "" && a.Gear.WeaponAttack == nil) {
		return ErrUnsupportedEffect
	}
	if a.WeaponSeal < 0 || a.WeaponSeal > 12 {
		return ErrUnsupportedEffect
	}
	if (a.WeaponSeal == 3 || a.WeaponSeal == 6) && c.Seals == nil {
		return ErrUnsupportedEffect
	}
	if c.Seals != nil && (c.Seals.PhoenixWeight > 0 || c.Seals.PhoenixWeight < -49) {
		return ErrUnsupportedEffect
	}
	switch a.Gear.WeaponID {
	case "weapon-69", "weapon-70", "weapon-71":
		if c.RequesterLevel == nil || *c.RequesterLevel < 0 {
			return ErrUnsupportedEffect
		}
	}
	if a.Gear.AccessoryID == "" {
		return nil
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return err
	}
	d, err := catalog.FindByID(a.Gear.AccessoryID)
	if err != nil {
		return ErrUnsupportedEffect
	}
	if d.UsageCategory != item.UsageCategoryCombatPassive {
		return nil
	}
	if a.Gear.AccessoryID == "item-110" {
		return ErrUnsupportedEffect
	}
	if a.Gear.AccessoryID == "item-187" {
		_, err := resetJobWeights(a.Jobs.JobID)
		return err
	}
	if !resetAccessorySupported(a.Gear.AccessoryID) {
		return ErrUnsupportedEffect
	}
	return nil
}

// ApplyEquipmentReset is the single native reset path: baselines, accessory,
// weapon revision, source seal table, then agility floor. HP/MP callbacks run
// here even when they exceed MaxHP/MaxMP; they retain their own living guards.
func (s *LifetimeState) ApplyEquipmentReset(c *EquipmentResetContext, targetID string, rng random.Generator) error {
	if err := s.ValidateEquipmentReset(c, targetID, rng); err != nil {
		return err
	}
	a := s.actor(targetID)
	attack := 0
	if a.Gear.WeaponAttack != nil {
		attack = *a.Gear.WeaponAttack
	}
	a.ResetEffects(LifetimeReset{Attack: a.BaseStats.Attack + attack, Defense: a.BaseStats.Defense + a.Gear.ArmorDefense, Agility: a.BaseStats.Agility - a.Gear.WeaponWeight - a.Gear.ArmorWeight})
	// ResetEffects floors AG for its historical fact-only callers. Accessory
	// callbacks need the unfloored value until the last step of this full reset.
	a.Agility = a.BaseStats.Agility - a.Gear.WeaponWeight - a.Gear.ArmorWeight
	if err := s.applyResetAccessory(c, a, rng); err != nil {
		return err
	}
	switch a.Gear.WeaponID {
	case "weapon-69":
		a.Attack = int(float64(a.Attack) + float64(min(99, *c.RequesterLevel))*1.5)
	case "weapon-70":
		a.Attack += min(99, *c.RequesterLevel) * 2
	case "weapon-71":
		component := a.Attack - a.BaseStats.Attack
		if resetAwakened(a, s.actor(c.ActorID)) {
			component = int(float64(a.BaseStats.Attack) * (1.5 + rng.Float64()*.5))
		}
		factor := 1.0
		if *c.RequesterLevel < 50 {
			factor = .5
		} else if *c.RequesterLevel < 75 {
			factor = .7
		}
		a.Attack = int(float64(a.BaseStats.Attack) + float64(component)*factor)
	}
	switch a.WeaponSeal {
	case 1:
		a.Attack += 10
	case 2:
		a.Attack += 30
		a.Agility -= 20
	case 3:
		a.Attack += c.Seals.DragonAttack
		a.Agility -= 30
	case 4:
		a.Agility += 10
	case 5:
		a.Attack -= 10
		a.Agility += 30
	case 6:
		a.Attack -= 20
		a.Agility -= c.Seals.PhoenixWeight
	}
	a.Agility = max(0, a.Agility)
	return nil
}

func resetAwakened(target, actor *LifetimeActor) bool {
	return target.Gear.AccessoryID == "item-158" || target.Gear.AccessoryID == "item-240" || target.Gear.AccessoryID == "item-241" || target.Gear.AccessoryID == "item-242" || actor.Extended == ExtendedAwakening
}

func resetJobWeights(id string) (BattleBaseStats, error) {
	catalog, err := job.InitialCatalog()
	if err != nil {
		return BattleBaseStats{}, err
	}
	n := callbackJob(id)
	if n < 1 || n > 88 {
		return BattleBaseStats{}, nil
	}
	d, err := catalog.FindByID(fmt.Sprintf("job-%02d", n))
	if err != nil || d.AttackGrowth+d.DefenseGrowth+d.AgilityGrowth == 0 {
		return BattleBaseStats{}, ErrUnsupportedEffect
	}
	return BattleBaseStats{d.AttackGrowth, d.DefenseGrowth, d.AgilityGrowth}, nil
}
