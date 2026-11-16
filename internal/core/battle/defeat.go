package battle

// RevivalResult describes the outcome of a defeat revival check.
type RevivalResult struct {
	Revived      bool          `json:"revived"`
	HP           int           `json:"hp"`
	Source       string        `json:"source"`
	Cursed       bool          `json:"cursed"`
	Status       string        `json:"status"`
	AttackBuff   int           `json:"attack_buff"`
	DefenseBuff  int           `json:"defense_buff"`
	AgilityBuff  int           `json:"agility_buff"`
	ConsumedItem *ConsumedItem `json:"consumed_item"`
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
				Source:  ab,
			}
		case "pharaoh":
			if currentMP != nil && *currentMP > maxMP/4 {
				cost := *currentMP / 2
				*currentMP -= cost
				return RevivalResult{
					Revived: true,
					HP:      maxHP,
					Source:  ab,
				}
			}
		case "touki_shield":
			hp := maxHP / 2
			if hp < 1 {
				hp = 1
			}
			return RevivalResult{
				Revived: true,
				HP:      hp,
				Source:  ab,
			}
		case "dokuro_amulet":
			hp := maxHP / 4
			if hp < 1 {
				hp = 1
			}
			return RevivalResult{
				Revived:      true,
				ConsumedItem: &ConsumedItem{ID: "item-193", Quantity: 1},
				HP:           hp,
				Status:       StatusDofuu,
				Source:       ab,
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
				Source:      ab,
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
