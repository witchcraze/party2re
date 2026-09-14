package battle

import "fmt"

func (ctx *battleContext) executeJobSkill(actor Participant, skill *ActionSkill, allyParty []Participant, opponents []Participant, fullOpponents []Participant) {
	ctx.mpMap[actor.ID] -= skill.MPCost

	switch skill.Kind {
	case ActionKindDejon:
		for _, opp := range fullOpponents {
			if ctx.hpMap[opp.ID] <= 0 && !ctx.banishedMap[opp.ID] {
				ctx.banishedMap[opp.ID] = true
				ctx.checkCrystalDrop(opp)
				ctx.logs = append(ctx.logs, TurnLog{
					Turn:        ctx.turns,
					ActorID:     actor.ID,
					ActionName:  skill.Name,
					TargetID:    opp.ID,
					Message:     fmt.Sprintf("%s が異空間へと吸い込まれた！", opp.NameOrID()),
					RemainingHP: copyHPMap(ctx.hpMap),
				})
			}
		}

	case ActionKindDefend:
		ctx.executeDefend(actor)

	case ActionKindHeal:
		var healTargets []Participant
		if skill.TargetScope == TargetScopeAllAllies {
			healTargets = allyParty
		} else if skill.TargetScope == TargetScopeSelf {
			healTargets = []Participant{actor}
		} else { // TargetScopeSingleAlly or default
			lowest := findLowestHPTarget(allyParty, ctx.hpMap)
			if lowest != nil {
				healTargets = []Participant{*lowest}
			}
		}
		for _, tgt := range healTargets {
			healAmt := skill.Power
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
				ActionName:  skill.Name,
				TargetID:    tgt.ID,
				HealingDone: healAmt,
				Message:     fmt.Sprintf("%s の %s！ %s のHPが %d 回復した！", actor.NameOrID(), skill.Name, tgt.NameOrID(), healAmt),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		}

	case ActionKindBuff:
		var buffTargets []Participant
		if skill.TargetScope == TargetScopeAllAllies {
			buffTargets = allyParty
		} else if skill.TargetScope == TargetScopeSelf || skill.TargetScope == TargetScopeSingleAlly {
			buffTargets = []Participant{actor}
		} else {
			buffTargets = []Participant{actor}
		}
		for _, tgt := range buffTargets {
			stat := skill.BuffStat
			if stat == "" {
				stat = "defense"
			}
			switch stat {
			case "attack":
				ctx.attackBuff[tgt.ID] += skill.Power
			case "agility":
				ctx.agilityBuff[tgt.ID] += skill.Power
			default:
				ctx.defenseBuff[tgt.ID] += skill.Power
			}
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  skill.Name,
				TargetID:    tgt.ID,
				Message:     fmt.Sprintf("%s の %s！ %s の%sが %d 上がった！", actor.NameOrID(), skill.Name, tgt.NameOrID(), stat, skill.Power),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		}

	case ActionKindStatus:
		var statusTargets []Participant
		if skill.TargetScope == TargetScopeAllEnemies {
			statusTargets = opponents
		} else {
			primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
			if primaryTarget != nil {
				statusTargets = []Participant{*primaryTarget}
			}
		}
		for _, tgt := range statusTargets {
			ctx.statusMap[tgt.ID] = skill.Status
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  skill.Name,
				TargetID:    tgt.ID,
				Message:     fmt.Sprintf("%s の %s！ %s は状態異常（%s）になった！", actor.NameOrID(), skill.Name, tgt.NameOrID(), skill.Status),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		}

	default: // ActionKindAttack
		var targetList []Participant
		if skill.TargetScope == TargetScopeAllEnemies {
			targetList = opponents
		} else if skill.TargetScope == TargetScopeSingleEnemy || skill.TargetScope == "" {
			primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
			if primaryTarget != nil {
				targetList = []Participant{*primaryTarget}
			}
		} else {
			primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
			if primaryTarget != nil {
				targetList = []Participant{*primaryTarget}
			}
		}
		for _, tgt := range targetList {
			effAtk := actor.Attack + ctx.attackBuff[actor.ID] + skill.Power
			effDef := tgt.Defense + ctx.defenseBuff[tgt.ID]
			baseDmg := damage(effAtk, effDef)
			ctx.applyDamage(actor, tgt, baseDmg, skill.Element, skill.Name, false, "")
		}
	}
}

