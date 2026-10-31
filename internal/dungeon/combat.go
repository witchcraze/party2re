package dungeon

import (
	"context"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corerandom "github.com/witchcraze/party2re/internal/core/random"
)

func (s *Service) resolveMonsterCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	monster DungeonMonster,
	eventType TileEventType,
	newX, newY int,
) (ExpeditionStepResult, error) {
	var playerPart corebattle.Participant
	if s.participantBuilder != nil {
		var err error
		playerPart, err = s.participantBuilder.BuildParticipantWithCurrentHP(ctx, char.ID, exp.CurrentHP)
		if err != nil {
			return ExpeditionStepResult{}, err
		}
	} else {
		playerPart = buildFallbackParticipantWithHP(*char, exp.CurrentHP)
	}

	monsterPart := corebattle.MustNewParticipant(monster.ID, monster.HP, monster.Attack, monster.Defense)
	monsterPart.Name = monster.Name
	monsterPart.Agility = monster.Agility

	req := corebattle.Request{
		Participants: []corebattle.Participant{
			playerPart,
			monsterPart,
		},
		RNG: adaptRandomSource(s.rng),
	}

	battleRes, err := s.battleEngine.Resolve(req)
	if err != nil {
		return ExpeditionStepResult{}, err
	}

	if battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == char.ID {
		hpDelta := 0
		if remHP, ok := battleRes.RemainingHP[char.ID]; ok {
			hpDelta = remHP - exp.CurrentHP
		} else if len(battleRes.Logs) > 0 {
			lastLog := battleRes.Logs[len(battleRes.Logs)-1]
			if remHP, ok := lastLog.RemainingHP[char.ID]; ok {
				hpDelta = remHP - exp.CurrentHP
			}
		}

		memberHPDeltas := map[string]int{char.ID: hpDelta}

		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              hpDelta,
			MemberHPDeltas:       memberHPDeltas,
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
		MemberHPDeltas:       map[string]int{char.ID: -exp.CurrentHP},
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
	var playerPart corebattle.Participant
	if s.participantBuilder != nil {
		var err error
		playerPart, err = s.participantBuilder.BuildParticipantWithCurrentHP(ctx, char.ID, exp.CurrentHP)
		if err != nil {
			return ExpeditionStepResult{}, err
		}
	} else {
		playerPart = buildFallbackParticipantWithHP(*char, exp.CurrentHP)
	}

	bossPart := corebattle.MustNewParticipant(bossMonster.ID, bossMonster.HP, bossMonster.Attack, bossMonster.Defense)
	bossPart.Name = bossMonster.Name
	bossPart.Agility = bossMonster.Agility

	req := corebattle.Request{
		Participants: []corebattle.Participant{
			playerPart,
			bossPart,
		},
		RNG: adaptRandomSource(s.rng),
	}

	battleRes, err := s.battleEngine.Resolve(req)
	if err != nil {
		return ExpeditionStepResult{}, err
	}

	if battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == char.ID {
		hpDelta := 0
		if remHP, ok := battleRes.RemainingHP[char.ID]; ok {
			hpDelta = remHP - exp.CurrentHP
		} else if len(battleRes.Logs) > 0 {
			lastLog := battleRes.Logs[len(battleRes.Logs)-1]
			if remHP, ok := lastLog.RemainingHP[char.ID]; ok {
				hpDelta = remHP - exp.CurrentHP
			}
		}

		memberHPDeltas := map[string]int{char.ID: hpDelta}

		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              hpDelta,
			MemberHPDeltas:       memberHPDeltas,
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
		MemberHPDeltas:       map[string]int{char.ID: -exp.CurrentHP},
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
