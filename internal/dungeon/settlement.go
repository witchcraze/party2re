package dungeon

import (
	"context"
	"errors"
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
	exp, err := s.activeStore.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if exp == nil || exp.Status != StatusExploring {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
	if exp.CharacterID != characterID {
		return ExpeditionStepResult{}, ErrForbidden
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

	for i := range exp.Members {
		if exp.Members[i].CurrentHP > 0 {
			exp.Members[i].AccumulatedExp += dungeon.ClearExpBonus
			exp.Members[i].AccumulatedGold += dungeon.ClearGoldBonus
		}
	}

	// 1. Commit accumulated rewards to character
	if exp.AccumulatedExp > 0 {
		if _, err := progression.ApplyExperience(char, exp.AccumulatedExp); err != nil {
			return ExpeditionStepResult{}, fmt.Errorf("applying experience: %w", err)
		}
	}
	if err := char.AddMoney(exp.AccumulatedGold); err != nil {
		return ExpeditionStepResult{}, fmt.Errorf("adding gold: %w", err)
	}
	medalsBonus := dungeon.Tier
	exp.AccumulatedMedals += medalsBonus
	if err := char.AddSmallMedals(exp.AccumulatedMedals); err != nil {
		return ExpeditionStepResult{}, fmt.Errorf("adding small medals: %w", err)
	}

	char.Stats.HP = exp.CurrentHP
	for _, m := range exp.Members {
		if m.CharacterID == char.ID {
			char.Stats.HP = m.CurrentHP
			char.Stats.MP = m.CurrentMP
			break
		}
	}
	char.Stats.ClampVitality()

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
	rec, err := s.repo.GetRecord(ctx, char.ID)
	if err != nil {
		return ExpeditionStepResult{}, fmt.Errorf("getting dungeon record: %w", err)
	}
	rec.TotalExpeditions++
	rec.TotalFloorsCleared += exp.CurrentFloor
	if dungeon.Tier > rec.HighestDungeonCleared {
		rec.HighestDungeonCleared = dungeon.Tier
	}

	// 3. Save History
	histID := exp.ID
	if histID == "" {
		histID = id.New()
	}
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

	// Two-Phase Settlement: commit durable state to MariaDB first
	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, rewardItems, exp.Members...); err != nil && !errors.Is(err, ErrExpeditionAlreadyFinished) {
		return ExpeditionStepResult{}, err
	}

	// Upon successful MariaDB commit, purge transient buffer from Valkey Master
	//lint:ignore error-swallow best-effort post-commit cache eviction
	_ = s.activeStore.DeleteActiveExpedition(ctx, char.ID, exp.ID)
	//lint:ignore error-swallow best-effort post-commit fallback cleanup
	_ = s.repo.DeleteActiveExpedition(ctx, char.ID, exp.ID)

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
		if _, err := progression.ApplyExperience(char, exp.AccumulatedExp); err != nil {
			return ExpeditionStepResult{}, fmt.Errorf("applying experience: %w", err)
		}
	}
	if err := char.AddMoney(exp.AccumulatedGold); err != nil {
		return ExpeditionStepResult{}, fmt.Errorf("adding gold: %w", err)
	}
	if err := char.AddSmallMedals(exp.AccumulatedMedals); err != nil {
		return ExpeditionStepResult{}, fmt.Errorf("adding small medals: %w", err)
	}

	char.Stats.HP = exp.CurrentHP
	for _, m := range exp.Members {
		if m.CharacterID == char.ID {
			char.Stats.HP = m.CurrentHP
			char.Stats.MP = m.CurrentMP
			break
		}
	}
	char.Stats.ClampVitality()

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

	rec, err := s.repo.GetRecord(ctx, char.ID)
	if err != nil {
		return ExpeditionStepResult{}, fmt.Errorf("getting dungeon record: %w", err)
	}
	rec.TotalExpeditions++
	rec.TotalFloorsCleared += exp.CurrentFloor

	histID := exp.ID
	if histID == "" {
		histID = id.New()
	}
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

	// Two-Phase Settlement: commit durable state to MariaDB first
	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, rewardItems, exp.Members...); err != nil && !errors.Is(err, ErrExpeditionAlreadyFinished) {
		return ExpeditionStepResult{}, err
	}

	// Upon successful MariaDB commit, purge transient buffer from Valkey Master
	//lint:ignore error-swallow best-effort post-commit cache eviction
	_ = s.activeStore.DeleteActiveExpedition(ctx, char.ID, exp.ID)
	//lint:ignore error-swallow best-effort post-commit fallback cleanup
	_ = s.repo.DeleteActiveExpedition(ctx, char.ID, exp.ID)

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

	rec, err := s.repo.GetRecord(ctx, char.ID)
	if err != nil {
		return ExpeditionStepResult{}, fmt.Errorf("getting dungeon record: %w", err)
	}
	rec.TotalExpeditions++

	histID := exp.ID
	if histID == "" {
		histID = id.New()
	}
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

	char.Stats.HP = exp.CurrentHP
	for _, m := range exp.Members {
		if m.CharacterID == char.ID {
			char.Stats.HP = m.CurrentHP
			char.Stats.MP = m.CurrentMP
			break
		}
	}
	char.Stats.ClampVitality()

	// Wiping out forfeits unbanked ledger rewards (0 EXP, 0 Gold, 0 items awarded)
	// Two-Phase Settlement: commit durable state to MariaDB first
	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, nil, exp.Members...); err != nil && !errors.Is(err, ErrExpeditionAlreadyFinished) {
		return ExpeditionStepResult{}, err
	}

	// Upon successful MariaDB commit, purge transient buffer from Valkey Master
	//lint:ignore error-swallow best-effort post-commit cache eviction
	_ = s.activeStore.DeleteActiveExpedition(ctx, char.ID, exp.ID)
	//lint:ignore error-swallow best-effort post-commit fallback cleanup
	_ = s.repo.DeleteActiveExpedition(ctx, char.ID, exp.ID)

	return ExpeditionStepResult{
		Expedition: *exp,
		EventType:  EventWipeout,
		IsFinished: true,
		Message:    msg,
	}, nil
}
