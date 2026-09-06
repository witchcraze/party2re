package dungeon

import (
	"context"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func (s *Service) resolveMonsterCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	monster DungeonMonster,
	eventType TileEventType,
	newX, newY int,
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
		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              0,
			TurnsDelta:           -1,
			ExpDelta:             monster.ExpReward,
			GoldDelta:            monster.GoldReward,
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
			ExpEarned:    monster.ExpReward,
			GoldFound:    monster.GoldReward,
			ItemFound:    monster.DropItemID,
			Message:      fmt.Sprintf("%s との戦闘に勝利した！ (EXP: +%d, G: +%d)", monster.Name, monster.ExpReward, monster.GoldReward),
		}, nil
	}

	// Player Defeated
	stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
		ExpectedExpeditionID: exp.ID,
		NewFloor:             exp.CurrentFloor,
		NewX:                 newX,
		NewY:                 newY,
		HPDelta:              -exp.CurrentHP,
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
		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              0,
			TurnsDelta:           -1,
			ExpDelta:             bossMonster.ExpReward,
			GoldDelta:            bossMonster.GoldReward,
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

	// Defeat by Boss
	stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
		ExpectedExpeditionID: exp.ID,
		NewFloor:             exp.CurrentFloor,
		NewX:                 newX,
		NewY:                 newY,
		HPDelta:              -exp.CurrentHP,
		TurnsDelta:           -1,
		Now:                  time.Now().UTC(),
	})
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	return s.handleWipeout(ctx, &stepRes.Expedition, char, fmt.Sprintf("フロアボス %s の圧倒的な力の前に敗れ去った…", bossMonster.Name))
}
