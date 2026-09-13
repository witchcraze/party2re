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

// ResolvePartyBattle resolves a multi-participant, multi-turn battle between allies and enemies or multiple teams.
// Evaluates agility turn order, skills (MP), custom skills (CMP), elemental fields, and defeat revival.
func (Engine) ResolvePartyBattle(req PartyBattleRequest) (PartyBattleResult, error) {
	for _, reward := range []Reward{req.VictoryReward, req.DefeatReward, req.DrawReward} {
		if err := validateReward(reward); err != nil {
			return PartyBattleResult{}, err
		}
	}

	var allParticipants []Participant
	teamMap := make(map[string]string)
	teamMembers := make(map[string][]Participant)
	var teamOrder []string

	if len(req.Teams) > 0 {
		if len(req.Teams) < 2 {
			return PartyBattleResult{}, ErrInvalidRequest
		}
		for tID := range req.Teams {
			teamOrder = append(teamOrder, tID)
		}
		sort.Strings(teamOrder)

		for _, tID := range teamOrder {
			members := req.Teams[tID]
			if len(members) == 0 {
				return PartyBattleResult{}, ErrInvalidRequest
			}
			for _, p := range members {
				if err := validateParticipant(p); err != nil {
					return PartyBattleResult{}, err
				}
				if p.TeamID == "" {
					p.TeamID = tID
				}
				allParticipants = append(allParticipants, p)
				teamMap[p.ID] = p.TeamID
				teamMembers[p.TeamID] = append(teamMembers[p.TeamID], p)
			}
		}
		if len(req.Allies) == 0 && len(teamOrder) > 0 {
			req.Allies = teamMembers[teamOrder[0]]
			for _, tID := range teamOrder[1:] {
				req.Enemies = append(req.Enemies, teamMembers[tID]...)
			}
		}
	} else {
		if len(req.Allies) == 0 || len(req.Enemies) == 0 {
			return PartyBattleResult{}, ErrInvalidRequest
		}
		for _, a := range req.Allies {
			if err := validateParticipant(a); err != nil {
				return PartyBattleResult{}, err
			}
			tID := a.TeamID
			if tID == "" {
				tID = "allies"
				a.TeamID = tID
			}
			allParticipants = append(allParticipants, a)
			teamMap[a.ID] = tID
			if _, exists := teamMembers[tID]; !exists {
				teamOrder = append(teamOrder, tID)
			}
			teamMembers[tID] = append(teamMembers[tID], a)
		}
		for _, e := range req.Enemies {
			if err := validateParticipant(e); err != nil {
				return PartyBattleResult{}, err
			}
			tID := e.TeamID
			if tID == "" {
				tID = "enemies"
				e.TeamID = tID
			}
			allParticipants = append(allParticipants, e)
			teamMap[e.ID] = tID
			if _, exists := teamMembers[tID]; !exists {
				teamOrder = append(teamOrder, tID)
			}
			teamMembers[tID] = append(teamMembers[tID], e)
		}
	}

	if len(teamMembers) < 2 {
		return PartyBattleResult{}, ErrInvalidRequest
	}

	seenIDs := make(map[string]bool)
	for _, p := range allParticipants {
		if seenIDs[p.ID] {
			return PartyBattleResult{}, ErrInvalidRequest
		}
		seenIDs[p.ID] = true
	}

	allyTeamID := teamOrder[0]

	bonusPercent := (len(req.Allies) - 1) * 10
	if bonusPercent < 0 {
		bonusPercent = 0
	} else if bonusPercent > 30 {
		bonusPercent = 30
	}

	ctx := &battleContext{
		req:             req,
		hpMap:           make(map[string]int),
		mpMap:           make(map[string]int),
		cmpMap:          make(map[string]int),
		defendingMap:    make(map[string]bool),
		statusMap:       make(map[string]string),
		attackBuff:      make(map[string]int),
		defenseBuff:     make(map[string]int),
		agilityBuff:     make(map[string]int),
		abilitiesMap:    make(map[string][]string),
		itemsMap:        make(map[string][]ActionItem),
		banishedMap:     make(map[string]bool),
		teamMap:         teamMap,
		allParticipants: allParticipants,
	}

	for _, p := range allParticipants {
		ctx.hpMap[p.ID] = p.HP
		ctx.mpMap[p.ID] = p.MP
		ctx.cmpMap[p.ID] = p.CMP
		ctx.abilitiesMap[p.ID] = append([]string(nil), p.Abilities...)
		ctx.defendingMap[p.ID] = p.Defending
		ctx.statusMap[p.ID] = p.Status
		ctx.itemsMap[p.ID] = append([]ActionItem(nil), p.ActionItems...)
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
		for i, p := range allParticipants {
			if ctx.hpMap[p.ID] > 0 && !ctx.banishedMap[p.ID] {
				combatants = append(combatants, combatantWrapper{
					participant: p,
					isAlly:      ctx.teamMap[p.ID] == allyTeamID,
					index:       i,
				})
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
			if ctx.hpMap[actorID] <= 0 || ctx.banishedMap[actorID] {
				continue
			}

			// Clear previous turn's defense stance when actor acts
			ctx.defendingMap[actorID] = false

			// Check status ailment incapacitation (paralysis, sleep)
			if ctx.checkStatusSkip(actor) {
				continue
			}

			actorTeam := ctx.teamMap[actorID]
			var opponents []Participant
			var allyParty []Participant
			var fullOpponents []Participant

			for _, p := range allParticipants {
				pTeam := ctx.teamMap[p.ID]
				if pTeam == actorTeam {
					if ctx.hpMap[p.ID] > 0 && !ctx.banishedMap[p.ID] {
						allyParty = append(allyParty, p)
					}
				} else {
					fullOpponents = append(fullOpponents, p)
					if ctx.hpMap[p.ID] > 0 && !ctx.banishedMap[p.ID] {
						opponents = append(opponents, p)
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
						if sk.Kind == ActionKindDejon {
							hasFallenTarget := false
							for _, opp := range fullOpponents {
								if ctx.hpMap[opp.ID] <= 0 && !ctx.banishedMap[opp.ID] {
									hasFallenTarget = true
									break
								}
							}
							if !hasFallenTarget {
								continue
							}
						}
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
				ctx.executeJobSkill(actor, usedSkill, allyParty, opponents, fullOpponents)
			} else if usedItem != nil {
				_ = ctx.executeItem(actor, usedItem, allyParty, opponents)
			} else if actor.Defending {
				ctx.executeDefend(actor)
			} else {
				ctx.executeNormalAttack(actor, opponents)
			}

			// Check battle termination early
			aliveTeams := make(map[string]bool)
			for _, p := range allParticipants {
				if ctx.hpMap[p.ID] > 0 && !ctx.banishedMap[p.ID] {
					aliveTeams[ctx.teamMap[p.ID]] = true
				}
			}
			if len(aliveTeams) <= 1 {
				break
			}
		}

		// Apply poison DOT at the end of each round
		ctx.applyPoisonDOT(allParticipants)

		// End of round field turn countdown
		ctx.field.EndTurn()

		aliveTeams := make(map[string]bool)
		for _, p := range allParticipants {
			if ctx.hpMap[p.ID] > 0 && !ctx.banishedMap[p.ID] {
				aliveTeams[ctx.teamMap[p.ID]] = true
			}
		}
		if len(aliveTeams) <= 1 {
			break
		}
	}

	aliveTeams := make(map[string]bool)
	var survivingTeam string
	for _, p := range allParticipants {
		if ctx.hpMap[p.ID] > 0 && !ctx.banishedMap[p.ID] {
			tID := ctx.teamMap[p.ID]
			aliveTeams[tID] = true
			survivingTeam = tID
		}
	}

	var alliesSurvived []string
	var alliesFallen []string
	for _, a := range req.Allies {
		if ctx.hpMap[a.ID] > 0 && !ctx.banishedMap[a.ID] {
			alliesSurvived = append(alliesSurvived, a.ID)
		} else {
			alliesFallen = append(alliesFallen, a.ID)
		}
	}

	var outcome Outcome
	var winnerSide string
	var winnerTeam string
	var selectedReward Reward

	if len(aliveTeams) == 0 {
		outcome = OutcomeDraw
		winnerSide = "none"
		winnerTeam = ""
		selectedReward = req.DrawReward
	} else if len(aliveTeams) == 1 {
		winnerTeam = survivingTeam
		if survivingTeam == allyTeamID {
			outcome = OutcomeWin
			winnerSide = "allies"
			selectedReward = req.VictoryReward
		} else {
			outcome = OutcomeDefeat
			if survivingTeam == "enemies" {
				winnerSide = "enemies"
			} else {
				winnerSide = survivingTeam
			}
			selectedReward = req.DefeatReward
		}
	} else {
		outcome = OutcomeDraw
		winnerSide = "none"
		winnerTeam = ""
		selectedReward = req.DrawReward
	}

	totalReward := applyBonus(selectedReward, bonusPercent)

	return PartyBattleResult{
		Outcome:         outcome,
		WinnerSide:      winnerSide,
		WinnerTeam:      winnerTeam,
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
		BanishedIDs:     copyBoolMap(ctx.banishedMap),
		Logs:            ctx.logs,
		FinalField:      ctx.field,
	}, nil
}

func copyBoolMap(m map[string]bool) map[string]bool {
	if m == nil {
		return nil
	}
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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
