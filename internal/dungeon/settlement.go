package dungeon

import (
	"context"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
)

func (s *Service) Escape(ctx context.Context, characterID string) (ExpeditionStepResult, error) {
	if characterID == "" {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}
	exp, err := s.repo.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if exp == nil || exp.Status != StatusExploring {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}
	return s.handleEscape(ctx, exp, &char, "探索を中断し、戦利品を持って無事に脱出した！")
}

func (s *Service) handleDungeonClear(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	dungeon Dungeon,
) (ExpeditionStepResult, error) {
	now := time.Now().UTC()
	exp.Status = StatusCleared
	exp.AccumulatedExp += dungeon.ClearExpBonus
	exp.AccumulatedGold += dungeon.ClearGoldBonus
	exp.UpdatedAt = now

	// 1. Commit accumulated rewards to character
	if exp.AccumulatedExp > 0 {
		_, _ = progression.ApplyExperience(char, exp.AccumulatedExp)
	}
	_ = char.AddMoney(exp.AccumulatedGold)
	medalsBonus := dungeon.Tier
	exp.AccumulatedMedals += medalsBonus
	_ = char.AddSmallMedals(exp.AccumulatedMedals)

	rewardItems := make([]coreitem.Instance, 0, len(exp.AccumulatedItems))
	for _, defID := range exp.AccumulatedItems {
		itemID := id.New()
		rewardItems = append(rewardItems, coreitem.Instance{
			ID:               itemID,
			DefinitionID:     defID,
			Quantity:         1,
			EnhancementLevel: 0,
		})
	}

	// 2. Update Dungeon Record
	rec, _ := s.repo.GetRecord(ctx, char.ID)
	rec.TotalExpeditions++
	rec.TotalFloorsCleared += exp.CurrentFloor
	if dungeon.Tier > rec.HighestDungeonCleared {
		rec.HighestDungeonCleared = dungeon.Tier
	}

	// 3. Save History
	histID := id.New()
	history := DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        dungeon.ID,
		FloorsReached:    exp.CurrentFloor,
		Outcome:          StatusCleared,
		ExpReward:        exp.AccumulatedExp,
		GoldReward:       exp.AccumulatedGold,
		MedalsReward:     exp.AccumulatedMedals,
		ItemsRewardCount: len(rewardItems),
		CreatedAt:        now,
	}

	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, rewardItems); err != nil {
		return ExpeditionStepResult{}, err
	}

	_ = s.repo.DeleteActiveExpedition(ctx, char.ID)

	return ExpeditionStepResult{
		Expedition:  *exp,
		EventType:   EventBoss,
		ExpEarned:   exp.AccumulatedExp,
		GoldFound:   exp.AccumulatedGold,
		MedalsFound: exp.AccumulatedMedals,
		IsFinished:  true,
		Message:     fmt.Sprintf("ダンジョン「%s」を踏破・完全制覇した！ (EXP: +%d, Gold: +%d, メダル: %d枚, アイテム: %d個)", dungeon.Name, exp.AccumulatedExp, exp.AccumulatedGold, exp.AccumulatedMedals, len(rewardItems)),
	}, nil
}

func (s *Service) handleEscape(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	msg string,
) (ExpeditionStepResult, error) {
	now := time.Now().UTC()
	exp.Status = StatusEscaped
	exp.UpdatedAt = now

	// Transfer accumulated EXP & Gold to character
	if exp.AccumulatedExp > 0 {
		_, _ = progression.ApplyExperience(char, exp.AccumulatedExp)
	}
	_ = char.AddMoney(exp.AccumulatedGold)
	_ = char.AddSmallMedals(exp.AccumulatedMedals)

	rewardItems := make([]coreitem.Instance, 0, len(exp.AccumulatedItems))
	for _, defID := range exp.AccumulatedItems {
		itemID := id.New()
		rewardItems = append(rewardItems, coreitem.Instance{
			ID:               itemID,
			DefinitionID:     defID,
			Quantity:         1,
			EnhancementLevel: 0,
		})
	}

	rec, _ := s.repo.GetRecord(ctx, char.ID)
	rec.TotalExpeditions++
	rec.TotalFloorsCleared += exp.CurrentFloor

	histID := id.New()
	history := DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        exp.DungeonID,
		FloorsReached:    exp.CurrentFloor,
		Outcome:          StatusEscaped,
		ExpReward:        exp.AccumulatedExp,
		GoldReward:       exp.AccumulatedGold,
		MedalsReward:     exp.AccumulatedMedals,
		ItemsRewardCount: len(rewardItems),
		CreatedAt:        now,
	}

	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, rewardItems); err != nil {
		return ExpeditionStepResult{}, err
	}

	_ = s.repo.DeleteActiveExpedition(ctx, char.ID)

	return ExpeditionStepResult{
		Expedition:  *exp,
		EventType:   EventEscape,
		ExpEarned:   exp.AccumulatedExp,
		GoldFound:   exp.AccumulatedGold,
		MedalsFound: exp.AccumulatedMedals,
		IsFinished:  true,
		Message:     msg,
	}, nil
}

func (s *Service) handleWipeout(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	msg string,
) (ExpeditionStepResult, error) {
	now := time.Now().UTC()
	exp.Status = StatusWipedOut
	exp.UpdatedAt = now

	rec, _ := s.repo.GetRecord(ctx, char.ID)
	rec.TotalExpeditions++

	histID := id.New()
	history := DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        exp.DungeonID,
		FloorsReached:    exp.CurrentFloor,
		Outcome:          StatusWipedOut,
		ExpReward:        0,
		GoldReward:       0,
		ItemsRewardCount: 0,
		CreatedAt:        now,
	}

	// Wiping out forfeits unbanked ledger rewards (0 EXP, 0 Gold, 0 items awarded)
	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, nil); err != nil {
		return ExpeditionStepResult{}, err
	}

	_ = s.repo.DeleteActiveExpedition(ctx, char.ID)

	return ExpeditionStepResult{
		Expedition: *exp,
		EventType:  EventWipeout,
		IsFinished: true,
		Message:    msg,
	}, nil
}
