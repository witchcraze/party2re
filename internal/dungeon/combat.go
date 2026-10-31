package dungeon

import (
	"context"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corerandom "github.com/witchcraze/party2re/internal/core/random"
)

func (s *Service) buildCombatAllies(ctx context.Context, exp *ActiveExpedition, char *corecharacter.Character) ([]corebattle.Participant, error) {
	if len(exp.Members) == 0 {
		var playerPart corebattle.Participant
		var err error
		if s.participantBuilder != nil {
			playerPart, err = s.participantBuilder.BuildParticipantWithCurrentHP(ctx, char.ID, exp.CurrentHP)
			if err != nil {
				return nil, err
			}
		} else {
			playerPart = buildFallbackParticipantWithHP(*char, exp.CurrentHP)
		}
		playerPart.TeamID = "allies"
		return []corebattle.Participant{playerPart}, nil
	}

	allies := make([]corebattle.Participant, 0, len(exp.Members))
	for i := range exp.Members {
		m := &exp.Members[i]
		if m.CurrentHP <= 0 {
			continue
		}
		var p corebattle.Participant
		var err error
		if s.participantBuilder != nil {
			p, err = s.participantBuilder.BuildParticipantWithState(ctx, m.CharacterID, m.CurrentHP, &m.CurrentMP, m.ConsumedItems)
		} else {
			var memberChar corecharacter.Character
			if m.CharacterID == char.ID {
				memberChar = *char
			} else {
				memberChar, err = s.characterRepo.FindByID(ctx, m.CharacterID)
			}
			if err == nil {
				p = buildFallbackParticipantWithHP(memberChar, m.CurrentHP)
				p.MP = m.CurrentMP
				p.MaxMP = m.MaxMP
			}
		}
		if err != nil {
			return nil, err
		}
		p.TeamID = "allies"
		allies = append(allies, p)
	}
	return allies, nil
}

