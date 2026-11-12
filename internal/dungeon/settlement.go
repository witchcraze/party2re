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
	if exp == nil {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
	if exp.CharacterID != characterID {
		return ExpeditionStepResult{}, ErrForbidden
	}

	if exp.Status != StatusExploring {
		return s.resumeSettlement(ctx, exp)
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return ExpeditionStepResult{}, ErrCharacterNotFound
		}
		return ExpeditionStepResult{}, err
	}
	return s.handleEscape(ctx, exp, &char, "探索を中断し、戦利品を持って無事に脱出した！")
}

func (s *Service) handleEscape(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	msg string,
) (ExpeditionStepResult, error) {
	if err := s.freezeExpedition(ctx, exp, StatusEscaped); err != nil {
		return ExpeditionStepResult{}, err
	}
	now := time.Now().UTC()
	exp.Encounter = nil
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
			if m.HasCurrentMP() {
				char.Stats.MP = m.CurrentMP
			}
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
	if err := s.repo.FinalizeExpedition(ctx, history, char, rewardItems, exp.Members...); err != nil && !errors.Is(err, ErrExpeditionAlreadyFinished) {
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
	if err := s.freezeExpedition(ctx, exp, StatusWipedOut); err != nil {
		return ExpeditionStepResult{}, err
	}
	now := time.Now().UTC()
	exp.Encounter = nil
	exp.Status = StatusWipedOut
	exp.UpdatedAt = now

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
			if m.HasCurrentMP() {
				char.Stats.MP = m.CurrentMP
			}
			break
		}
	}
	char.Stats.ClampVitality()

	// Wiping out forfeits unbanked ledger rewards (0 EXP, 0 Gold, 0 items awarded)
	// Two-Phase Settlement: commit durable state to MariaDB first
	if err := s.repo.FinalizeExpedition(ctx, history, char, nil, exp.Members...); err != nil && !errors.Is(err, ErrExpeditionAlreadyFinished) {
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
