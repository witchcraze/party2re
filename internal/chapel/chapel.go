package chapel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type BlessingType string

const (
	BlessingNone    BlessingType = "NONE"
	BlessingGold    BlessingType = "GOLD"    // お金がほしい (+50% Gold chance)
	BlessingExp     BlessingType = "EXP"     // 強くなりたい (+50% EXP chance)
	BlessingMonster BlessingType = "MONSTER" // モンスターと仲良くしたい (モンスターが仲間になりやすくなる)
	BlessingDrop    BlessingType = "DROP"    // 宝箱がほしい (+10% Drop rate boost)
	BlessingCasino  BlessingType = "CASINO"  // コインがほしい (Casino bonus luck)
)

const (
	LocationName     = "礼拝堂"
	NPCName          = "@シスター"
	BackgroundImage  = "bgimg/chapel.gif"
	MsgAlreadyPrayed = "祈りに大事なのは、数でなく気持ちなのです"
)

var ValidBlessings = map[BlessingType]bool{
	BlessingNone:    true,
	BlessingGold:    true,
	BlessingExp:     true,
	BlessingMonster: true,
	BlessingDrop:    true,
	BlessingCasino:  true,
}

var (
	ErrInvalidCharacterID = errors.New("character ID cannot be empty")
	ErrInvalidBlessing    = errors.New("invalid blessing type")
	ErrAlreadyPrayed      = errors.New("character already has an active blessing: " + MsgAlreadyPrayed)
)

