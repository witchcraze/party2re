package battle

import (
	"fmt"
)

func (ctx *battleContext) executeItem(actor Participant, it *ActionItem, allyParty []Participant, opponents []Participant) error {
	if err := ValidateItemAction(*it); err != nil {
		return err
	}

	// Consume item from actor's inventory in this battle
	items := ctx.itemsMap[actor.ID]
	for i, item := range items {
		if item.ID == it.ID {
			ctx.itemsMap[actor.ID] = append(items[:i], items[i+1:]...)
			break
		}
	}

	switch it.Kind {
	case ActionKindHeal:
		var healTargets []Participant
		if it.TargetScope == TargetScopeAllAllies {
			healTargets = allyParty
		} else if it.TargetScope == TargetScopeSelf {
			healTargets = []Participant{actor}
		} else {
			lowest := findLowestHPTarget(allyParty, ctx.hpMap)
			if lowest != nil {
				healTargets = []Participant{*lowest}
			}
		}
		for _, tgt := range healTargets {
			healAmt := it.Power
			ctx.hpMap[tgt.ID] += healAmt
			maxHP := tgt.MaxHP
			if maxHP <= 0 {
				maxHP = tgt.HP
			}
			if ctx.hpMap[tgt.ID] > maxHP {
				ctx.hpMap[tgt.ID] = maxHP
			}
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  it.Name,
				TargetID:    tgt.ID,
				HealingDone: healAmt,
				Message:     fmt.Sprintf("%s は %s をつかった！ %s のHPが %d 回復した！", actor.NameOrID(), it.Name, tgt.NameOrID(), healAmt),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		}

	case ActionKindBuff:
		var buffTargets []Participant
		if it.TargetScope == TargetScopeAllAllies {
			buffTargets = allyParty
		} else {
			buffTargets = []Participant{actor}
		}
		for _, tgt := range buffTargets {
			stat := it.BuffStat
			if stat == "" {
				stat = "defense"
			}
			switch stat {
			case "attack":
				ctx.attackBuff[tgt.ID] += it.Power
			case "agility":
				ctx.agilityBuff[tgt.ID] += it.Power
			default:
				ctx.defenseBuff[tgt.ID] += it.Power
			}
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  it.Name,
				TargetID:    tgt.ID,
				Message:     fmt.Sprintf("%s は %s をつかった！ %s の%sが %d 上がった！", actor.NameOrID(), it.Name, tgt.NameOrID(), stat, it.Power),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		}

	case ActionKindStatus:
		var statusTargets []Participant
		if it.TargetScope == TargetScopeAllEnemies {
			statusTargets = opponents
		} else {
			primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
			if primaryTarget != nil {
				statusTargets = []Participant{*primaryTarget}
			}
		}
		for _, tgt := range statusTargets {
			ctx.statusMap[tgt.ID] = it.Status
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  it.Name,
				TargetID:    tgt.ID,
				Message:     fmt.Sprintf("%s は %s をつかった！ %s は状態異常（%s）になった！", actor.NameOrID(), it.Name, tgt.NameOrID(), it.Status),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		}

	default: // ActionKindAttack
		var targetList []Participant
		if it.TargetScope == TargetScopeAllEnemies {
			targetList = opponents
		} else {
			primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
			if primaryTarget != nil {
				targetList = []Participant{*primaryTarget}
			}
		}
		for _, tgt := range targetList {
			effAtk := actor.Attack + ctx.attackBuff[actor.ID] + it.Power
			effDef := tgt.Defense + ctx.defenseBuff[tgt.ID]
			baseDmg := damage(effAtk, effDef)
			ctx.applyDamage(actor, tgt, baseDmg, it.Element, it.Name, false, "")
		}
	}

	return nil
}
