package boss

import (
	"context"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
)

// ChallengeBoss orchestrates a single-player encounter against a king boss or world boss.
func (s *Service) ChallengeBoss(ctx context.Context, characterID, bossID string) (ChallengeResult, error) {
	if characterID == "" {
		return ChallengeResult{}, ErrCharacterNotFound
	}
	if bossID == "" {
		return ChallengeResult{}, ErrInvalidBossID
	}

	boss, ok := s.bossMap[bossID]
	if !ok {
		return ChallengeResult{}, ErrBossNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ChallengeResult{}, ErrCharacterNotFound
	}

	// 1. Validate level requirement
	if char.Level < boss.MinLevel {
		return ChallengeResult{}, ErrLevelRequirementNotMet
	}

	// 2. Validate boss record & prerequisites
	rec, err := s.repo.GetOrCreateRecord(ctx, characterID)
	if err != nil {
		return ChallengeResult{}, err
	}

	now := time.Now().UTC()
	rec.ResetDailyAttemptsIfExpired(now)

	if boss.Tier > 1 {
		prereqTier := boss.Tier - 1
		if boss.Tier == 99 {
			prereqTier = 10
		}
		if rec.HighestTierCleared < prereqTier {
			return ChallengeResult{}, ErrPrerequisiteNotMet
		}
	}

	// 3. Validate daily limit
	if rec.DailyAttemptsUsed >= boss.DailyEntryLimit {
		return ChallengeResult{}, ErrDailyAttemptsExhausted
	}

	// Consume 1 daily attempt
	rec.DailyAttemptsUsed++
	rec.LastChallengedAt = &now

	// 4. Combat Resolution
	req := corebattle.Request{
		Participants: []corebattle.Participant{
			corebattle.NewParticipantFromCharacter(char),
			corebattle.MustNewParticipant(boss.ID, boss.HP, boss.Attack, boss.Defense),
		},
	}

	battleResult, err := s.battleEngine.Resolve(req)
	if err != nil {
		return ChallengeResult{}, fmt.Errorf("battle resolution failed: %w", err)
	}

	outcome := battleResult.Outcome
	var expGained, goldGained, medalsGained int
	var rewardItemID string
	isFirstClear := false
	var rewardItemInstance *coreitem.Instance

	if outcome == corebattle.OutcomeWin && battleResult.WinnerID == char.ID {
		isFirstClear = rec.HighestTierCleared < boss.Tier
		expGained = boss.ExperienceReward
		goldGained = boss.GoldReward
		medalsGained = boss.SmallMedalReward

		if isFirstClear {
			expGained += boss.FirstClearExpBonus
			goldGained += boss.FirstClearGoldBonus
			medalsGained += boss.FirstClearMedalBonus
			if rec.FirstClearedAt == nil {
				rec.FirstClearedAt = &now
			}
			if boss.Tier > rec.HighestTierCleared {
				rec.HighestTierCleared = boss.Tier
			}
		}

		rec.TotalBossDefeats++

		if len(boss.DropItemIDs) > 0 {
			rewardItemID = boss.DropItemIDs[0]
			itemInstanceID := id.New()
			rewardItemInstance = &coreitem.Instance{
				ID:               itemInstanceID,
				DefinitionID:     rewardItemID,
				Quantity:         1,
				EnhancementLevel: 0,
			}
		}

		// Apply EXP, Gold, and SmallMedals to character
		if expGained > 0 {
			_, _ = progression.ApplyExperience(&char, expGained)
		}
		_ = char.AddMoney(goldGained)
		_ = char.AddSmallMedals(medalsGained)
	}

	historyID := id.New()

	history := BossChallengeHistory{
		ID:                historyID,
		CharacterID:       char.ID,
		BossID:            boss.ID,
		Tier:              boss.Tier,
		Outcome:           outcome,
		Turns:             battleResult.Turns,
		RewardExp:         expGained,
		RewardGold:        goldGained,
		RewardSmallMedals: medalsGained,
		RewardItemID:      rewardItemID,
		IsFirstClear:      isFirstClear,
		CreatedAt:         now,
	}

	if err := s.repo.RecordChallenge(ctx, history, rec, char, rewardItemInstance); err != nil {
		return ChallengeResult{}, fmt.Errorf("record boss challenge: %w", err)
	}

	if outcome == corebattle.OutcomeWin && battleResult.WinnerID == char.ID {
		if s.victoryBanquetHook != nil {
			_ = s.victoryBanquetHook(ctx, boss.ID, boss.Name, char.ID, char.Name, boss.Tier)
		}
		if s.victoryHook != nil {
			_ = s.victoryHook(ctx, char.ID, boss.ID, boss.Tier)
		}
	}

	return ChallengeResult{
		BattleResult:      battleResult,
		Outcome:           outcome,
		ExperienceReward:  expGained,
		GoldReward:        goldGained,
		SmallMedalsReward: medalsGained,
		ItemRewardID:      rewardItemID,
		IsFirstClear:      isFirstClear,
		UpdatedRecord:     rec,
	}, nil
}
