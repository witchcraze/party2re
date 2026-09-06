package dungeon

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/witchcraze/party2re/internal/id"
)

func (s *Service) GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	return s.activeStore.GetActiveExpedition(ctx, characterID)
}

func (s *Service) StartExpedition(ctx context.Context, characterID, dungeonID string) (*ActiveExpedition, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	dungeon, ok := s.dungeonMap[dungeonID]
	if !ok {
		return nil, ErrDungeonNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	if char.Level < dungeon.MinLevel {
		return nil, ErrLevelRequirementNotMet
	}

	existing, err := s.activeStore.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status == StatusExploring {
		return nil, ErrActiveExpeditionExists
	}

	if len(dungeon.Floors) == 0 {
		return nil, errors.New("dungeon has no floors")
	}

	firstFloor := dungeon.Floors[0]
	expID := id.New()

	now := time.Now().UTC()
	exp := ActiveExpedition{
		ID:               expID,
		CharacterID:      char.ID,
		DungeonID:        dungeon.ID,
		CurrentFloor:     1,
		PosX:             firstFloor.StartX,
		PosY:             firstFloor.StartY,
		CurrentHP:        char.Stats.HP,
		TurnsRemaining:   dungeon.MaxTurnsPerFloor,
		AccumulatedExp:   0,
		AccumulatedGold:  0,
		AccumulatedItems: []string{},
		Status:           StatusExploring,
		StartedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.activeStore.SaveActiveExpedition(ctx, exp); err != nil {
		return nil, err
	}

	return &exp, nil
}

func (s *Service) Move(ctx context.Context, characterID string, dir Direction) (ExpeditionStepResult, error) {
	if characterID == "" {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}

	exp, err := s.activeStore.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if exp == nil || exp.Status != StatusExploring {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}

	dungeon, ok := s.dungeonMap[exp.DungeonID]
	if !ok {
		return ExpeditionStepResult{}, ErrDungeonNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}

	floorIdx := exp.CurrentFloor - 1
	if floorIdx < 0 || floorIdx >= len(dungeon.Floors) {
		return ExpeditionStepResult{}, errors.New("invalid floor index")
	}
	floor := dungeon.Floors[floorIdx]

	newX, newY := exp.PosX, exp.PosY
	switch dir {
	case DirectionNorth:
		newY--
	case DirectionSouth:
		newY++
	case DirectionEast:
		newX++
	case DirectionWest:
		newX--
	default:
		return ExpeditionStepResult{}, ErrInvalidDirection
	}

	if newX < 0 || newX >= floor.Width || newY < 0 || newY >= floor.Height {
		return ExpeditionStepResult{}, ErrImpassableWall
	}

	tileChar := floor.Grid[newY][newX]
	if tileChar == '1' {
		return ExpeditionStepResult{}, ErrImpassableWall
	}

	now := time.Now().UTC()

	// Dispatch Tile Event
	switch tileChar {
	case 'S', '0': // Start or Normal Path
		if tileChar == '0' && len(floor.Monsters) > 0 {
			monster := floor.Monsters[newX%len(floor.Monsters)]
			return s.resolveMonsterCombat(ctx, exp, &char, monster, EventBattle, newX, newY)
		}

		stepRes, err := s.activeStore.Step(ctx, characterID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              0,
			TurnsDelta:           -1,
			ExpDelta:             0,
			GoldDelta:            0,
			MedalsDelta:          0,
			RewardItemID:         "",
			Now:                  now,
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}

		if stepRes.Status == StatusWipedOut {
			return s.handleWipeout(ctx, &stepRes.Expedition, &char, "行動限界（ターン切れ）により意識を失い、探索に失敗した…")
		}

		return ExpeditionStepResult{
			Expedition: stepRes.Expedition,
			EventType:  EventMove,
			Message:    "静かな通路を進んだ。",
		}, nil

	case 'T': // Treasure Chest
		goldFound := 100 * exp.CurrentFloor
		itemFound := "potion"
		if len(floor.Monsters) > 0 && floor.Monsters[0].DropItemID != "" {
			itemFound = floor.Monsters[0].DropItemID
		}
		medalsFound := 1

		stepRes, err := s.activeStore.Step(ctx, characterID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              0,
			TurnsDelta:           -1,
			ExpDelta:             0,
			GoldDelta:            goldFound,
			MedalsDelta:          medalsFound,
			RewardItemID:         itemFound,
			Now:                  now,
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}

		if stepRes.Status == StatusWipedOut {
			return s.handleWipeout(ctx, &stepRes.Expedition, &char, "行動限界（ターン切れ）により意識を失い、探索に失敗した…")
		}

		return ExpeditionStepResult{
			Expedition:  stepRes.Expedition,
			EventType:   EventTreasure,
			GoldFound:   goldFound,
			MedalsFound: medalsFound,
			ItemFound:   itemFound,
			Message:     fmt.Sprintf("宝箱を発見した！ %d G と %s 、ちいさなメダル %d枚を手に入れた！", goldFound, itemFound, medalsFound),
		}, nil

	case 'X': // Hazard Trap
		trapDamage := int(math.Max(10, float64(char.Stats.MaxHP)*0.15))
		stepRes, err := s.activeStore.Step(ctx, characterID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              -trapDamage,
			TurnsDelta:           -1,
			ExpDelta:             0,
			GoldDelta:            0,
			MedalsDelta:          0,
			RewardItemID:         "",
			Now:                  now,
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}

		if stepRes.Status == StatusWipedOut {
			return s.handleWipeout(ctx, &stepRes.Expedition, &char, fmt.Sprintf("罠が作動し %d の猛烈なダメージを受けた！力尽きて倒れた…", trapDamage))
		}

		return ExpeditionStepResult{
			Expedition:  stepRes.Expedition,
			EventType:   EventTrap,
			DamageTaken: trapDamage,
			Message:     fmt.Sprintf("罠を踏んでしまった！ %d のダメージを受けた！", trapDamage),
		}, nil

	case 'D': // Down Stairs
		if exp.CurrentFloor < len(dungeon.Floors) {
			nextFloor := dungeon.Floors[exp.CurrentFloor]
			stepRes, err := s.activeStore.Step(ctx, characterID, StepParams{
				ExpectedExpeditionID: exp.ID,
				NewFloor:             exp.CurrentFloor + 1,
				NewX:                 nextFloor.StartX,
				NewY:                 nextFloor.StartY,
				HPDelta:              0,
				TurnsDelta:           dungeon.MaxTurnsPerFloor - exp.TurnsRemaining,
				ExpDelta:             0,
				GoldDelta:            0,
				MedalsDelta:          0,
				RewardItemID:         "",
				Now:                  now,
			})
			if err != nil {
				return ExpeditionStepResult{}, err
			}

			return ExpeditionStepResult{
				Expedition: stepRes.Expedition,
				EventType:  EventStairs,
				Message:    fmt.Sprintf("階段を発見し、地下 %d 階へ降りた！", stepRes.Expedition.CurrentFloor),
			}, nil
		}
		// If last floor has no boss, stair completes dungeon
		return s.handleDungeonClear(ctx, exp, &char, dungeon)

	case 'B': // Boss Battle
		bossMonster := floor.Boss
		if bossMonster == nil && len(floor.Monsters) > 0 {
			bossMonster = &floor.Monsters[0]
		}
		if bossMonster == nil {
			return s.handleDungeonClear(ctx, exp, &char, dungeon)
		}
		return s.resolveBossCombat(ctx, exp, &char, dungeon, *bossMonster, newX, newY)

	case 'E': // Safe Escape Exit
		return s.handleEscape(ctx, exp, &char, "脱出の魔法陣を発見し、無事に帰還した！")

	default:
		stepRes, err := s.activeStore.Step(ctx, characterID, StepParams{
			ExpectedExpeditionID: exp.ID,
			NewFloor:             exp.CurrentFloor,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              0,
			TurnsDelta:           -1,
			ExpDelta:             0,
			GoldDelta:            0,
			MedalsDelta:          0,
			RewardItemID:         "",
			Now:                  now,
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}

		if stepRes.Status == StatusWipedOut {
			return s.handleWipeout(ctx, &stepRes.Expedition, &char, "行動限界（ターン切れ）により意識を失い、探索に失敗した…")
		}

		return ExpeditionStepResult{
			Expedition: stepRes.Expedition,
			EventType:  EventMove,
			Message:    "通路を進んだ。",
		}, nil
	}
}
