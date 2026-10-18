package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// AutomaticRevivalContext supplies the same certified reset used by defeat.
// Its owner admits current equipment for the separate requester/actor/target,
// including possible HP0 resets; item110's fallen reset remains unsupported.
type AutomaticRevivalContext struct {
	TargetContext
	Reset func(string) error
}

// AutomaticRevivalResult controls the enclosing defeat's early return. A
// prevention rounded to HP0 still returns before final defeat/reward handling.
type AutomaticRevivalResult struct {
	Prevented bool
}

func automaticRevivalBranch(a *LifetimeActor) string {
	if a.Temporary == TemporaryRevival {
		return "temporary"
	}
	if hasAbility(a.Abilities, "pharaoh") && float64(a.MP) > float64(a.MaxMP)*.25 {
		return "pharaoh"
	}
	if (hasItem(a.ItemDefinitionIDs, "item-161") && a.Tension >= 8) || hasItem(a.ItemDefinitionIDs, "item-193") {
		return "half"
	}
	if hasItem(a.ItemDefinitionIDs, "item-260") && !hasAbility(a.Abilities, "cursed") {
		return "curse"
	}
	return ""
}

// ValidateAutomaticRevival is pure. This primitive owns only the prefix of
// defeat through revival, not final strikes, rewards or fortress effects.
func (s *LifetimeState) ValidateAutomaticRevival(c *AutomaticRevivalContext, targetID string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	a := s.actor(targetID)
	if a == nil || a.MaxHP < 0 || a.MaxHP > 1e12 || a.MaxMP < 0 || a.MaxMP > 1e12 || a.MP < 0 || a.MP > 1e12 {
		return ErrInvalidParticipant
	}
	branch := automaticRevivalBranch(a)
	if branch != "" && c.Reset == nil {
		return ErrUnsupportedEffect
	}
	if branch == "temporary" || branch == "pharaoh" {
		return nil
	}
	for _, member := range s.Actors {
		if member.ID != targetID && member.Gear == nil {
			return ErrUnsupportedEffect
		}
	}
	return nil
}

// ApplyAutomaticRevival implements _battle.cgi:28–87 in source precedence.
// The enclosing defeat calls this even when a sacrifice has positive HP and
// returns immediately on Prevented. It must not award defeat rewards here.
func (s *LifetimeState) ApplyAutomaticRevival(c *AutomaticRevivalContext, targetID string, rng random.Generator) (AutomaticRevivalResult, error) {
	r := AutomaticRevivalResult{}
	if err := s.ValidateAutomaticRevival(c, targetID, rng); err != nil {
		return r, err
	}
	a := s.actor(targetID)
	branch := automaticRevivalBranch(a)
	if branch != "temporary" && branch != "pharaoh" {
		for i := range s.Actors {
			member := &s.Actors[i]
			if member.ID != targetID && member.Gear.WeaponID == "weapon-37" && member.Gear.ArmorID == "armor-38" {
				member.Attack = min(999, member.Attack+int(float64(a.Attack)*.5))
			}
		}
	}
	if branch == "" {
		return r, nil
	}
	a.Temporary = ""
	switch branch {
	case "temporary":
		a.HP = int(float64(a.MaxHP) * (.1 + rng.Float64()*.1))
	case "pharaoh":
		a.Status, a.HP = "", a.MaxHP
		a.MP -= a.MP / 2
	case "half":
		a.HP = a.MaxHP / 2
	case "curse":
		a.Status, a.HP = "", int(float64(a.MaxHP)*.3)
	}
	if err := c.Reset(targetID); err != nil {
		return r, err
	}
	a = s.actor(targetID)
	if branch == "half" && hasItem(a.ItemDefinitionIDs, "item-193") {
		// Reset sees the accessory; consumption and immobilization follow it.
		a.ItemDefinitionIDs = slices.DeleteFunc(slices.Clone(a.ItemDefinitionIDs), func(id string) bool { return id == "item-193" })
		a.Abilities = slices.DeleteFunc(slices.Clone(a.Abilities), func(id string) bool { return id == "dokuro_amulet" })
		if a.Gear != nil {
			gear := *a.Gear
			gear.AccessoryID = ""
			a.Gear = &gear
		}
		a.ConsumedItems = append(a.ConsumedItems, ConsumedItem{ID: "item-193", Quantity: 1})
		a.Status = StatusDofuu
	}
	if branch == "curse" {
		a.Attack += 300
		a.Defense += 300
		a.Agility += 300
		if !hasAbility(a.Abilities, "cursed") {
			a.Abilities = append(a.Abilities, "cursed")
		}
	}
	r.Prevented = true
	return r, nil
}
