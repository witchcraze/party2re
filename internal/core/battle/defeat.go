package battle

import "fmt"

// RevivalResult describes the outcome of a defeat revival check.
type RevivalResult struct {
	Revived     bool
	HP          int
	Message     string
	Cursed      bool
	AttackBuff  int
	DefenseBuff int
	AgilityBuff int
}

// CheckRevival inspects participant abilities and resources when HP reaches 0 to determine if revival triggers.
func CheckRevival(p *Participant, currentMP *int) RevivalResult {
	if p == nil {
		return RevivalResult{}
	}
	maxHP := p.MaxHP
	if maxHP <= 0 {
		maxHP = p.HP
	}
	if maxHP <= 0 {
		maxHP = 100
	}

	maxMP := p.MaxMP
	if maxMP <= 0 {
		maxMP = p.MP
	}

	name := p.Name
	if name == "" {
		name = p.ID
	}

	abilities := append([]string(nil), p.Abilities...)
	if hasItem(p.ItemDefinitionIDs, "item-260") && !hasAbility(abilities, "cursed") {
		abilities = append(abilities, "cursed_revive")
	}
	for _, ab := range abilities {
		switch ab {
		case "undying", "revive":
			hp := maxHP / 5
			if hp < 1 {
				hp = 1
			}

			return RevivalResult{
				Revived: true,
				HP:      hp,
				Message: fmt.Sprintf("%sは瀕死でよみがえった！", name),
			}
		case "pharaoh":
			if currentMP != nil && *currentMP > maxMP/4 {
				cost := *currentMP / 2
				*currentMP -= cost
				return RevivalResult{
					Revived: true,
					HP:      maxHP,
					Message: fmt.Sprintf("%sは不死の呪いでよみがえった！", name),
				}
			}
		case "touki_shield", "dokuro_amulet":
			hp := maxHP / 2
			if hp < 1 {
				hp = 1
			}
			return RevivalResult{
				Revived: true,
				HP:      hp,
				Message: fmt.Sprintf("%sは瀕死でよみがえった！", name),
			}
		case "cursed_revive":
			hp := maxHP * 3 / 10
			if hp < 1 {
				hp = 1
			}
			return RevivalResult{
				Revived:     true,
				HP:          hp,
				Cursed:      true,
				AttackBuff:  300,
				DefenseBuff: 300,
				AgilityBuff: 300,
				Message:     fmt.Sprintf("%sは瀕死でよみがえった！ %sは呪われた！", name, name),
			}
		}
	}
	return RevivalResult{}
}

func hasAbility(abilities []string, wanted string) bool {
	for _, ability := range abilities {
		if ability == wanted {
			return true
		}
	}
	return false
}