func (s *Service) resolveEncounter(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	monster DungeonMonster,
) (corebattle.Result, bool, map[string]int, map[string]int, map[string][]corebattle.ConsumedItem, int, error) {
	allies, err := s.buildCombatAllies(ctx, exp, char)
	if err != nil {
		return corebattle.Result{}, false, nil, nil, nil, 0, err
	}
	if len(allies) == 0 {
		return corebattle.Result{Outcome: corebattle.OutcomeDefeat}, false, nil, nil, nil, -exp.CurrentHP, nil
	}

	monsterPart := corebattle.MustNewParticipant(monster.ID, monster.HP, monster.Attack, monster.Defense)
	monsterPart.Name = monster.Name
	monsterPart.Agility = monster.Agility
	monsterPart.TeamID = "enemies"

	var partyRes corebattle.PartyBattleResult
	var battleRes corebattle.Result

	if partyResolver, ok := s.battleEngine.(corebattle.PartyBattleResolver); ok {
		partyRes, err = partyResolver.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  allies,
			Enemies: []corebattle.Participant{monsterPart},
			RNG:     adaptRandomSource(s.rng),
		})
		if err != nil {
			return corebattle.Result{}, false, nil, nil, nil, 0, err
		}
		winnerID := monster.ID
		loserID := char.ID
		if partyRes.Outcome == corebattle.OutcomeWin && partyRes.WinnerSide == "allies" {
			winnerID = char.ID
			loserID = monster.ID
		}
		battleRes = corebattle.Result{
			Outcome:         partyRes.Outcome,
			WinnerID:        winnerID,
			LoserID:         loserID,
			Turns:           partyRes.Turns,
			RemainingHP:     partyRes.RemainingHP,
			RemainingMP:     partyRes.RemainingMP,
			RemainingStatus: partyRes.RemainingStatus,
			ConsumedItems:   partyRes.ConsumedItems,
			Logs:            partyRes.Logs,
		}
	} else {
		if len(allies) > 1 {
			return corebattle.Result{}, false, nil, nil, nil, 0, corebattle.ErrInvalidRequest
		}
		battleRes, err = s.battleEngine.Resolve(corebattle.Request{
			Participants: []corebattle.Participant{allies[0], monsterPart},
			RNG:          adaptRandomSource(s.rng),
		})
		if err != nil {
			return corebattle.Result{}, false, nil, nil, nil, 0, err
		}
		partyRes = corebattle.PartyBattleResult{
			Outcome:         battleRes.Outcome,
			WinnerSide:      "enemies",
			RemainingHP:     battleRes.RemainingHP,
			RemainingMP:     battleRes.RemainingMP,
			RemainingStatus: battleRes.RemainingStatus,
			ConsumedItems:   battleRes.ConsumedItems,
			Logs:            battleRes.Logs,
		}
		if battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == allies[0].ID {
			partyRes.WinnerSide = "allies"
		}
	}

	won := partyRes.Outcome == corebattle.OutcomeWin && partyRes.WinnerSide == "allies"
	memberHPDeltas := make(map[string]int)
	memberMPDeltas := make(map[string]int)
	consumedItems := make(map[string][]corebattle.ConsumedItem)

	if len(exp.Members) > 0 {
		for _, m := range exp.Members {
			if m.CurrentHP <= 0 {
				continue
			}
			if remHP, ok := partyRes.RemainingHP[m.CharacterID]; ok {
				memberHPDeltas[m.CharacterID] = remHP - m.CurrentHP
			} else if !won {
				memberHPDeltas[m.CharacterID] = -m.CurrentHP
			}
			if remMP, ok := partyRes.RemainingMP[m.CharacterID]; ok {
				memberMPDeltas[m.CharacterID] = remMP - m.CurrentMP
			}
			if items, ok := partyRes.ConsumedItems[m.CharacterID]; ok && len(items) > 0 {
				consumedItems[m.CharacterID] = items
			}
		}
	} else {
		hpDelta := 0
		if remHP, ok := battleRes.RemainingHP[char.ID]; ok {
			hpDelta = remHP - exp.CurrentHP
		} else if len(battleRes.Logs) > 0 {
			lastLog := battleRes.Logs[len(battleRes.Logs)-1]
			if remHP, ok := lastLog.RemainingHP[char.ID]; ok {
				hpDelta = remHP - exp.CurrentHP
			}
		} else if !won {
			hpDelta = -exp.CurrentHP
		}
		memberHPDeltas[char.ID] = hpDelta
	}

	leaderHPDelta := 0
	if d, ok := memberHPDeltas[char.ID]; ok {
		leaderHPDelta = d
	} else if !won {
		leaderHPDelta = -exp.CurrentHP
	}

	return battleRes, won, memberHPDeltas, memberMPDeltas, consumedItems, leaderHPDelta, nil
}