func (ctx *battleContext) executeCustomSkill(actor Participant, cs *ActionCustomSkill, allyParty []Participant, opponents []Participant) {
	ctx.cmpMap[actor.ID] -= cs.CMPCost
	for _, gem := range cs.Gems {
		switch gem.Kind {
		case "field":
			ctx.field.CreateField(gem.Element, gem.Duration)
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  cs.Name,
				Message:     fmt.Sprintf("%s 「%s」 %s！ %sの属性フィールドが展開された！", actor.NameOrID(), cs.Incantation, cs.Name, gem.Element),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		case "anti_field":
			ctx.field.CreateAntiField(gem.Duration)
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  cs.Name,
				Message:     fmt.Sprintf("%s 「%s」 %s！ アンチフィールドが展開された！", actor.NameOrID(), cs.Incantation, cs.Name),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		case ActionKindHeal:
			var healTargets []Participant
			if gem.TargetScope == TargetScopeAllAllies {
				healTargets = allyParty
			} else {
				lowest := findLowestHPTarget(allyParty, ctx.hpMap)
				if lowest != nil {
					healTargets = []Participant{*lowest}
				}
			}
			for _, tgt := range healTargets {
				healAmt := gem.Power
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
					ActionName:  cs.Name,
					TargetID:    tgt.ID,
					HealingDone: healAmt,
					Message:     fmt.Sprintf("%s 「%s」 %s！ %s のHPが %d 回復した！", actor.NameOrID(), cs.Incantation, cs.Name, tgt.NameOrID(), healAmt),
					RemainingHP: copyHPMap(ctx.hpMap),
				})
			}
		case ActionKindBuff:
			var buffTargets []Participant
			if gem.TargetScope == TargetScopeAllAllies {
				buffTargets = allyParty
			} else {
				buffTargets = []Participant{actor}
			}
			for _, tgt := range buffTargets {
				stat := gem.BuffStat
				if stat == "" {
					stat = "defense"
				}
				switch stat {
				case "attack":
					ctx.attackBuff[tgt.ID] += gem.Power
				case "agility":
					ctx.agilityBuff[tgt.ID] += gem.Power
				default:
					ctx.defenseBuff[tgt.ID] += gem.Power
				}
				ctx.logs = append(ctx.logs, TurnLog{
					Turn:        ctx.turns,
					ActorID:     actor.ID,
					ActionName:  cs.Name,
					TargetID:    tgt.ID,
					Message:     fmt.Sprintf("%s 「%s」 %s！ %s の%sが %d 上がった！", actor.NameOrID(), cs.Incantation, cs.Name, tgt.NameOrID(), stat, gem.Power),
					RemainingHP: copyHPMap(ctx.hpMap),
				})
			}
		default: // ActionKindAttack
			var targetList []Participant
			if gem.TargetScope == TargetScopeAllEnemies {
				targetList = opponents
			} else {
				primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
				if primaryTarget != nil {
					targetList = []Participant{*primaryTarget}
				}
			}
			for _, tgt := range targetList {
				effAtk := actor.Attack + ctx.attackBuff[actor.ID] + gem.Power
				effDef := tgt.Defense + ctx.defenseBuff[tgt.ID]
				baseDmg := damage(effAtk, effDef)
				ctx.applyDamage(actor, tgt, baseDmg, gem.Element, cs.Name, true, cs.Incantation)
			}
		}
	}
}
