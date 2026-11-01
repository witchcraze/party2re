package battle

import "fmt"

func (ctx *battleContext) executeJobSkill(actor Participant, skill *ActionSkill, allyParty []Participant, opponents []Participant, fullOpponents []Participant) error {
	if err := ValidateSkillAction(*skill); err != nil {
		return err
	}
	ctx.mpMap[actor.ID] -= skill.MPCost

	isMagic := skill.Category == string(CategoryMagic) || skill.Category == "magic"
	if isMagic {
		status := ctx.statusMap[actor.ID]
		if status == StatusMagicSeal || status == "魔封" {
			if ctx.rng != nil && ctx.rng.Float64() < 0.25 {
				ctx.statusMap[actor.ID] = ""
				ctx.logs = append(ctx.logs, TurnLog{
					Turn:        ctx.turns,
					ActorID:     actor.ID,
					ActionName:  skill.Name,
					Message:     fmt.Sprintf("%s は魔法が使えるようになりました！", actor.NameOrID()),
					RemainingHP: copyHPMap(ctx.hpMap),
				})
			} else {
				ctx.logs = append(ctx.logs, TurnLog{
					Turn:        ctx.turns,
					ActorID:     actor.ID,
					ActionName:  skill.Name,
					Message:     fmt.Sprintf("しかし、%s は魔法が封じられていた", actor.NameOrID()),
					RemainingHP: copyHPMap(ctx.hpMap),
				})
				return nil
			}
		}
	}

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
			if ctx.isConfused(actor) {
				if tgt := ctx.resolveConfusedSingleTarget(actor); tgt != nil {
					healTargets = []Participant{*tgt}
				}
			} else {
				lowest := findLowestHPTarget(allyParty, ctx.hpMap)
				if lowest != nil {
					healTargets = []Participant{*lowest}
				}
			}
		}
		isMagic := skill.Category == string(CategoryMagic) || skill.Category == "magic"
		ten := ctx.getTension(actor.ID)
		for _, tgt := range healTargets {
			var healAmt int
			if isMagic {
				v := float64(skill.Power)
				if hasItem(actor.ItemDefinitionIDs, "item-146") { // ヒーリングストーン (_skill.cgi:9081)
					v *= 1.8
				}
				if hasItem(actor.ItemDefinitionIDs, "item-256") { // item-256 (_skill.cgi:9082)
					v *= 1.2
				}
				if st := ctx.statusMap[actor.ID]; st == StatusSabaku || st == "鎖縛" {
					sabakuMult := 0.7
					if ctx.rng != nil {
						sabakuMult += ctx.rng.Float64() * 0.2
					}
					v *= sabakuMult
				}
				if st := ctx.statusMap[tgt.ID]; st == StatusVirulentPoison || st == "劇毒" {
					v = float64(int(v * 0.1))
				}
				variance := 1.0
				if ctx.rng != nil {
					variance = 0.9 + ctx.rng.Float64()*0.3
				}
				healAmt = int(v * variance * ten)
			} else {
				healAmt = skill.Power
			}
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

			// item-256 splash heal to other living party members (_skill.cgi:9118-9130)
			if isMagic && hasItem(actor.ItemDefinitionIDs, "item-256") {
				splashAmt := healAmt / 2
				primaryMax := tgt.MaxHP
				if primaryMax <= 0 {
					primaryMax = tgt.HP
				}
				capAmt := int(float64(primaryMax) * 0.3)
				if splashAmt > capAmt {
					splashAmt = capAmt
				}
				for _, ally := range allyParty {
					if ally.ID == tgt.ID || ctx.hpMap[ally.ID] <= 0 {
						continue
					}
					allyMax := ally.MaxHP
					if allyMax <= 0 {
						allyMax = ally.HP
					}
					actualSplash := splashAmt
					ctx.hpMap[ally.ID] += actualSplash
					if ctx.hpMap[ally.ID] > allyMax {
						ctx.hpMap[ally.ID] = allyMax
					}
					ctx.logs = append(ctx.logs, TurnLog{
						Turn:        ctx.turns,
						ActorID:     actor.ID,
						ActionName:  skill.Name,
						TargetID:    ally.ID,
						HealingDone: actualSplash,
						Message:     fmt.Sprintf("%s の %s！ %s のHPが %d 回復した！", actor.NameOrID(), skill.Name, ally.NameOrID(), actualSplash),
						RemainingHP: copyHPMap(ctx.hpMap),
					})
				}
			}
		}
		if isMagic {
			if ctx.tensionMap != nil {
				ctx.tensionMap[actor.ID] = 1.0
			}
		}

	case ActionKindBuff:
		var buffTargets []Participant
		if skill.TargetScope == TargetScopeAllAllies {
			buffTargets = allyParty
		} else if skill.TargetScope == TargetScopeSelf {
			buffTargets = []Participant{actor}
		} else {
			if ctx.isConfused(actor) {
				if tgt := ctx.resolveConfusedSingleTarget(actor); tgt != nil {
					buffTargets = []Participant{*tgt}
				}
			} else {
				buffTargets = []Participant{actor}
			}
		}
		for _, tgt := range buffTargets {
			if ctx.statusMap[tgt.ID] == StatusSabaku || ctx.statusMap[tgt.ID] == "鎖縛" {
				ctx.logs = append(ctx.logs, TurnLog{
					Turn:        ctx.turns,
					ActorID:     actor.ID,
					ActionName:  skill.Name,
					TargetID:    tgt.ID,
					Message:     fmt.Sprintf("%s は鎖縛により能力があがらない！", tgt.NameOrID()),
					RemainingHP: copyHPMap(ctx.hpMap),
				})
				continue
			}
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
			if ctx.isConfused(actor) {
				if tgt := ctx.resolveConfusedSingleTarget(actor); tgt != nil {
					statusTargets = []Participant{*tgt}
				}
			} else {
				primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
				if primaryTarget != nil {
					statusTargets = []Participant{*primaryTarget}
				}
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

	case ActionKindAttack:
		var targetList []Participant
		if skill.TargetScope == TargetScopeAllEnemies {
			targetList = opponents
		} else {
			if ctx.isConfused(actor) {
				if tgt := ctx.resolveConfusedSingleTarget(actor); tgt != nil {
					targetList = []Participant{*tgt}
				}
			} else {
				primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
				if primaryTarget != nil {
					targetList = []Participant{*primaryTarget}
				}
			}
		}
		decayMult := 1.0
		isArea := skill.TargetScope == TargetScopeAllEnemies
		hasDiamondRing := hasItem(actor.ItemDefinitionIDs, "item-144")
		isMagic := skill.Category == string(CategoryMagic) || skill.Category == "magic"
		has143 := hasItem(actor.ItemDefinitionIDs, "item-143")
		has149 := hasItem(actor.ItemDefinitionIDs, "item-149")

		evalInitialPower := func(v float64) float64 {
			if has149 {
				if skill.Direct {
					v *= 1.1
				} else {
					v *= 1.5
				}
			}
			if has143 && isMagic && v < 300 {
				v = float64(int(v * 2))
			}
			return v
		}
		initialPower := evalInitialPower(float64(skill.Power))

		for _, tgt := range targetList {
			var baseDmg int
			if isMagic || skill.Direct {
				ten := ctx.getTension(actor.ID)
				basePower := evalInitialPower(float64(skill.Power) * decayMult)
				if st := ctx.statusMap[tgt.ID]; st == StatusKinju || st == "禁呪" {
					basePower *= 1.25
				}
				if hasItem(actor.ItemDefinitionIDs, "weapon-34") && isMagic && ctx.rng != nil && ctx.rng.Intn(4) < 1 {
					basePower *= 1.2
				}
				if hasDiamondRing && isArea && basePower < 300 {
					basePower *= 1.3
				}
				if !has149 {
					basePower *= ten
				}
				effDef := tgt.Defense + ctx.defenseBuff[tgt.ID]
				baseDmg = rollDamage(basePower, float64(effDef)*0.3, true, tgt.Defense, ctx.rng)
			} else {
				effAtk := float64(actor.Attack+ctx.attackBuff[actor.ID]+skill.Power) * decayMult
				effDef := tgt.Defense + ctx.defenseBuff[tgt.ID]
				baseDmg = CalculateDamage(int(effAtk), effDef, ctx.rng, false)
			}
			effective := ctx.applyDamageWithCategory(actor, tgt, baseDmg, skill.Element, skill.Category, skill.Name, false, "", false)
			if (isMagic || skill.Direct) && effective {
				ctx.consumeTension(actor)
			}
			if isArea && (!hasDiamondRing || initialPower > 300) {
				decayMult *= 0.85
			}
		}
	}
	return nil
}

