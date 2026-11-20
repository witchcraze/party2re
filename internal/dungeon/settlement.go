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
	return s.handleEscape(ctx, exp, &char)
}

func (s *Service) handleEscape(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
) (ExpeditionStepResult, error) {
	if err := s.freezeExpedition(ctx, exp, StatusEscaped); err != nil {
		return ExpeditionStepResult{}, err
	}

	// Transfer accumulated EXP & Gold to character
	if exp.AccumulatedExp > 0 {
		if _, err := progression.ApplyExperience(char, exp.AccumulatedExp); err != nil {
			return ExpeditionStepResult{}, fmt.Errorf("applying experience: %w", err)
		}
	}
	if err := char.AddMoney(exp.AccumulatedGold); err != nil {
		return ExpeditionStepResult{}, fmt.Errorf("adding gold: %w", err)
	}

	rewardItems := make([]coreitem.Instance, 0, len(exp.AccumulatedItems))
	for _, defID := range exp.AccumulatedItems {
		rewardItems = append(rewardItems, coreitem.Instance{
			ID:               id.New(),
			DefinitionID:     defID,
			Quantity:         1,
			EnhancementLevel: 0,
		})
	}

	return s.finalizeFrozenSettlement(ctx, exp, char, StatusEscaped, EventEscape, rewardItems, exp.AccumulatedExp, exp.AccumulatedGold)
}

// handleWipeout settles a carried-over pre-update frozen wiped_out buffer without creating new wiped_out runs.
func (s *Service) handleWipeout(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
) (ExpeditionStepResult, error) {
	if exp.Status != StatusWipedOut {
		return ExpeditionStepResult{}, ErrExpeditionStateChanged
	}
	return s.finalizeFrozenSettlement(ctx, exp, char, StatusWipedOut, EventWipeout, nil, 0, 0)
}

func (s *Service) finalizeFrozenSettlement(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	status ExpeditionStatus,
	eventType TileEventType,
	rewardItems []coreitem.Instance,
	expEarned int,
	goldFound int,
) (ExpeditionStepResult, error) {
	now := time.Now().UTC()
	exp.Encounter = nil
	exp.Status = status
	exp.UpdatedAt = now

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

	histID := exp.ID
	if histID == "" {
		histID = id.New()
	}
	history := DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        exp.DungeonID,
		Outcome:          status,
		ExpReward:        expEarned,
		GoldReward:       goldFound,
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
		Expedition: *exp,
		EventType:  eventType,
		ExpEarned:  expEarned,
		GoldFound:  goldFound,
		IsFinished: true,
	}, nil
}
