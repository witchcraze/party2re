package battle

import "errors"

var ErrUnsupportedEffect = errors.New("battle effect is unsupported")

// ValidateSkillAction checks the complete native payload, not legacy catalog parity.
// Random/delegated callbacks use the explicit invocation boundary; batch scalar
// payloads deliberately cannot enable them.
func ValidateSkillAction(sk ActionSkill) error {
	if sk.MPCost < 0 || sk.Power < 0 {
		return ErrUnsupportedEffect
	}
	return validateEffect(sk.Kind, sk.TargetScope, sk.BuffStat, sk.Status)
}

// ValidateCustomSkillAction admits all slots before any slot can mutate battle state.
func ValidateCustomSkillAction(cs ActionCustomSkill) error {
	if len(cs.Gems) == 0 || cs.CMPCost < 0 {
		return ErrUnsupportedEffect
	}
	for _, gem := range cs.Gems {
		if gem.Power < 0 {
			return ErrUnsupportedEffect
		}
		switch gem.Kind {
		case "field":
			switch gem.Element {
			case ElementFire, ElementWater, ElementWind, ElementEarth, ElementLight, ElementDark:
			default:
				return ErrUnsupportedEffect
			}
			if gem.Duration <= 0 {
				return ErrUnsupportedEffect
			}
		case "anti_field":
			if gem.Duration <= 0 {
				return ErrUnsupportedEffect
			}
		case ActionKindAttack, ActionKindHeal, ActionKindBuff:
			if gem.TargetScope == TargetScopeSelf {
				return ErrUnsupportedEffect // Custom executor has no explicit self-target branch.
			}
			if err := validateEffect(gem.Kind, gem.TargetScope, gem.BuffStat, ""); err != nil {
				return err
			}
		default:
			return ErrUnsupportedEffect
		}
	}
	return nil
}

func validateEffect(kind, scope, stat, status string) error {
	switch kind {
	case ActionKindAttack, ActionKindStatus, ActionKindDejon:
		if scope != "" && scope != TargetScopeSingleEnemy && scope != TargetScopeAllEnemies {
			return ErrUnsupportedEffect
		}
	case ActionKindHeal, ActionKindBuff:
		if scope != "" && scope != TargetScopeSingleAlly && scope != TargetScopeAllAllies && scope != TargetScopeSelf {
			return ErrUnsupportedEffect
		}
	case ActionKindDefend:
		if scope != "" && scope != TargetScopeSelf {
			return ErrUnsupportedEffect
		}
	default:
		return ErrUnsupportedEffect
	}
	if kind == ActionKindBuff && stat != "" && stat != "attack" && stat != "defense" && stat != "agility" {
		return ErrUnsupportedEffect
	}
	if kind == ActionKindStatus {
		switch status {
		case StatusParalyze, StatusSleep, StatusPoison, StatusDofuu, StatusKinju, StatusSabaku, StatusConfusion, StatusDeadlyPoison, StatusVirulentPoison:
		default:
			return ErrUnsupportedEffect
		}
	}
	return nil
}
