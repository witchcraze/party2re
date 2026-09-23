package battle

import (
	"fmt"
)

func isImmobilized(status string) bool {
	return status == StatusParalyze || status == StatusSleep ||
		status == StatusKinju || status == StatusSabaku || status == StatusDofuu ||
		status == "麻痺" || status == "眠り" || status == "禁呪" || status == "鎖縛" || status == "動封"
}

func (ctx *battleContext) isConfused(actor Participant) bool {
	st := ctx.statusMap[actor.ID]
	return st == StatusConfusion || st == "混乱"
}

func (ctx *battleContext) checkConfusion(actor Participant) {
	if !ctx.isConfused(actor) {
		return
	}
	// 20% chance to naturally cure (legacy rand(5) < 1)
	if ctx.rng != nil && ctx.rng.Float64() < 0.20 {
		ctx.statusMap[actor.ID] = ""
		ctx.logs = append(ctx.logs, TurnLog{
			Turn:        ctx.turns,
			ActorID:     actor.ID,
			ActionName:  "回復",
			Message:     fmt.Sprintf("%s の混乱が治った！", actor.NameOrID()),
			RemainingHP: copyHPMap(ctx.hpMap),
		})
		return
	}
	ctx.logs = append(ctx.logs, TurnLog{
		Turn:        ctx.turns,
		ActorID:     actor.ID,
		ActionName:  "混乱",
		Message:     fmt.Sprintf("%s は混乱している！", actor.NameOrID()),
		RemainingHP: copyHPMap(ctx.hpMap),
	})
}

func (ctx *battleContext) getConfusedTargets(actor Participant) []Participant {
	var candidates []Participant
	for _, p := range ctx.allParticipants {
		if ctx.hpMap[p.ID] > 0 && !ctx.banishedMap[p.ID] && p.ID != actor.ID {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		candidates = append(candidates, actor)
	}
	return candidates
}

func (ctx *battleContext) resolveConfusedSingleTarget(actor Participant) *Participant {
	candidates := ctx.getConfusedTargets(actor)
	if len(candidates) == 0 {
		return nil
	}
	idx := 0
	if ctx.rng != nil && len(candidates) > 1 {
		idx = ctx.rng.Intn(len(candidates))
	}
	tgt := candidates[idx]
	return &tgt
}

func (ctx *battleContext) resolveAttackTarget(actor Participant, opponents []Participant) *Participant {
	if ctx.isConfused(actor) {
		return ctx.resolveConfusedSingleTarget(actor)
	}
	return findLowestHPTarget(opponents, ctx.hpMap)
}

func (ctx *battleContext) checkStatusSkip(actor Participant) bool {
	st := ctx.statusMap[actor.ID]
	switch st {
	case StatusDofuu, "動封":
		// 100% action skip, clears immediately (legacy _battle.cgi:686-689)
		ctx.statusMap[actor.ID] = ""
		ctx.logs = append(ctx.logs, TurnLog{
			Turn:        ctx.turns,
			ActorID:     actor.ID,
			ActionName:  "行動不能",
			Message:     fmt.Sprintf("しかし、%s は動くことができない！", actor.NameOrID()),
			RemainingHP: copyHPMap(ctx.hpMap),
		})
		return true

	case StatusParalyze, "麻痺":
		if ctx.rng != nil && ctx.rng.Float64() < 0.33 { // legacy: rand(3) < 1
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

	case StatusSleep, "眠り":
		if ctx.rng != nil && ctx.rng.Float64() < 0.33 { // legacy: rand(3) < 1
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

	case StatusKinju, "禁呪":
		// 25% chance to trigger self-damage and skip turn (legacy _battle.cgi:709-721)
		if ctx.rng != nil && ctx.rng.Float64() < 0.25 {
			maxHP := actor.MaxHP
			if maxHP <= 0 {
				maxHP = actor.HP
			}
			dmg := maxHP / 10
			if dmg < 1 {
				dmg = 1
			}
			if dmg > 999 && ctx.rng != nil {
				dmg = 950 + ctx.rng.Intn(100)
			}
			ctx.hpMap[actor.ID] -= dmg
			if ctx.hpMap[actor.ID] < 0 {
				ctx.hpMap[actor.ID] = 0
			}
			ctx.statusMap[actor.ID] = ""

			msg := fmt.Sprintf("%s は禁呪により %d のダメージをうけた！", actor.NameOrID(), dmg)
			if ctx.hpMap[actor.ID] <= 0 {
				curMP := ctx.mpMap[actor.ID]
				targetCopy := actor
				targetCopy.Abilities = ctx.abilitiesMap[actor.ID]
				rev := CheckRevival(&targetCopy, &curMP)
				ctx.mpMap[actor.ID] = curMP
				if rev.Revived {
					ctx.hpMap[actor.ID] = rev.HP
					ctx.attackBuff[actor.ID] += rev.AttackBuff
					ctx.defenseBuff[actor.ID] += rev.DefenseBuff
					ctx.agilityBuff[actor.ID] += rev.AgilityBuff
					if rev.Status != "" {
						ctx.statusMap[actor.ID] = rev.Status
					}
					msg += " " + rev.Message
					if rev.Cursed {
						ctx.abilitiesMap[actor.ID] = append(ctx.abilitiesMap[actor.ID], "cursed")
					} else {
						ctx.abilitiesMap[actor.ID] = nil
					}
				} else {
					ctx.applyMazinSynergy(actor)
					ctx.checkCrystalDrop(actor)
				}
			}

			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  "禁呪",
				DamageDealt: dmg,
				Message:     msg,
				RemainingHP: copyHPMap(ctx.hpMap),
			})
			return true
		}
		// 75% chance: acts normally, status remains
		return false

	case StatusSabaku, "鎖縛":
		// 25% chance to skip turn (legacy _battle.cgi:722-728)
		if ctx.rng != nil && ctx.rng.Float64() < 0.25 {
			msg := fmt.Sprintf("しかし、%s は動くことができない！", actor.NameOrID())
			// 50% chance to cure when skipped
			if ctx.rng.Float64() < 0.50 {
				ctx.statusMap[actor.ID] = ""
				msg += fmt.Sprintf(" %s は鎖から解放された！", actor.NameOrID())
			}
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  "行動不能",
				Message:     msg,
				RemainingHP: copyHPMap(ctx.hpMap),
			})
			return true
		}
		// 75% chance: acts normally, status remains
		return false

	default:
		return false
	}
}

