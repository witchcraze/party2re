package battle

import (
	"fmt"
	"math/rand"
)

type battleContext struct {
	turns           int
	req             PartyBattleRequest
	field           *FieldState
	hpMap           map[string]int
	mpMap           map[string]int
	cmpMap          map[string]int
	defendingMap    map[string]bool
	statusMap       map[string]string
	attackBuff      map[string]int
	defenseBuff     map[string]int
	agilityBuff     map[string]int
	abilitiesMap    map[string][]string
	itemsMap        map[string][]ActionItem
	consumedItems   map[string][]ConsumedItem
	banishedMap     map[string]bool
	logs            []TurnLog
	teamMap         map[string]string
	allParticipants []Participant
}

func (ctx *battleContext) checkStatusSkip(actor Participant) bool {
	st := ctx.statusMap[actor.ID]
	switch st {
	case StatusParalyze:
		if rand.Float64() < 0.33 {
			ctx.statusMap[actor.ID] = ""
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  "回復",
				Message:     fmt.Sprintf("%s の麻痺が治った！", actor.NameOrID()),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
			return false
		}
		ctx.logs = append(ctx.logs, TurnLog{
			Turn:        ctx.turns,
			ActorID:     actor.ID,
			ActionName:  "行動不能",
			Message:     fmt.Sprintf("%s は麻痺して動くことができない！", actor.NameOrID()),
			RemainingHP: copyHPMap(ctx.hpMap),
		})
		return true
	case StatusSleep:
		if rand.Float64() < 0.33 {
			ctx.statusMap[actor.ID] = ""
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  "起床",
				Message:     fmt.Sprintf("%s は眠りから覚めた！", actor.NameOrID()),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
			return false
		}
		ctx.logs = append(ctx.logs, TurnLog{
			Turn:        ctx.turns,
			ActorID:     actor.ID,
			ActionName:  "眠り",
			Message:     fmt.Sprintf("%s は眠っている！", actor.NameOrID()),
			RemainingHP: copyHPMap(ctx.hpMap),
		})
		return true
	}
	return false
}

func (ctx *battleContext) executeDefend(actor Participant) {
	ctx.defendingMap[actor.ID] = true
	ctx.logs = append(ctx.logs, TurnLog{
		Turn:        ctx.turns,
		ActorID:     actor.ID,
		ActionName:  "ぼうぎょ",
		Message:     fmt.Sprintf("%s は身を固めている！", actor.NameOrID()),
		RemainingHP: copyHPMap(ctx.hpMap),
	})
}

func (ctx *battleContext) applyDamage(actor Participant, target Participant, baseDamage int, element string, actionName string, isCustom bool, incantation string) {
	mult := ctx.field.DamageMultiplier(element)
	dmg := int(float64(baseDamage) * mult)
	if ctx.defendingMap[target.ID] {
		dmg = dmg / 2
	}
	if dmg < 1 {
		dmg = 1
	}
	ctx.hpMap[target.ID] -= dmg
	if ctx.hpMap[target.ID] < 0 {
		ctx.hpMap[target.ID] = 0
	}

	var msg string
	if isCustom {
		msg = fmt.Sprintf("%s 「%s」 %s！ %s に %d のダメージ！", actor.NameOrID(), incantation, actionName, target.NameOrID(), dmg)
	} else {
		msg = fmt.Sprintf("%s の %s！ %s に %d のダメージ！", actor.NameOrID(), actionName, target.NameOrID(), dmg)
	}

	if ctx.hpMap[target.ID] <= 0 {
		if !ctx.banishedMap[target.ID] {
			curMP := ctx.mpMap[target.ID]
			targetCopy := target
			targetCopy.Abilities = ctx.abilitiesMap[target.ID]
			rev := CheckRevival(&targetCopy, &curMP)
			ctx.mpMap[target.ID] = curMP
			if rev.Revived {
				ctx.hpMap[target.ID] = rev.HP
				ctx.attackBuff[target.ID] += rev.AttackBuff
				ctx.defenseBuff[target.ID] += rev.DefenseBuff
				ctx.agilityBuff[target.ID] += rev.AgilityBuff
				msg += " " + rev.Message
				if rev.Cursed {
					ctx.abilitiesMap[target.ID] = append(ctx.abilitiesMap[target.ID], "cursed")
				} else {
					ctx.abilitiesMap[target.ID] = nil
				}
			} else {
				ctx.applyMazinSynergy(target)
			}
		}
	}

	ctx.logs = append(ctx.logs, TurnLog{
		Turn:        ctx.turns,
		ActorID:     actor.ID,
		ActionName:  actionName,
		TargetID:    target.ID,
		DamageDealt: dmg,
		Message:     msg,
		RemainingHP: copyHPMap(ctx.hpMap),
	})
}