func (s *Service) resolveMonsterCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	monster DungeonMonster,
	eventType TileEventType,
	newX, newY int,
) (ExpeditionStepResult, error) {
	battleRes, won, memberHPDeltas, memberMPDeltas, consumedItems, leaderHPDelta, err := s.resolveEncounter(ctx, exp, char, monster)
	if err != nil {
		return ExpeditionStepResult{}, err
	}

	if won {
		memberExpDeltas := make(map[string]int, len(exp.Members))
		memberGoldDeltas := make(map[string]int, len(exp.Members))
		for _, m := range exp.Members {
			if remHP, ok := battleRes.RemainingHP[m.CharacterID]; ok && remHP > 0 {
				memberExpDeltas[m.CharacterID] = monster.ExpReward
				memberGoldDeltas[m.CharacterID] = monster.GoldReward
			}
		}
		leaderExpDelta := memberExpDeltas[char.ID]
		leaderGoldDelta := memberGoldDeltas[char.ID]
		if len(exp.Members) <= 1 && leaderHPDelta+exp.CurrentHP > 0 {
			leaderExpDelta = monster.ExpReward
			leaderGoldDelta = monster.GoldReward
		}

		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              leaderHPDelta,
			MemberHPDeltas:       memberHPDeltas,
			MemberMPDeltas:       memberMPDeltas,
			MemberExpDeltas:      memberExpDeltas,
			MemberGoldDeltas:     memberGoldDeltas,
			ConsumedItems:        consumedItems,
			TurnsDelta:           -1,
			ExpDelta:             leaderExpDelta,
			GoldDelta:            leaderGoldDelta,
			MedalsDelta:          0,
			RewardItemID:         monster.DropItemID,
			Now:                  time.Now().UTC(),
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}

		if s.monsterDefeatedHook != nil {
			_ = s.monsterDefeatedHook(ctx, char.ID, 1)
		}
		return ExpeditionStepResult{
			Expedition:   stepRes.Expedition,
			EventType:    eventType,
			BattleResult: &battleRes,
			ExpEarned:    leaderExpDelta,
			GoldFound:    leaderGoldDelta,
			ItemFound:    monster.DropItemID,
			Message:      fmt.Sprintf("%s との戦闘に勝利した！ (EXP: +%d, G: +%d)", monster.Name, monster.ExpReward, monster.GoldReward),
		}, nil
	}

	allDead := true
	if len(exp.Members) > 0 {
		for _, m := range exp.Members {
			if remHP, ok := battleRes.RemainingHP[m.CharacterID]; ok && remHP > 0 {
				allDead = false
				break
			}
		}
	} else {
		allDead = (exp.CurrentHP+leaderHPDelta <= 0)
	}

	if battleRes.Outcome == corebattle.OutcomeDraw && !allDead {
		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 exp.PosX,
			NewY:                 exp.PosY,
			HPDelta:              leaderHPDelta,
			MemberHPDeltas:       memberHPDeltas,
			MemberMPDeltas:       memberMPDeltas,
			ConsumedItems:        consumedItems,
			TurnsDelta:           -1,
			Now:                  time.Now().UTC(),
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}
		if stepRes.Status == StatusWipedOut {
			return s.handleWipeout(ctx, &stepRes.Expedition, char, fmt.Sprintf("%s との戦いが長引き、行動限界に達してしまった…", monster.Name))
		}
		return ExpeditionStepResult{
			Expedition:   stepRes.Expedition,
			EventType:    eventType,
			BattleResult: &battleRes,
			Message:      fmt.Sprintf("%s との戦いは時間切れで決着がつかなかった… (引き分け)", monster.Name),
		}, nil
	}

	// Player Defeated
	stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
		ExpectedExpeditionID: exp.ID,
		NewFloor:             exp.CurrentFloor,
		NewX:                 newX,
		NewY:                 newY,
		HPDelta:              leaderHPDelta,
		MemberHPDeltas:       memberHPDeltas,
		MemberMPDeltas:       memberMPDeltas,
		ConsumedItems:        consumedItems,
		TurnsDelta:           -1,
		Now:                  time.Now().UTC(),
	})
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	return s.handleWipeout(ctx, &stepRes.Expedition, char, fmt.Sprintf("%s との戦いに敗れ、全滅してしまった…", monster.Name))
}

