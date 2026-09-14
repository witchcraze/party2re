package battle

import (
	"fmt"

	"github.com/witchcraze/party2re/internal/core/random"
)

type battleContext struct {
	turns             int
	req               PartyBattleRequest
	rng               random.Generator
	field             *FieldState
	hpMap             map[string]int
	mpMap             map[string]int
	cmpMap            map[string]int
	defendingMap      map[string]bool
	statusMap         map[string]string
	attackBuff        map[string]int
	defenseBuff       map[string]int
	agilityBuff       map[string]int
	abilitiesMap      map[string][]string
	itemsMap          map[string][]ActionItem
	consumedItems     map[string][]ConsumedItem
	banishedMap       map[string]bool
	logs              []TurnLog
	teamMap           map[string]string
	allParticipants   []Participant
	allyTeamID        string
	crystalDroppedMap map[string]bool
	droppedCrystals   int
}

func (ctx *battleContext) checkStatusSkip(actor Participant) bool {
	st := ctx.statusMap[actor.ID]
	switch st {
	case StatusParalyze:
		if ctx.rng.Float64() < 0.33 {
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
		if ctx.rng.Float64() < 0.33 {
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
				ctx.checkCrystalDrop(target)
			}
		}
	} else if hasAbility(ctx.abilitiesMap[actor.ID], "seal_kuu") {
		// Seal 11 (空): 12.5% chance (rand(8) < 1) to dispel target's stat buffs and defending state (_skill.cgi:8997-9005)
		if ctx.rng != nil && ctx.rng.Intn(8) < 1 {
			ctx.attackBuff[target.ID] = 0
			ctx.defenseBuff[target.ID] = 0
			ctx.agilityBuff[target.ID] = 0
			ctx.defendingMap[target.ID] = false
			ctx.statusMap[target.ID] = ""
			msg += fmt.Sprintf(" %s のステータスが元に戻った！", target.NameOrID())
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

func (ctx *battleContext) executeNormalAttack(actor Participant, opponents []Participant) {
	attackOnce := func() bool {
		primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
		if primaryTarget == nil {
			return false
		}
		effAtk := actor.Attack + ctx.attackBuff[actor.ID]
		effDef := primaryTarget.Defense + ctx.defenseBuff[primaryTarget.ID]
		baseDmg := damage(effAtk, effDef)

		actionName := "攻撃"
		element := ""
		// Seal 12 (理): Consumes 3 MP, physical attack becomes magic, 0.8x damage (_skill.cgi:10186-10191)
		if hasAbility(ctx.abilitiesMap[actor.ID], "seal_kotowari") {
			ctx.mpMap[actor.ID] -= 3
			if ctx.mpMap[actor.ID] < 0 {
				ctx.mpMap[actor.ID] = 0
			}
			actionName = "理力攻撃"
			element = "magic"
			baseDmg = int(float64(baseDmg) * 0.8)
			if baseDmg < 1 {
				baseDmg = 1
			}
		}

		ctx.applyDamage(actor, *primaryTarget, baseDmg, element, actionName, false, "")
		return true
	}

	if attackOnce() {
		// Seal 10 (神速): Normal attack strikes twice (_skill.cgi:111, _data.cgi:2220)
		if hasAbility(ctx.abilitiesMap[actor.ID], "seal_shinsoku") {
			attackOnce()
		}
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
					ctx.checkCrystalDrop(p)
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

func (ctx *battleContext) checkCrystalDrop(target Participant) {
	if ctx.teamMap[target.ID] == ctx.allyTeamID {
		return
	}
	if ctx.crystalDroppedMap[target.ID] {
		return
	}
	ctx.crystalDroppedMap[target.ID] = true

	targetMaxHP := target.MaxHP
	if targetMaxHP <= 0 {
		targetMaxHP = target.HP
	}
	targetMaxMP := target.MaxMP
	if targetMaxMP <= 0 {
		targetMaxMP = target.MP
	}
	enemyStrong := targetMaxHP + targetMaxMP + target.Attack + int(float64(target.Defense)*0.5) + target.Agility

	allyStrongSum := 0
	allyCount := 0
	for _, p := range ctx.allParticipants {
		if ctx.teamMap[p.ID] == ctx.allyTeamID {
			mhp := p.MaxHP
			if mhp <= 0 {
				mhp = p.HP
			}
			mmp := p.MaxMP
			if mmp <= 0 {
				mmp = p.MP
			}
			allyStrongSum += mhp + mmp + p.Attack + int(float64(p.Defense)*0.5) + p.Agility
			allyCount++
		}
	}
	allyStrong := 0
	if allyCount > 0 {
		allyStrong = allyStrongSum / allyCount
	}

	chance := 1
	if enemyStrong > int(float64(allyStrong)*0.5) {
		chance = 2
	}

	if ctx.rng != nil && ctx.rng.Intn(50) < chance {
		ctx.droppedCrystals++
	}
}
