package challenge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrSessionNotFound     = errors.New("challenge session not found")
	ErrSessionNotActive    = errors.New("challenge session is not active")
	ErrActiveSessionExists = errors.New("active challenge session already exists")
	ErrTierNotFound        = errors.New("challenge tier not found")
	ErrLevelTooLow         = errors.New("character level is too low for this tier")
	ErrCharacterNotFound   = errors.New("character not found")
)

type SessionStatus string

const (
	StatusActive   SessionStatus = "active"
	StatusClaimed  SessionStatus = "claimed"
	StatusDefeated SessionStatus = "defeated"
)

type ChallengeMonster struct {
	Name        string `json:"name"`
	BaseHP      int    `json:"base_hp"`
	BaseAttack  int    `json:"base_attack"`
	BaseDefense int    `json:"base_defense"`
	BaseExp     int    `json:"base_exp"`
	BaseGold    int    `json:"base_gold"`
}

type ChallengeTier struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	MinLevel          int              `json:"min_level"`
	BaseMonster       ChallengeMonster `json:"base_monster"`
	ScaleFactor       float64          `json:"scale_factor"`
	MilestoneInterval int              `json:"milestone_interval"`
	MilestoneItemPool []string         `json:"milestone_item_pool"`
}

type ChallengeSession struct {
	ID                 string        `json:"id"`
	CharacterID        string        `json:"character_id"`
	TierID             string        `json:"tier_id"`
	CurrentRound       int           `json:"current_round"`
	CharacterCurrentHP int           `json:"character_current_hp"`
	AccumulatedExp     int           `json:"accumulated_exp"`
	AccumulatedGold    int           `json:"accumulated_gold"`
	AccumulatedItems   []string      `json:"accumulated_items"`
	Status             SessionStatus `json:"status"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

type CharacterChallengeRecord struct {
	CharacterID    string    `json:"character_id"`
	TierID         string    `json:"tier_id"`
	HighestRound   int       `json:"highest_round"`
	TotalAttempts  int       `json:"total_attempts"`
	TotalVictories int       `json:"total_victories"`
	BestClearedAt  time.Time `json:"best_cleared_at"`
}

type LeaderboardEntry struct {
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	Level         int       `json:"level"`
	JobID         string    `json:"job_id"`
	HighestRound  int       `json:"highest_round"`
	BestClearedAt time.Time `json:"best_cleared_at"`
}

type RoundResult struct {
	Round              int               `json:"round"`
	MonsterName        string            `json:"monster_name"`
	BattleResult       corebattle.Result `json:"battle_result"`
	Won                bool              `json:"won"`
	RecoveredHP        int               `json:"recovered_hp"`
	CharacterCurrentHP int               `json:"character_current_hp"`
	RoundExp           int               `json:"round_exp"`
	RoundGold          int               `json:"round_gold"`
	AwardedItem        string            `json:"awarded_item,omitempty"`
	SessionEnded       bool              `json:"session_ended"`
	SessionStatus      SessionStatus     `json:"session_status"`
}

type CashoutResult struct {
	RoundsCleared  int      `json:"rounds_cleared"`
	AwardedExp     int      `json:"awarded_exp"`
	AwardedGold    int      `json:"awarded_gold"`
	AwardedItems   []string `json:"awarded_items"`
	NewRecordRound int      `json:"new_record_round"`
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Repository interface {
	SaveSession(ctx context.Context, session ChallengeSession) error
	FindSessionByID(ctx context.Context, id string) (*ChallengeSession, error)
	FindActiveSessionByCharacter(ctx context.Context, characterID string) (*ChallengeSession, error)
	UpdateSession(ctx context.Context, session ChallengeSession) error
	SaveRecord(ctx context.Context, record CharacterChallengeRecord) error
	FindRecord(ctx context.Context, characterID string, tierID string) (*CharacterChallengeRecord, error)
	GetLeaderboard(ctx context.Context, tierID string, limit int) ([]LeaderboardEntry, error)
	FinalizeSession(ctx context.Context, session ChallengeSession, expReward int, goldReward int, items []string, newStreak int) error
}

type Service struct {
	repo         Repository
	charRepo     CharacterRepository
	battleEngine corebattle.Resolver
	tiers        map[string]ChallengeTier
}

func NewService(repo Repository, charRepo CharacterRepository, battleEngine corebattle.Resolver) (*Service, error) {
	if repo == nil {
		return nil, errors.New("challenge repository is required")
	}
	if charRepo == nil {
		return nil, errors.New("character repository is required")
	}
	if battleEngine == nil {
		battleEngine = corebattle.Engine{}
	}

	tiers := defaultTiers()
	return &Service{
		repo:         repo,
		charRepo:     charRepo,
		battleEngine: battleEngine,
		tiers:        tiers,
	}, nil
}

func defaultTiers() map[string]ChallengeTier {
	return map[string]ChallengeTier{
		"novice": {
			ID:          "novice",
			Name:        "初級チャレンジ (Novice Trial)",
			Description: "駆け出し冒険者のためのサバイバル連戦。スライムやゴブリンの群れに立ち向かう。",
			MinLevel:    5,
			BaseMonster: ChallengeMonster{
				Name:        "修行スライム",
				BaseHP:      120,
				BaseAttack:  25,
				BaseDefense: 10,
				BaseExp:     15,
				BaseGold:    8,
			},
			ScaleFactor:       0.08,
			MilestoneInterval: 5,
			MilestoneItemPool: []string{"potion_minor", "herb_medicinal"},
		},
		"intermediate": {
			ID:          "intermediate",
			Name:        "中級チャレンジ (Veteran Trial)",
			Description: "熟練冒険者向けのエンドレス試練。オーガやスケルトンナイトが次々と襲いかかる。",
			MinLevel:    20,
			BaseMonster: ChallengeMonster{
				Name:        "闘技場オーガ",
				BaseHP:      350,
				BaseAttack:  65,
				BaseDefense: 35,
				BaseExp:     60,
				BaseGold:    35,
			},
			ScaleFactor:       0.10,
			MilestoneInterval: 5,
			MilestoneItemPool: []string{"potion_standard", "elixir_minor", "iron_ore"},
		},
		"master": {
			ID:          "master",
			Name:        "上級チャレンジ (Master Trial)",
			Description: "歴戦の勇士に課される極限の闘技。ドラゴンやグレーターデーモンとの連戦。",
			MinLevel:    40,
			BaseMonster: ChallengeMonster{
				Name:        "修羅の魔獣",
				BaseHP:      900,
				BaseAttack:  140,
				BaseDefense: 80,
				BaseExp:     220,
				BaseGold:    120,
			},
			ScaleFactor:       0.12,
			MilestoneInterval: 5,
			MilestoneItemPool: []string{"potion_high", "elixir_standard", "mithril_ore"},
		},
		"abyss": {
			ID:          "abyss",
			Name:        "奈落チャレンジ (Abyss Trial)",
			Description: "底知れぬ深淵の試練。神域の怪異が無限の猛威を振るう。",
			MinLevel:    60,
			BaseMonster: ChallengeMonster{
				Name:        "深淵の幻影",
				BaseHP:      2000,
				BaseAttack:  280,
				BaseDefense: 160,
				BaseExp:     600,
				BaseGold:    350,
			},
			ScaleFactor:       0.15,
			MilestoneInterval: 5,
			MilestoneItemPool: []string{"elixir_high", "orichalcum_ore", "philosophers_stone"},
		},
	}
}

func (s *Service) ListTiers() []ChallengeTier {
	order := []string{"novice", "intermediate", "master", "abyss"}
	var list []ChallengeTier
	for _, id := range order {
		if t, ok := s.tiers[id]; ok {
			list = append(list, t)
		}
	}
	return list
}

func (s *Service) GetTier(tierID string) (*ChallengeTier, error) {
	t, ok := s.tiers[tierID]
	if !ok {
		return nil, ErrTierNotFound
	}
	return &t, nil
}

func (s *Service) StartSession(ctx context.Context, characterID string, tierID string) (*ChallengeSession, error) {
	tier, err := s.GetTier(tierID)
	if err != nil {
		return nil, err
	}

	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	level := char.Level
	if level <= 0 {
		level = 1
	}
	if level < tier.MinLevel {
		return nil, ErrLevelTooLow
	}

	existing, err := s.repo.FindActiveSessionByCharacter(ctx, characterID)
	if err == nil && existing != nil && existing.Status == StatusActive {
		return nil, ErrActiveSessionExists
	}

	sessionID, err := generateID()
	if err != nil {
		return nil, err
	}

	maxHP := char.Stats.MaxHP
	if maxHP <= 0 {
		maxHP = char.Stats.HP
	}
	if maxHP <= 0 {
		maxHP = 100
	}

	now := time.Now().UTC()
	session := ChallengeSession{
		ID:                 sessionID,
		CharacterID:        characterID,
		TierID:             tierID,
		CurrentRound:       1,
		CharacterCurrentHP: maxHP,
		AccumulatedExp:     0,
		AccumulatedGold:    0,
		AccumulatedItems:   []string{},
		Status:             StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.repo.SaveSession(ctx, session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (s *Service) ExecuteRound(ctx context.Context, sessionID string) (*RoundResult, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != StatusActive {
		return nil, ErrSessionNotActive
	}

	tier, err := s.GetTier(session.TierID)
	if err != nil {
		return nil, err
	}

	char, err := s.charRepo.FindByID(ctx, session.CharacterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	maxHP := char.Stats.MaxHP
	if maxHP <= 0 {
		maxHP = char.Stats.HP
	}
	if maxHP <= 0 {
		maxHP = 100
	}

	// Scale monster for round
	round := session.CurrentRound
	scale := 1.0 + float64(round-1)*tier.ScaleFactor
	mHP := int(math.Round(float64(tier.BaseMonster.BaseHP) * scale))
	mAtk := int(math.Round(float64(tier.BaseMonster.BaseAttack) * scale))
	mDef := int(math.Round(float64(tier.BaseMonster.BaseDefense) * scale))
	mExp := int(math.Round(float64(tier.BaseMonster.BaseExp) * scale))
	mGold := int(math.Round(float64(tier.BaseMonster.BaseGold) * scale))
	mName := fmt.Sprintf("%s (Wave %d)", tier.BaseMonster.Name, round)

	// Resolve Battle
	charParticipant := corebattle.Participant{
		ID:      char.ID,
		HP:      session.CharacterCurrentHP,
		Attack:  char.Stats.Attack,
		Defense: char.Stats.Defense,
	}
	monsterParticipant := corebattle.Participant{
		ID:      mName,
		HP:      mHP,
		Attack:  mAtk,
		Defense: mDef,
	}

	battleReq := corebattle.Request{
		Participants: []corebattle.Participant{charParticipant, monsterParticipant},
	}
	battleRes, err := s.battleEngine.Resolve(battleReq)
	if err != nil {
		return nil, err
	}

	won := battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == char.ID

	if won {
		// Calculate surviving HP
		survivingHP := 1
		if len(battleRes.Logs) > 0 {
			lastLog := battleRes.Logs[len(battleRes.Logs)-1]
			if hp, ok := lastLog.RemainingHP[char.ID]; ok && hp > 0 {
				survivingHP = hp
			}
		}

		// 20% MaxHP Recovery between rounds
		recovery := int(float64(maxHP) * 0.20)
		survivingHP += recovery
		if survivingHP > maxHP {
			survivingHP = maxHP
		}

		session.CharacterCurrentHP = survivingHP
		session.AccumulatedExp += mExp
		session.AccumulatedGold += mGold

		// Milestone item check
		var awardedItem string
		if tier.MilestoneInterval > 0 && round%tier.MilestoneInterval == 0 && len(tier.MilestoneItemPool) > 0 {
			awardedItem = tier.MilestoneItemPool[(round/tier.MilestoneInterval-1)%len(tier.MilestoneItemPool)]
			session.AccumulatedItems = append(session.AccumulatedItems, awardedItem)
		}

		session.CurrentRound++
		session.UpdatedAt = time.Now().UTC()

		if err := s.repo.UpdateSession(ctx, *session); err != nil {
			return nil, err
		}

		return &RoundResult{
			Round:              round,
			MonsterName:        mName,
			BattleResult:       battleRes,
			Won:                true,
			RecoveredHP:        recovery,
			CharacterCurrentHP: survivingHP,
			RoundExp:           mExp,
			RoundGold:          mGold,
			AwardedItem:        awardedItem,
			SessionEnded:       false,
			SessionStatus:      StatusActive,
		}, nil
	}

	// Defeat: session terminates
	session.Status = StatusDefeated
	session.CharacterCurrentHP = 0
	session.UpdatedAt = time.Now().UTC()

	// On defeat, half exp/gold awarded, items forfeited
	awardedExp := session.AccumulatedExp / 2
	awardedGold := session.AccumulatedGold / 2
	clearedRounds := round - 1

	if err := s.repo.FinalizeSession(ctx, *session, awardedExp, awardedGold, nil, clearedRounds); err != nil {
		return nil, err
	}

	return &RoundResult{
		Round:              round,
		MonsterName:        mName,
		BattleResult:       battleRes,
		Won:                false,
		RecoveredHP:        0,
		CharacterCurrentHP: 0,
		RoundExp:           0,
		RoundGold:          0,
		SessionEnded:       true,
		SessionStatus:      StatusDefeated,
	}, nil
}

func (s *Service) Cashout(ctx context.Context, sessionID string) (*CashoutResult, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != StatusActive {
		return nil, ErrSessionNotActive
	}

	clearedRounds := session.CurrentRound - 1
	session.Status = StatusClaimed
	session.UpdatedAt = time.Now().UTC()

	exp := session.AccumulatedExp
	gold := session.AccumulatedGold
	items := session.AccumulatedItems

	if err := s.repo.FinalizeSession(ctx, *session, exp, gold, items, clearedRounds); err != nil {
		return nil, err
	}

	return &CashoutResult{
		RoundsCleared:  clearedRounds,
		AwardedExp:     exp,
		AwardedGold:    gold,
		AwardedItems:   items,
		NewRecordRound: clearedRounds,
	}, nil
}

func (s *Service) GetActiveSession(ctx context.Context, characterID string) (*ChallengeSession, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, errors.New("character id is required")
	}
	return s.repo.FindActiveSessionByCharacter(ctx, characterID)
}

func (s *Service) GetRecord(ctx context.Context, characterID string, tierID string) (*CharacterChallengeRecord, error) {
	return s.repo.FindRecord(ctx, characterID, tierID)
}

func (s *Service) GetLeaderboard(ctx context.Context, tierID string, limit int) ([]LeaderboardEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetLeaderboard(ctx, tierID, limit)
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func EncodeJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func DecodeJSON[T any](data string) (T, error) {
	var target T
	err := json.Unmarshal([]byte(data), &target)
	return target, err
}