func (ctx *battleContext) applyMazinSynergy(fallen Participant) {
	fallenTeam := ctx.teamMap[fallen.ID]
	for _, ally := range ctx.allParticipants {
		if ctx.teamMap[ally.ID] != fallenTeam {
			continue
		}
		if ally.ID == fallen.ID || ctx.hpMap[ally.ID] <= 0 || !hasItem(ally.ItemDefinitionIDs, "item-037") || !hasItem(ally.ItemDefinitionIDs, "item-038") {
			continue
		}
		bonus := fallen.Attack / 2
		ctx.attackBuff[ally.ID] += bonus
		if ally.Attack+ctx.attackBuff[ally.ID] > 999 {
			ctx.attackBuff[ally.ID] = 999 - ally.Attack
		}
	}
}

func hasItem(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

func (ctx *battleContext) executeJobSkill(actor Participant, skill *ActionSkill, allyParty []Participant, opponents []Participant, fullOpponents []Participant) {
	ctx.mpMap[actor.ID] -= skill.MPCost

	switch skill.Kind {
	case ActionKindDejon:
		for _, opp := range fullOpponents {
			if ctx.hpMap[opp.ID] <= 0 && !ctx.banishedMap[opp.ID] {
				ctx.banishedMap[opp.ID] = true
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

func (ctx *battleContext) executeNormalAttack(actor Participant, opponents []Participant) {
	primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
	if primaryTarget != nil {
		effAtk := actor.Attack + ctx.attackBuff[actor.ID]
		effDef := primaryTarget.Defense + ctx.defenseBuff[primaryTarget.ID]
		baseDmg := damage(effAtk, effDef)
		ctx.applyDamage(actor, *primaryTarget, baseDmg, "", "攻撃", false, "")
	}
}

func (ctx *battleContext) applyPoisonDOT(combatants []Participant) {
	for _, p := range combatants {
		if ctx.statusMap[p.ID] == StatusPoison && ctx.hpMap[p.ID] > 0 {
			maxHP := p.MaxHP
			if maxHP <= 0 {
				maxHP = p.HP
			}
			dotDmg := maxHP / 10
			if dotDmg < 1 {
				dotDmg = 1
			}
			ctx.hpMap[p.ID] -= dotDmg
			if ctx.hpMap[p.ID] < 0 {
				ctx.hpMap[p.ID] = 0
			}
			msg := fmt.Sprintf("%s は毒により %d のダメージをうけた！", p.NameOrID(), dotDmg)
			if ctx.hpMap[p.ID] <= 0 {
				curMP := ctx.mpMap[p.ID]
				targetCopy := p
				targetCopy.Abilities = ctx.abilitiesMap[p.ID]
				rev := CheckRevival(&targetCopy, &curMP)
				ctx.mpMap[p.ID] = curMP
				if rev.Revived {
					ctx.hpMap[p.ID] = rev.HP
					ctx.attackBuff[p.ID] += rev.AttackBuff
					ctx.defenseBuff[p.ID] += rev.DefenseBuff
					ctx.agilityBuff[p.ID] += rev.AgilityBuff
					msg += " " + rev.Message
					if rev.Cursed {
						ctx.abilitiesMap[p.ID] = append(ctx.abilitiesMap[p.ID], "cursed")
					} else {
						ctx.abilitiesMap[p.ID] = nil
					}
				} else {
					ctx.applyMazinSynergy(p)
				}
			}
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     p.ID,
				ActionName:  "毒ダメージ",
				TargetID:    p.ID,
				DamageDealt: dotDmg,
				Message:     msg,
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		}
	}
}