func (ctx *battleContext) executeCustomSkill(actor Participant, cs *ActionCustomSkill, allyParty []Participant, opponents []Participant) error {
	if err := ValidateCustomSkillAction(*cs); err != nil {
		return err
	}
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
				if ctx.isConfused(actor) {
					if tgt := ctx.resolveConfusedSingleTarget(actor); tgt != nil {
						healTargets = []Participant{*tgt}
					}
				} else {
					lowest := findLowestHPTarget(allyParty, ctx.hpMap)
					if lowest != nil {
						healTargets = []Participant{*lowest}
					}
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
				if ctx.isConfused(actor) {
					if tgt := ctx.resolveConfusedSingleTarget(actor); tgt != nil {
						buffTargets = []Participant{*tgt}
					}
				} else {
					buffTargets = []Participant{actor}
				}
			}
			for _, tgt := range buffTargets {
				if ctx.statusMap[tgt.ID] == StatusSabaku || ctx.statusMap[tgt.ID] == "鎖縛" {
					ctx.logs = append(ctx.logs, TurnLog{
						Turn:        ctx.turns,
						ActorID:     actor.ID,
						ActionName:  cs.Name,
						TargetID:    tgt.ID,
						Message:     fmt.Sprintf("%s は鎖縛により能力があがらない！", tgt.NameOrID()),
						RemainingHP: copyHPMap(ctx.hpMap),
					})
					continue
				}
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
		case ActionKindAttack:
			var targetList []Participant
			if gem.TargetScope == TargetScopeAllEnemies {
				targetList = opponents
			} else {
				if ctx.isConfused(actor) {
					if tgt := ctx.resolveConfusedSingleTarget(actor); tgt != nil {
						targetList = []Participant{*tgt}
					}
				} else {
					primaryTarget := findLowestHPTarget(opponents, ctx.hpMap)
					if primaryTarget != nil {
						targetList = []Participant{*primaryTarget}
					}
				}
			}
			decayMult := 1.0
			hasDiamondRing := hasItem(actor.ItemDefinitionIDs, "item-144")
			for _, tgt := range targetList {
				effAtk := float64(actor.Attack+ctx.attackBuff[actor.ID]+gem.Power) * decayMult
				effDef := tgt.Defense + ctx.defenseBuff[tgt.ID]
				baseDmg := CalculateDamage(int(effAtk), effDef, ctx.rng, false)
				ctx.applyDamage(actor, tgt, baseDmg, gem.Element, cs.Name, true, cs.Incantation, false)
				if gem.TargetScope == TargetScopeAllEnemies && !hasDiamondRing {
					decayMult *= 0.85
				}
			}
		}
		if ctx.hpMap[actor.ID] <= 0 {
			break
		}
	}
	return nil
}
