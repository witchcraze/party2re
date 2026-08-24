package chapel

import (
	"context"
	"errors"
	"time"
)

type BlessingType string

const (
	BlessingNone   BlessingType = "NONE"
	BlessingGold   BlessingType = "GOLD"   // お金がほしい (+50% Gold chance)
	BlessingExp    BlessingType = "EXP"    // 強くなりたい (+50% EXP chance)
	BlessingDrop   BlessingType = "DROP"   // 宝箱がほしい (+10% Drop rate boost)
	BlessingCasino BlessingType = "CASINO" // コインがほしい (Casino bonus luck)
)

var ValidBlessings = map[BlessingType]bool{
	BlessingNone:   true,
	BlessingGold:   true,
	BlessingExp:    true,
	BlessingDrop:   true,
	BlessingCasino: true,
}

var (
	ErrInvalidCharacterID = errors.New("character ID cannot be empty")
	ErrInvalidBlessing    = errors.New("invalid blessing type")
	ErrInvalidDonation    = errors.New("donation amount must be positive")
	ErrInsufficientGold   = errors.New("insufficient character gold for donation")
)

type CharacterBlessing struct {
	CharacterID       string       `json:"character_id"`
	ActiveBlessing    BlessingType `json:"active_blessing"`
	DonationGoldTotal int64        `json:"donation_gold_total"`
	PrayedAt          time.Time    `json:"prayed_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

type RewardModifiers struct {
	ExpMultiplier  float64 `json:"exp_multiplier"`
	GoldMultiplier float64 `json:"gold_multiplier"`
	DropBonusRate  float64 `json:"drop_bonus_rate"`
}

// ComputeRewardModifiers returns calculated bonus multipliers for a given blessing type.
func ComputeRewardModifiers(blessing BlessingType, roll float64) RewardModifiers {
	mods := RewardModifiers{
		ExpMultiplier:  1.0,
		GoldMultiplier: 1.0,
		DropBonusRate:  0.0,
	}

	switch blessing {
	case BlessingExp:
		// 25% chance of 1.5x EXP
		if roll < 0.25 {
			mods.ExpMultiplier = 1.5
		}
	case BlessingGold:
		// 25% chance of 1.5x Gold
		if roll < 0.25 {
			mods.GoldMultiplier = 1.5
		}
	case BlessingDrop:
		mods.DropBonusRate = 0.10
	}

	return mods
}

type Repository interface {
	GetBlessing(ctx context.Context, characterID string) (CharacterBlessing, error)
	SelectBlessing(ctx context.Context, characterID string, blessing BlessingType) (CharacterBlessing, error)
	Donate(ctx context.Context, characterID string, goldAmount int) (CharacterBlessing, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) GetBlessing(ctx context.Context, characterID string) (CharacterBlessing, error) {
	if characterID == "" {
		return CharacterBlessing{}, ErrInvalidCharacterID
	}
	return s.repo.GetBlessing(ctx, characterID)
}

func (s *Service) SelectBlessing(ctx context.Context, characterID string, blessing BlessingType) (CharacterBlessing, error) {
	if characterID == "" {
		return CharacterBlessing{}, ErrInvalidCharacterID
	}
	if !ValidBlessings[blessing] {
		return CharacterBlessing{}, ErrInvalidBlessing
	}
	return s.repo.SelectBlessing(ctx, characterID, blessing)
}

func (s *Service) Donate(ctx context.Context, characterID string, goldAmount int) (CharacterBlessing, error) {
	if characterID == "" {
		return CharacterBlessing{}, ErrInvalidCharacterID
	}
	if goldAmount <= 0 {
		return CharacterBlessing{}, ErrInvalidDonation
	}
	return s.repo.Donate(ctx, characterID, goldAmount)
}