func (s *Service) resolveBossCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	dungeon Dungeon,
	bossMonster DungeonMonster,
	newX, newY int,
) (ExpeditionStepResult, error) {
	battleRes, won, memberHPDeltas, memberMPDeltas, consumedItems, leaderHPDelta, err := s.resolveEncounter(ctx, exp, char, bossMonster)
	if err != nil {
		return ExpeditionStepResult{}, err
	}

	if won {
		memberExpDeltas := make(map[string]int, len(exp.Members))
		memberGoldDeltas := make(map[string]int, len(exp.Members))
		for _, m := range exp.Members {
			if remHP, ok := battleRes.RemainingHP[m.CharacterID]; ok && remHP > 0 {
				memberExpDeltas[m.CharacterID] = bossMonster.ExpReward
				memberGoldDeltas[m.CharacterID] = bossMonster.GoldReward
			}
		}
		leaderExpDelta := memberExpDeltas[char.ID]
		leaderGoldDelta := memberGoldDeltas[char.ID]
		if len(exp.Members) <= 1 && leaderHPDelta+exp.CurrentHP > 0 {
			leaderExpDelta = bossMonster.ExpReward
			leaderGoldDelta = bossMonster.GoldReward
		}

		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              leaderHPDelta,
			MemberHPDeltas:       memberHPDeltas,
			MemberMPDeltas:       memberMPDeltas,
			MemberExpDeltas:      memberExpDeltas,
			MemberGoldDeltas:     memberGoldDeltas,
			ConsumedItems:        consumedItems,
			TurnsDelta:           -1,
			ExpDelta:             leaderExpDelta,
			GoldDelta:            leaderGoldDelta,
			MedalsDelta:          0,
			RewardItemID:         bossMonster.DropItemID,
			Now:                  time.Now().UTC(),
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}

		if s.monsterDefeatedHook != nil {
			_ = s.monsterDefeatedHook(ctx, char.ID, 1)
		}
		return s.handleDungeonClear(ctx, &stepRes.Expedition, char, dungeon)
	}

	allDead := true
	if len(exp.Members) > 0 {
		for _, m := range exp.Members {
			if remHP, ok := battleRes.RemainingHP[m.CharacterID]; ok && remHP > 0 {
				allDead = false
				break
			}
		}
	} else {
		allDead = (exp.CurrentHP+leaderHPDelta <= 0)
	}

	if battleRes.Outcome == corebattle.OutcomeDraw && !allDead {
		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 exp.PosX,
			NewY:                 exp.PosY,
			HPDelta:              leaderHPDelta,
			MemberHPDeltas:       memberHPDeltas,
			MemberMPDeltas:       memberMPDeltas,
			ConsumedItems:        consumedItems,
			TurnsDelta:           -1,
			Now:                  time.Now().UTC(),
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}
		if stepRes.Status == StatusWipedOut {
			return s.handleWipeout(ctx, &stepRes.Expedition, char, fmt.Sprintf("フロアボス %s との戦いが長引き、行動限界に達してしまった…", bossMonster.Name))
		}
		return ExpeditionStepResult{
			Expedition:   stepRes.Expedition,
			EventType:    EventBoss,
			BattleResult: &battleRes,
			Message:      fmt.Sprintf("フロアボス %s との戦いは時間切れで決着がつかなかった… (引き分け)", bossMonster.Name),
		}, nil
	}

	// Defeat by Boss
	stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
		ExpectedExpeditionID: exp.ID,
		NewFloor:             exp.CurrentFloor,
		NewX:                 newX,
		NewY:                 newY,
		HPDelta:              leaderHPDelta,
		MemberHPDeltas:       memberHPDeltas,
		MemberMPDeltas:       memberMPDeltas,
		ConsumedItems:        consumedItems,
		TurnsDelta:           -1,
		Now:                  time.Now().UTC(),
	})
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	return s.handleWipeout(ctx, &stepRes.Expedition, char, fmt.Sprintf("フロアボス %s の圧倒的な力の前に敗れ去った…", bossMonster.Name))
}

func buildFallbackParticipantWithHP(char corecharacter.Character, currentHP int) corebattle.Participant {
	hp := currentHP
	if hp <= 0 {
		hp = char.Stats.HP
		if hp <= 0 && char.Stats.MaxHP > 0 {
			hp = char.Stats.MaxHP
		}
	}
	p := corebattle.MustNewParticipant(char.ID, hp, char.Stats.Attack, char.Stats.Defense)
	p.Name = char.Name
	p.MaxHP = char.Stats.MaxHP
	p.MP = char.Stats.MP
	p.MaxMP = char.Stats.MaxMP
	p.Agility = char.Stats.Agility
	return p
}

type randomSourceAdapter struct {
	src RandomSource
}

func (a randomSourceAdapter) Intn(n int) int {
	return a.src.Intn(n)
}

func (a randomSourceAdapter) IntN(n int) int {
	return a.src.Intn(n)
}

func (a randomSourceAdapter) Float64() float64 {
	return a.src.Float64()
}

func adaptRandomSource(src RandomSource) corerandom.Generator {
	if src == nil {
		return nil
	}
	if gen, ok := src.(corerandom.Generator); ok {
		return gen
	}
	return randomSourceAdapter{src: src}
}
