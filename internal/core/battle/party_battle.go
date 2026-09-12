package battle

import (
	"sort"
)

type combatantWrapper struct {
	participant Participant
	isAlly      bool
	index       int
}

func (p *Participant) NameOrID() string {
	if p.Name != "" {
		return p.Name
	}
	return p.ID
}

// ResolvePartyBattle resolves a multi-participant, multi-turn battle between allies and enemies.
// Evaluates agility turn order, skills (MP), custom skills (CMP), elemental fields, and defeat revival.
func (Engine) ResolvePartyBattle(req PartyBattleRequest) (PartyBattleResult, error) {
	if len(req.Allies) == 0 || len(req.Enemies) == 0 {
		return PartyBattleResult{}, ErrInvalidRequest
	}
	for _, a := range req.Allies {
		if err := validateParticipant(a); err != nil {
			return PartyBattleResult{}, err
		}
	}
	for _, e := range req.Enemies {
		if err := validateParticipant(e); err != nil {
			return PartyBattleResult{}, err
		}
	}
	for _, reward := range []Reward{req.VictoryReward, req.DefeatReward, req.DrawReward} {
		if err := validateReward(reward); err != nil {
			return PartyBattleResult{}, err
		}
	}

	bonusPercent := (len(req.Allies) - 1) * 10
	if bonusPercent < 0 {
		bonusPercent = 0
	} else if bonusPercent > 30 {
		bonusPercent = 30
	}

	ctx := &battleContext{
		req:          req,
		hpMap:        make(map[string]int),
		mpMap:        make(map[string]int),
		cmpMap:       make(map[string]int),
		defendingMap: make(map[string]bool),
		statusMap:    make(map[string]string),
		attackBuff:   make(map[string]int),
		defenseBuff:  make(map[string]int),
		agilityBuff:  make(map[string]int),
		abilitiesMap: make(map[string][]string),
		itemsMap:     make(map[string][]ActionItem),
	}

	for _, a := range req.Allies {
		ctx.hpMap[a.ID] = a.HP
		ctx.mpMap[a.ID] = a.MP
		ctx.cmpMap[a.ID] = a.CMP
		ctx.abilitiesMap[a.ID] = append([]string(nil), a.Abilities...)
		ctx.defendingMap[a.ID] = a.Defending
		ctx.statusMap[a.ID] = a.Status
		ctx.itemsMap[a.ID] = append([]ActionItem(nil), a.ActionItems...)
	}
	for _, e := range req.Enemies {
		ctx.hpMap[e.ID] = e.HP
		ctx.mpMap[e.ID] = e.MP
		ctx.cmpMap[e.ID] = e.CMP
		ctx.abilitiesMap[e.ID] = append([]string(nil), e.Abilities...)
		ctx.defendingMap[e.ID] = e.Defending
		ctx.statusMap[e.ID] = e.Status
		ctx.itemsMap[e.ID] = append([]ActionItem(nil), e.ActionItems...)
	}

	if req.InitialField != nil {
		ctx.field = req.InitialField.Clone()
	} else {
		ctx.field = &FieldState{}
	}

	maxTurns := 100

	for ctx.turns < maxTurns {
		ctx.turns++

		// 1. Gather all living combatants
		var combatants []combatantWrapper
		for i, a := range req.Allies {
			if ctx.hpMap[a.ID] > 0 {
				combatants = append(combatants, combatantWrapper{participant: a, isAlly: true, index: i})
			}
		}
		for i, e := range req.Enemies {
			if ctx.hpMap[e.ID] > 0 {
				combatants = append(combatants, combatantWrapper{participant: e, isAlly: false, index: i})
			}
		}

		if len(combatants) == 0 {
			break
		}

		// 2. Sort combatants by Agility descending (including agility buffs)
		sort.SliceStable(combatants, func(i, j int) bool {
			agI := combatants[i].participant.Agility + ctx.agilityBuff[combatants[i].participant.ID]
			agJ := combatants[j].participant.Agility + ctx.agilityBuff[combatants[j].participant.ID]
			if agI != agJ {
				return agI > agJ
			}
			if combatants[i].isAlly != combatants[j].isAlly {
				return combatants[i].isAlly
			}
			return combatants[i].index < combatants[j].index
		})

		// 3. Process actions in turn order
		for _, cw := range combatants {
			actor := cw.participant
			actorID := actor.ID
			if ctx.hpMap[actorID] <= 0 {
				continue
			}

			// Clear previous turn's defense stance when actor acts
			ctx.defendingMap[actorID] = false

			// Check status ailment incapacitation (paralysis, sleep)
			if ctx.checkStatusSkip(actor) {
				continue
			}

			var opponents []Participant
			var allyParty []Participant
			if cw.isAlly {
				for _, e := range req.Enemies {
					if ctx.hpMap[e.ID] > 0 {
						opponents = append(opponents, e)
					}
				}
				for _, a := range req.Allies {
					if ctx.hpMap[a.ID] > 0 {
						allyParty = append(allyParty, a)
					}
				}
			} else {
				for _, a := range req.Allies {
					if ctx.hpMap[a.ID] > 0 {
						opponents = append(opponents, a)
					}
				}
				for _, e := range req.Enemies {
					if ctx.hpMap[e.ID] > 0 {
						allyParty = append(allyParty, e)
					}
				}
			}

			if len(opponents) == 0 {
				break
			}

			// Decide Action: Custom Skill -> Job Skill -> Item -> Defend -> Normal Attack
			var usedCustom *ActionCustomSkill
			for i := range actor.CustomSkills {
				cs := &actor.CustomSkills[i]
				if ctx.cmpMap[actorID] >= cs.CMPCost {
					usedCustom = cs
					break
				}
			}

			var usedSkill *ActionSkill
			if usedCustom == nil {
				for i := range actor.Skills {
					sk := &actor.Skills[i]
					if ctx.mpMap[actorID] >= sk.MPCost {
						usedSkill = sk
						break
					}
				}
			}

			var usedItem *ActionItem
			if usedCustom == nil && usedSkill == nil && len(ctx.itemsMap[actorID]) > 0 {
				usedItem = &ctx.itemsMap[actorID][0]
			}

			if usedCustom != nil {
				ctx.executeCustomSkill(actor, usedCustom, allyParty, opponents)
			} else if usedSkill != nil {
				ctx.executeJobSkill(actor, usedSkill, allyParty, opponents)
			} else if usedItem != nil {
				_ = ctx.executeItem(actor, usedItem, allyParty, opponents)
			} else if actor.Defending {
				ctx.executeDefend(actor)
			} else {
				ctx.executeNormalAttack(actor, opponents)
			}

			// Check battle termination early
			alliesDead := true
			for _, a := range req.Allies {
				if ctx.hpMap[a.ID] > 0 {
					alliesDead = false
					break
				}
			}
			enemiesDead := true
			for _, e := range req.Enemies {
				if ctx.hpMap[e.ID] > 0 {
					enemiesDead = false
					break
				}
			}
			if alliesDead || enemiesDead {
				break
			}
		}

		// Apply poison DOT at the end of each round
		var allParticipants []Participant
		allParticipants = append(allParticipants, req.Allies...)
		allParticipants = append(allParticipants, req.Enemies...)
		ctx.applyPoisonDOT(allParticipants)

		// End of round field turn countdown
		ctx.field.EndTurn()

		alliesDead := true
		for _, a := range req.Allies {
			if ctx.hpMap[a.ID] > 0 {
				alliesDead = false
				break
			}
		}
		enemiesDead := true
		for _, e := range req.Enemies {
			if ctx.hpMap[e.ID] > 0 {
				enemiesDead = false
				break
			}
		}
		if alliesDead || enemiesDead {
			break
		}
	}

	alliesDead := true
	var alliesSurvived []string
	var alliesFallen []string
	for _, a := range req.Allies {
		if ctx.hpMap[a.ID] > 0 {
			alliesDead = false
			alliesSurvived = append(alliesSurvived, a.ID)
		} else {
			alliesFallen = append(alliesFallen, a.ID)
		}
	}

	enemiesDead := true
	for _, e := range req.Enemies {
		if ctx.hpMap[e.ID] > 0 {
			enemiesDead = false
			break
		}
	}

	var outcome Outcome
	var winnerSide string
	var selectedReward Reward
	if alliesDead && enemiesDead {
		outcome = OutcomeDraw
		winnerSide = "none"
		selectedReward = req.DrawReward
	} else if enemiesDead {
		outcome = OutcomeWin
		winnerSide = "allies"
		selectedReward = req.VictoryReward
	} else if alliesDead {
		outcome = OutcomeDefeat
		winnerSide = "enemies"
		selectedReward = req.DefeatReward
	} else {
		outcome = OutcomeDraw
		winnerSide = "none"
		selectedReward = req.DrawReward
	}

	totalReward := applyBonus(selectedReward, bonusPercent)

	return PartyBattleResult{
		Outcome:         outcome,
		WinnerSide:      winnerSide,
		Turns:           ctx.turns,
		BaseReward:      selectedReward,
		BonusPercent:    bonusPercent,
		TotalReward:     totalReward,
		AlliesSurvived:  alliesSurvived,
		AlliesFallen:    alliesFallen,
		RemainingHP:     copyHPMap(ctx.hpMap),
		RemainingMP:     copyHPMap(ctx.mpMap),
		RemainingCMP:    copyHPMap(ctx.cmpMap),
		RemainingStatus: copyStatusMap(ctx.statusMap),
		ConsumedItems:   copyConsumedItemsMap(ctx.consumedItems),
		Logs:            ctx.logs,
		FinalField:      ctx.field,
	}, nil
}

func findLowestHPTarget(targets []Participant, hpMap map[string]int) *Participant {
	var lowest *Participant
	minHP := int(^uint(0) >> 1)
	for i := range targets {
		tgt := &targets[i]
		if hp, exists := hpMap[tgt.ID]; exists && hp > 0 {
			if hp < minHP {
				minHP = hp
				lowest = tgt
			}
		}
	}
	return lowest
}

func copyHPMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyStatusMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyConsumedItemsMap(m map[string][]ConsumedItem) map[string][]ConsumedItem {
	if m == nil {
		return nil
	}
	out := make(map[string][]ConsumedItem, len(m))
	for k, v := range m {
		out[k] = append([]ConsumedItem(nil), v...)
	}
	return out
}

func applyBonus(r Reward, bonusPercent int) Reward {
	if bonusPercent <= 0 {
		return r
	}
	mult := float64(100+bonusPercent) / 100.0
	out := r
	out.Experience = int(float64(r.Experience) * mult)
	out.Currency = int(float64(r.Currency) * mult)
	return out
}