func (ctx *battleContext) applyPostActionPoison(actor Participant) {
	if ctx.hpMap[actor.ID] <= 0 || ctx.banishedMap[actor.ID] {
		return
	}
	st := ctx.statusMap[actor.ID]
	isDeadly := st == StatusDeadlyPoison || st == "猛毒" || st == "劇毒"
	isStandard := st == StatusPoison || st == "poison" || st == "毒"

	if !isDeadly && !isStandard {
		return
	}

	maxHP := actor.MaxHP
	if maxHP <= 0 {
		maxHP = actor.HP
	}
	dotDmg := maxHP / 10
	if dotDmg < 1 {
		dotDmg = 1
	}
	if dotDmg > 999 && ctx.rng != nil {
		dotDmg = 950 + ctx.rng.Intn(100)
	}

	ctx.hpMap[actor.ID] -= dotDmg
	if ctx.hpMap[actor.ID] < 0 {
		ctx.hpMap[actor.ID] = 0
	}

	statusName := "毒"
	if isDeadly {
		statusName = "猛毒"
	}
	msg := fmt.Sprintf("%s は%sにより %d のダメージをうけた！", actor.NameOrID(), statusName, dotDmg)

	if ctx.hpMap[actor.ID] <= 0 {
		curMP := ctx.mpMap[actor.ID]
		targetCopy := actor
		targetCopy.Abilities = ctx.abilitiesMap[actor.ID]
		rev := CheckRevival(&targetCopy, &curMP)
		ctx.mpMap[actor.ID] = curMP
		if rev.Revived {
			ctx.hpMap[actor.ID] = rev.HP
			ctx.attackBuff[actor.ID] += rev.AttackBuff
			ctx.defenseBuff[actor.ID] += rev.DefenseBuff
			ctx.agilityBuff[actor.ID] += rev.AgilityBuff
			if rev.Status != "" {
				ctx.statusMap[actor.ID] = rev.Status
			}
			msg += " " + rev.Message
			if rev.Cursed {
				ctx.abilitiesMap[actor.ID] = append(ctx.abilitiesMap[actor.ID], "cursed")
			} else {
				ctx.abilitiesMap[actor.ID] = nil
			}
		} else {
			ctx.applyMazinSynergy(actor)
			ctx.checkCrystalDrop(actor)
		}
	}

	ctx.logs = append(ctx.logs, TurnLog{
		Turn:        ctx.turns,
		ActorID:     actor.ID,
		ActionName:  "毒ダメージ",
		TargetID:    actor.ID,
		DamageDealt: dotDmg,
		Message:     msg,
		RemainingHP: copyHPMap(ctx.hpMap),
	})

	// Natural cure roll: only standard poison has 20% natural cure chance (legacy _battle.cgi:851)
	if isStandard && ctx.hpMap[actor.ID] > 0 {
		if ctx.rng != nil && ctx.rng.Float64() < 0.20 {
			ctx.statusMap[actor.ID] = ""
			ctx.logs = append(ctx.logs, TurnLog{
				Turn:        ctx.turns,
				ActorID:     actor.ID,
				ActionName:  "回復",
				TargetID:    actor.ID,
				Message:     fmt.Sprintf("%s の毒が治った！", actor.NameOrID()),
				RemainingHP: copyHPMap(ctx.hpMap),
			})
		}
	}
}
