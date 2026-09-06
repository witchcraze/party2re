package dungeon

import (
	"context"
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func (s *Service) resolveMonsterCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	monster DungeonMonster,
	eventType TileEventType,
) (ExpeditionStepResult, error) {
	req := corebattle.Request{
		Participants: []corebattle.Participant{
			corebattle.NewParticipantFromCharacterWithHP(*char, exp.CurrentHP),
			corebattle.MustNewParticipant(monster.ID, monster.HP, monster.Attack, monster.Defense),
		},
	}

	battleRes, err := s.battleEngine.Resolve(req)
	if err != nil {
		return ExpeditionStepResult{}, err
	}

	if battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == char.ID {
		exp.AccumulatedExp += monster.ExpReward
		exp.AccumulatedGold += monster.GoldReward
		if monster.DropItemID != "" {
			exp.AccumulatedItems = append(exp.AccumulatedItems, monster.DropItemID)
		}
		if err := s.repo.SaveActiveExpedition(ctx, *exp); err != nil {
			return ExpeditionStepResult{}, err
		}
		if s.monsterDefeatedHook != nil {
			_ = s.monsterDefeatedHook(ctx, char.ID, 1)
		}
		return ExpeditionStepResult{
			Expedition:   *exp,
			EventType:    eventType,
			BattleResult: &battleRes,
			ExpEarned:    monster.ExpReward,
			GoldFound:    monster.GoldReward,
			ItemFound:    monster.DropItemID,
			Message:      fmt.Sprintf("%s との戦闘に勝利した！ (EXP: +%d, G: +%d)", monster.Name, monster.ExpReward, monster.GoldReward),
		}, nil
	}

	// Player Defeated
	exp.CurrentHP = 0
	return s.handleWipeout(ctx, exp, char, fmt.Sprintf("%s との戦いに敗れ、全滅してしまった…", monster.Name))
}

func (s *Service) resolveBossCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	dungeon Dungeon,
	bossMonster DungeonMonster,
) (ExpeditionStepResult, error) {
	req := corebattle.Request{
		Participants: []corebattle.Participant{
			corebattle.NewParticipantFromCharacterWithHP(*char, exp.CurrentHP),
			corebattle.MustNewParticipant(bossMonster.ID, bossMonster.HP, bossMonster.Attack, bossMonster.Defense),
		},
	}

	battleRes, err := s.battleEngine.Resolve(req)
	if err != nil {
		return ExpeditionStepResult{}, err
	}

	if battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == char.ID {
		exp.AccumulatedExp += bossMonster.ExpReward
		exp.AccumulatedGold += bossMonster.GoldReward
		if bossMonster.DropItemID != "" {
			exp.AccumulatedItems = append(exp.AccumulatedItems, bossMonster.DropItemID)
		}
		if s.monsterDefeatedHook != nil {
			_ = s.monsterDefeatedHook(ctx, char.ID, 1)
		}
		return s.handleDungeonClear(ctx, exp, char, dungeon)
	}

	// Defeat by Boss
	exp.CurrentHP = 0
	return s.handleWipeout(ctx, exp, char, fmt.Sprintf("フロアボス %s の圧倒的な力の前に敗れ去った…", bossMonster.Name))
}
