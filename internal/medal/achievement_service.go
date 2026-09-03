package medal

import (
	"context"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// GetAchievementCatalog returns all registered achievement definitions.
func (s *Service) GetAchievementCatalog() []Achievement {
	res := make([]Achievement, len(s.achievements))
	copy(res, s.achievements)
	return res
}

// RecordProgress updates character progress toward achievements matching the specified metric.
func (s *Service) RecordProgress(ctx context.Context, characterID string, metric MetricType, amount int) error {
	if characterID == "" {
		return corecharacter.ErrNotFound
	}
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if s.achievementRepo == nil {
		return ErrAchievementRepoNotConfigured
	}

	var matching []Achievement
	for _, a := range s.achievements {
		if a.Metric == metric {
			matching = append(matching, a)
		}
	}
	if len(matching) == 0 {
		return nil
	}

	return s.achievementRepo.RecordProgress(ctx, characterID, metric, amount, matching)
}

// HandleEvent processes a gameplay DomainEvent and increments matching achievement milestones.
func (s *Service) HandleEvent(ctx context.Context, event DomainEvent) error {
	amount := event.Amount
	if amount <= 0 {
		amount = 1
	}
	return s.RecordProgress(ctx, event.CharacterID, event.Metric, amount)
}

// GetAchievements returns all achievements enriched with the character's current progress and completion state.
func (s *Service) GetAchievements(ctx context.Context, characterID string) ([]AchievementProgress, error) {
	if characterID == "" {
		return nil, corecharacter.ErrNotFound
	}
	if _, err := s.characters.FindByID(ctx, characterID); err != nil {
		return nil, err
	}

	var records []AchievementRecord
	if s.achievementRepo != nil {
		var err error
		records, err = s.achievementRepo.GetCharacterAchievements(ctx, characterID)
		if err != nil {
			return nil, err
		}
	}

	recMap := make(map[string]AchievementRecord, len(records))
	for _, r := range records {
		recMap[r.AchievementID] = r
	}

	result := make([]AchievementProgress, 0, len(s.achievements))
	for _, ach := range s.achievements {
		rec, found := recMap[ach.ID]
		currentProgress := 0
		isCompleted := false
		var completedAt *time.Time
		isClaimed := false
		var claimedAt *time.Time

		if found {
			currentProgress = rec.CurrentProgress
			isCompleted = rec.IsCompleted
			completedAt = rec.CompletedAt
			isClaimed = rec.IsClaimed
			claimedAt = rec.ClaimedAt
		}

		pct := 0.0
		if ach.Threshold > 0 {
			pct = (float64(currentProgress) / float64(ach.Threshold)) * 100.0
			if pct > 100.0 {
				pct = 100.0
			}
		}

		result = append(result, AchievementProgress{
			ID:                   ach.ID,
			Name:                 ach.Name,
			Category:             ach.Category,
			Description:          ach.Description,
			Metric:               ach.Metric,
			Threshold:            ach.Threshold,
			CurrentProgress:      currentProgress,
			CompletionPercentage: pct,
			IsCompleted:          isCompleted,
			CompletedAt:          completedAt,
			IsClaimed:            isClaimed,
			ClaimedAt:            claimedAt,
			MedalID:              ach.MedalID,
			MedalName:            ach.MedalName,
			MedalDescription:     ach.MedalDescription,
			SmallMedalsReward:    ach.SmallMedalsReward,
		})
	}

	return result, nil
}

// ClaimAchievement claims the commemorative medal and bonus small medals for a completed achievement milestone.
func (s *Service) ClaimAchievement(ctx context.Context, characterID string, achievementID string) (ClaimResult, error) {
	if characterID == "" {
		return ClaimResult{}, corecharacter.ErrNotFound
	}
	if achievementID == "" {
		return ClaimResult{}, ErrAchievementNotFound
	}
	if s.achievementRepo == nil {
		return ClaimResult{}, ErrAchievementRepoNotConfigured
	}

	var targetAch *Achievement
	for _, a := range s.achievements {
		if a.ID == achievementID {
			targetAch = &a
			break
		}
	}
	if targetAch == nil {
		return ClaimResult{}, ErrAchievementNotFound
	}

	var result ClaimResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character row first (Deterministic hierarchy: characters before feature rows)
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// 2. Lock achievement record row
		rec, err := s.achievementRepo.GetAchievementForUpdate(txCtx, characterID, achievementID)
		if err != nil {
			return err
		}

		// 3. Validation checks
		if !rec.IsCompleted {
			return ErrAchievementNotCompleted
		}
		if rec.IsClaimed {
			return ErrAchievementAlreadyClaimed
		}

		now := time.Now().UTC()

		// 4. Mark claimed
		if err := s.achievementRepo.MarkAchievementClaimed(txCtx, characterID, achievementID, now); err != nil {
			return err
		}

		// 5. Award commemorative medal
		charMedal := CharacterMedal{
			CharacterID: characterID,
			MedalID:     targetAch.MedalID,
			MedalName:   targetAch.MedalName,
			Category:    targetAch.Category,
			Description: targetAch.MedalDescription,
			AwardedAt:   now,
		}
		if err := s.achievementRepo.SaveMedal(txCtx, charMedal); err != nil {
			return err
		}

		// 6. Award small medals if configured
		if targetAch.SmallMedalsReward > 0 {
			if err := char.AddSmallMedals(targetAch.SmallMedalsReward); err != nil {
				return err
			}
			if err := s.characters.Update(txCtx, char); err != nil {
				return err
			}
		}

		result = ClaimResult{
			AchievementID:      targetAch.ID,
			AchievementName:    targetAch.Name,
			Medal:              charMedal,
			SmallMedalsAwarded: targetAch.SmallMedalsReward,
			Character:          char,
		}
		return nil
	})
	if err != nil {
		return ClaimResult{}, err
	}

	return result, nil
}

// GetCharacterMedals returns all commemorative medals earned by the character.
func (s *Service) GetCharacterMedals(ctx context.Context, characterID string) ([]CharacterMedal, error) {
	if characterID == "" {
		return nil, corecharacter.ErrNotFound
	}
	if _, err := s.characters.FindByID(ctx, characterID); err != nil {
		return nil, err
	}
	if s.achievementRepo == nil {
		return []CharacterMedal{}, nil
	}
	return s.achievementRepo.GetCharacterMedals(ctx, characterID)
}