type BlessingInfo struct {
	Number      int          `json:"number"`
	Type        BlessingType `json:"type"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
}

var AvailableBlessings = []BlessingInfo{
	{Number: 1, Type: BlessingGold, Name: "お金がほしい", Description: "モンスターを倒したとき、ゴールドが増えるかも？"},
	{Number: 2, Type: BlessingExp, Name: "強くなりたい", Description: "モンスターを倒したとき、経験値が増えるかも？"},
	{Number: 3, Type: BlessingMonster, Name: "モンスターと仲良くしたい", Description: "モンスターが仲間になりやすくなるかも？"},
	{Number: 4, Type: BlessingDrop, Name: "宝箱がほしい", Description: "宝箱が増えるかも？"},
	{Number: 5, Type: BlessingCasino, Name: "コインがほしい", Description: "コインが増えるかも？"},
}

var SisterDialogues = []string{
	"%sに神のご加護があらんことを",
	"待つだけでは祈りは叶いません…",
	"%sが力を尽くすとき、祈りは叶うことでしょう",
}

// ParseBlessingType parses canonical keys, Japanese names, or numbers into a BlessingType.
func ParseBlessingType(s string) (BlessingType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "0":
		return BlessingNone, nil
	case "gold", "1", "お金がほしい", "gold_wish":
		return BlessingGold, nil
	case "exp", "2", "強くなりたい", "exp_wish":
		return BlessingExp, nil
	case "monster", "3", "モンスターと仲良くしたい", "monster_wish":
		return BlessingMonster, nil
	case "drop", "4", "宝箱がほしい", "treasure_wish":
		return BlessingDrop, nil
	case "casino", "5", "コインがほしい", "casino_wish":
		return BlessingCasino, nil
	default:
		return "", ErrInvalidBlessing
	}
}

type CharacterBlessing struct {
	CharacterID    string       `json:"character_id"`
	ActiveBlessing BlessingType `json:"active_blessing"`
	PrayedAt       time.Time    `json:"prayed_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

type ChapelStatus struct {
	LocationName       string         `json:"location_name"`
	NPCName            string         `json:"npc_name"`
	BackgroundImage    string         `json:"background_image"`
	HasActiveBlessing  bool           `json:"has_active_blessing"`
	ActiveBlessing     BlessingType   `json:"active_blessing"`
	ActiveBlessingName string         `json:"active_blessing_name,omitempty"`
	PrayedAt           *time.Time     `json:"prayed_at,omitempty"`
	AvailableBlessings []BlessingInfo `json:"available_blessings"`
	Dialogues          []string       `json:"dialogues"`
}

type RewardModifiers struct {
	ExpMultiplier           float64 `json:"exp_multiplier"`
	GoldMultiplier          float64 `json:"gold_multiplier"`
	DropBonusRate           float64 `json:"drop_bonus_rate"`
	MonsterRecruitBonusRate float64 `json:"monster_recruit_bonus_rate"`
}

// ComputeRewardModifiers returns calculated bonus multipliers for a given blessing type.
func ComputeRewardModifiers(blessing BlessingType, roll float64) RewardModifiers {
	mods := RewardModifiers{
		ExpMultiplier:           1.0,
		GoldMultiplier:          1.0,
		DropBonusRate:           0.0,
		MonsterRecruitBonusRate: 0.0,
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
	case BlessingMonster:
		// Monster recruit rate +50%
		mods.MonsterRecruitBonusRate = 0.50
	case BlessingDrop:
		mods.DropBonusRate = 0.10
	}

	return mods
}

type Repository interface {
	GetBlessing(ctx context.Context, characterID string) (CharacterBlessing, error)
	SelectBlessing(ctx context.Context, characterID string, blessing BlessingType) (CharacterBlessing, error)
	ClearBlessing(ctx context.Context, characterID string) error
	ClearAllBlessings(ctx context.Context) error
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

func (s *Service) GetStatus(ctx context.Context, characterID, characterName string) (ChapelStatus, error) {
	if characterID == "" {
		return ChapelStatus{}, ErrInvalidCharacterID
	}
	b, err := s.repo.GetBlessing(ctx, characterID)
	if err != nil {
		return ChapelStatus{}, err
	}

	hasActive := b.ActiveBlessing != BlessingNone && b.ActiveBlessing != ""
	var blessingName string
	for _, info := range AvailableBlessings {
		if info.Type == b.ActiveBlessing {
			blessingName = info.Name
			break
		}
	}

	formattedDialogues := make([]string, len(SisterDialogues))
	name := characterName
	if name == "" {
		name = "冒険者"
	}
	for i, d := range SisterDialogues {
		formattedDialogues[i] = fmt.Sprintf(d, name)
	}

	var prayedAt *time.Time
	if !b.PrayedAt.IsZero() {
		prayedAt = &b.PrayedAt
	}

	return ChapelStatus{
		LocationName:       LocationName,
		NPCName:            NPCName,
		BackgroundImage:    BackgroundImage,
		HasActiveBlessing:  hasActive,
		ActiveBlessing:     b.ActiveBlessing,
		ActiveBlessingName: blessingName,
		PrayedAt:           prayedAt,
		AvailableBlessings: AvailableBlessings,
		Dialogues:          formattedDialogues,
	}, nil
}

func (s *Service) SelectBlessing(ctx context.Context, characterID string, blessing BlessingType) (CharacterBlessing, error) {
	if characterID == "" {
		return CharacterBlessing{}, ErrInvalidCharacterID
	}
	if !ValidBlessings[blessing] || blessing == BlessingNone {
		return CharacterBlessing{}, ErrInvalidBlessing
	}

	// Enforce single active wish constraint
	current, err := s.repo.GetBlessing(ctx, characterID)
	if err != nil {
		return CharacterBlessing{}, err
	}
	if current.ActiveBlessing != BlessingNone && current.ActiveBlessing != "" {
		return CharacterBlessing{}, ErrAlreadyPrayed
	}

	return s.repo.SelectBlessing(ctx, characterID, blessing)
}

func (s *Service) Pray(ctx context.Context, characterID string, blessing BlessingType) (CharacterBlessing, error) {
	return s.SelectBlessing(ctx, characterID, blessing)
}

func (s *Service) ClearBlessing(ctx context.Context, characterID string) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	return s.repo.ClearBlessing(ctx, characterID)
}

func (s *Service) ClearAllBlessings(ctx context.Context) error {
	return s.repo.ClearAllBlessings(ctx)
}
