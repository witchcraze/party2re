package battle

import (
	"github.com/witchcraze/party2re/internal/core/item"
)

// Target scopes for battle actions and skills.
const (
	TargetScopeSingleEnemy = "single_enemy"
	TargetScopeAllEnemies  = "all_enemies"
	TargetScopeSingleAlly  = "single_ally"
	TargetScopeAllAllies   = "all_allies"
	TargetScopeSelf        = "self"
)

// Action kinds supported by the battle engine.
const (
	ActionKindAttack = "attack"
	ActionKindDefend = "defend"
	ActionKindHeal   = "heal"
	ActionKindBuff   = "buff"
	ActionKindStatus = "status"
)

// ActionItem represents an active item command executed during combat (@どうぐ).
type ActionItem struct {
	ID            string             `json:"id"`
	InstanceID    string             `json:"instance_id,omitempty"`
	Name          string             `json:"name"`
	UsageCategory item.UsageCategory `json:"usage_category"`
	Kind          string             `json:"kind"` // "heal", "buff", "status", "attack"
	Power         int                `json:"power"`
	TargetScope   string             `json:"target_scope"`
	Element       string             `json:"element,omitempty"`
	BuffStat      string             `json:"buff_stat,omitempty"`
	Status        string             `json:"status,omitempty"`
}

// ValidateItemAction verifies if an item action is permissible in combat according to legacy rules.
func ValidateItemAction(it ActionItem) error {
	if !it.UsageCategory.IsUsableInCombatCommand() {
		return ErrCannotUseInCombat
	}
	return nil
}

// Status ailments supported during combat resolution.
const (
	StatusParalyze = "paralyze" // 麻痺: 行動不能
	StatusSleep    = "sleep"    // 眠り: 行動不能
	StatusPoison   = "poison"   // 毒: ターン終了時DOT
)

// ActionSkill represents a job or class skill that can be executed during battle.
type ActionSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MPCost      int    `json:"mp_cost"`
	Power       int    `json:"power"`
	Kind        string `json:"kind"`                // "attack", "defend", "heal", "buff", "status"
	TargetScope string `json:"target_scope"`        // "single_enemy", "all_enemies", "single_ally", "all_allies", "self"
	Element     string `json:"element,omitempty"`   // "fire", "water", etc.
	BuffStat    string `json:"buff_stat,omitempty"` // "attack", "defense", "agility"
	Status      string `json:"status,omitempty"`    // "paralyze", "sleep", "poison"
}

// ActionCustomSkill represents a crafted custom skill blending gems with incantation.
type ActionCustomSkill struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Incantation string      `json:"incantation"` // player quote / shout
	CMPCost     int         `json:"cmp_cost"`
	Gems        []GemEffect `json:"gems"`
}

// GemEffect represents an individual gem's contribution to a custom skill.
type GemEffect struct {
	Kind        string `json:"kind"` // "attack", "heal", "buff", "field", "anti_field"
	Power       int    `json:"power"`
	Element     string `json:"element,omitempty"`
	TargetScope string `json:"target_scope,omitempty"`
	Duration    int    `json:"duration,omitempty"`
	BuffStat    string `json:"buff_stat,omitempty"` // "attack", "defense", "agility"
}
