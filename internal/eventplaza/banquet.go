package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/id"
)

// CelebrationBanquet represents a victory feast in honor of slaying a legendary boss.
type CelebrationBanquet struct {
	ID                  string    `json:"id"`
	BossID              string    `json:"boss_id"`
	BossName            string    `json:"boss_name"`
	SlayerCharacterID   string    `json:"slayer_character_id"`
	SlayerCharacterName string    `json:"slayer_character_name"`
	Tier                int       `json:"tier"`
	ToastCount          int       `json:"toast_count"`
	CelebratedAt        time.Time `json:"celebrated_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// BanquetToastResult represents the outcome of raising a commemorative toast at a banquet.
type BanquetToastResult struct {
	BanquetID            string `json:"banquet_id"`
	CharacterID          string `json:"character_id"`
	GoldAwarded          int    `json:"gold_awarded"`
	CurrentCharacterGold int    `json:"current_character_gold"`
	ToastCount           int    `json:"toast_count"`
	Message              string `json:"message"`
}

func (s *Service) RecordVictoryBanquet(
	ctx context.Context,
	bossID, bossName, slayerID, slayerName string,
	tier int,
) (CelebrationBanquet, error) {
	if strings.TrimSpace(bossID) == "" || strings.TrimSpace(bossName) == "" ||
		strings.TrimSpace(slayerID) == "" || strings.TrimSpace(slayerName) == "" {
		return CelebrationBanquet{}, errors.New("invalid victory banquet parameters")
	}

	now := s.clock.Now()
	banquet := CelebrationBanquet{
		ID:                  id.New(),
		BossID:              strings.TrimSpace(bossID),
		BossName:            strings.TrimSpace(bossName),
		SlayerCharacterID:   strings.TrimSpace(slayerID),
		SlayerCharacterName: strings.TrimSpace(slayerName),
		Tier:                tier,
		ToastCount:          0,
		CelebratedAt:        now,
		ExpiresAt:           now.Add(DefaultBanquetDuration),
	}

	if err := s.repo.SaveBanquet(ctx, banquet); err != nil {
		return CelebrationBanquet{}, fmt.Errorf("failed to save celebration banquet: %w", err)
	}

	return banquet, nil
}

func (s *Service) ListActiveBanquets(ctx context.Context) ([]CelebrationBanquet, error) {
	now := s.clock.Now()
	return s.repo.ListActiveBanquets(ctx, now)
}

func (s *Service) ToastBanquet(ctx context.Context, banquetID string, characterID string) (BanquetToastResult, error) {
	if strings.TrimSpace(banquetID) == "" {
		return BanquetToastResult{}, ErrBanquetNotFound
	}
	if strings.TrimSpace(characterID) == "" {
		return BanquetToastResult{}, ErrCharacterNotFound
	}

	banquet, err := s.repo.FindBanquetByID(ctx, banquetID)
	if err != nil {
		return BanquetToastResult{}, ErrBanquetNotFound
	}

	now := s.clock.Now()
	if now.After(banquet.ExpiresAt) {
		return BanquetToastResult{}, ErrBanquetExpired
	}

	hasToasted, err := s.repo.HasToasted(ctx, banquetID, characterID)
	if err != nil {
		return BanquetToastResult{}, fmt.Errorf("failed to check toast status: %w", err)
	}
	if hasToasted {
		return BanquetToastResult{}, ErrAlreadyToasted
	}

	var result BanquetToastResult
	runInTx := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if err := s.repo.RecordToast(txCtx, banquetID, characterID, now); err != nil {
			return ErrAlreadyToasted
		}

		rewardGold := DefaultToastGoldReward * banquet.Tier
		if rewardGold <= 0 {
			rewardGold = DefaultToastGoldReward
		}
		_ = char.AddMoney(rewardGold)

		if err := s.characterRepo.Update(txCtx, char); err != nil {
			return fmt.Errorf("failed to update character money on toast: %w", err)
		}

		_ = s.recordPresence(txCtx, characterID, now)

		result = BanquetToastResult{
			BanquetID:            banquetID,
			CharacterID:          characterID,
			GoldAwarded:          rewardGold,
			CurrentCharacterGold: char.Money,
			ToastCount:           banquet.ToastCount + 1,
			Message:              fmt.Sprintf("英雄 %s の討伐偉業を称えて乾杯しました！祝宴の引き出物として %d G を受け取りました。", banquet.SlayerCharacterName, rewardGold),
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, runInTx); err != nil {
			return BanquetToastResult{}, err
		}
	} else {
		if err := runInTx(ctx); err != nil {
			return BanquetToastResult{}, err
		}
	}

	return result, nil
}
