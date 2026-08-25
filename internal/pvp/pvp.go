package pvp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
)

const (
	DefaultRating = 1000
	KFactor       = 32
	MinRating     = 0
)

var (
	ErrInvalidCharacterID  = errors.New("invalid character ID")
	ErrCannotChallengeSelf = errors.New("cannot challenge self in arena")
	ErrCharacterDefeated   = errors.New("attacker character is defeated or has 0 HP")
	ErrInvalidDependencies = errors.New("pvp dependencies cannot be nil")
)

type MatchOutcome string

const (
	OutcomeWin  MatchOutcome = "win"
	OutcomeLoss MatchOutcome = "loss"
	OutcomeDraw MatchOutcome = "draw"
)

type ArenaRating struct {
	CharacterID   string     `json:"character_id"`
	Rating        int        `json:"rating"`
	Wins          int        `json:"wins"`
	Losses        int        `json:"losses"`
	Draws         int        `json:"draws"`
	LastMatchedAt *time.Time `json:"last_matched_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type OpponentCandidate struct {
	CharacterID  string `json:"character_id"`
	Name         string `json:"name"`
	JobID        string `json:"job_id"`
	Level        int    `json:"level"`
	Rating       int    `json:"rating"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	RebirthCount int    `json:"rebirth_count"`
}

type MatchRecord struct {
	ID                   string       `json:"id"`
	AttackerID           string       `json:"attacker_id"`
	DefenderID           string       `json:"defender_id"`
	WinnerID             string       `json:"winner_id,omitempty"`
	LoserID              string       `json:"loser_id,omitempty"`
	Outcome              MatchOutcome `json:"outcome"`
	Turns                int          `json:"turns"`
	AttackerRatingBefore int          `json:"attacker_rating_before"`
	AttackerRatingAfter  int          `json:"attacker_rating_after"`
	DefenderRatingBefore int          `json:"defender_rating_before"`
	DefenderRatingAfter  int          `json:"defender_rating_after"`
	RewardGold           int          `json:"reward_gold"`
	RewardExp            int          `json:"reward_exp"`
	CreatedAt            time.Time    `json:"created_at"`
}

type ChallengeResult struct {
	Match                MatchRecord       `json:"match"`
	BattleResult         corebattle.Result `json:"battle_result"`
	AttackerLeveledUp    bool              `json:"attacker_leveled_up"`
	AttackerCurrentLevel int               `json:"attacker_current_level"`
}

// CalculateEloDelta calculates rating changes for attacker and defender using the standard Elo rating system.
func CalculateEloDelta(attackerRating, defenderRating int, outcome MatchOutcome) (int, int) {
	expectedAttacker := 1.0 / (1.0 + math.Pow(10.0, float64(defenderRating-attackerRating)/400.0))
	var scoreAttacker float64
	switch outcome {
	case OutcomeWin:
		scoreAttacker = 1.0
	case OutcomeLoss:
		scoreAttacker = 0.0
	case OutcomeDraw:
		scoreAttacker = 0.5
	}

	deltaAttacker := int(math.Round(float64(KFactor) * (scoreAttacker - expectedAttacker)))
	if outcome == OutcomeWin && deltaAttacker < 1 {
		deltaAttacker = 1
	} else if outcome == OutcomeLoss && deltaAttacker > -1 {
		deltaAttacker = -1
	}
	deltaDefender := -deltaAttacker
	return deltaAttacker, deltaDefender
}

// CalculateRewards calculates the EXP and Gold rewards awarded to the attacker.
func CalculateRewards(defender corecharacter.Character, outcome MatchOutcome) (int, int) {
	switch outcome {
	case OutcomeWin:
		exp := 50 + (defender.Level * 10)
		gold := 100 + (defender.Level * 20)
		return exp, gold
	case OutcomeLoss:
		return 10, 0
	case OutcomeDraw:
		return 25, 50
	default:
		return 0, 0
	}
}

type Repository interface {
	GetOrCreateRating(ctx context.Context, characterID string) (ArenaRating, error)
	RecordMatchAndUpdateRatings(ctx context.Context, match MatchRecord, attackerRating, defenderRating ArenaRating, attacker corecharacter.Character) error
	FindOpponents(ctx context.Context, characterID string, limit int) ([]OpponentCandidate, error)
	GetMatchHistory(ctx context.Context, characterID string, limit int) ([]MatchRecord, error)
	GetDefenseLogs(ctx context.Context, characterID string, limit int) ([]MatchRecord, error)
	GetLeaderboard(ctx context.Context, limit int) ([]OpponentCandidate, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Service struct {
	repo       Repository
	characters CharacterRepository
	battle     corebattle.Resolver
}

func NewService(repo Repository, characters CharacterRepository, battle corebattle.Resolver) (*Service, error) {
	if repo == nil || characters == nil || battle == nil {
		return nil, ErrInvalidDependencies
	}
	return &Service{
		repo:       repo,
		characters: characters,
		battle:     battle,
	}, nil
}

func (s *Service) GetRating(ctx context.Context, characterID string) (ArenaRating, error) {
	if characterID == "" {
		return ArenaRating{}, ErrInvalidCharacterID
	}
	return s.repo.GetOrCreateRating(ctx, characterID)
}

func (s *Service) FindOpponents(ctx context.Context, characterID string, limit int) ([]OpponentCandidate, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if limit <= 0 {
		limit = 5
	}
	return s.repo.FindOpponents(ctx, characterID, limit)
}

func (s *Service) GetMatchHistory(ctx context.Context, characterID string, limit int) ([]MatchRecord, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if limit <= 0 {
		limit = 10
	}
	return s.repo.GetMatchHistory(ctx, characterID, limit)
}

func (s *Service) GetDefenseLogs(ctx context.Context, characterID string, limit int) ([]MatchRecord, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if limit <= 0 {
		limit = 10
	}
	return s.repo.GetDefenseLogs(ctx, characterID, limit)
}

func (s *Service) GetLeaderboard(ctx context.Context, limit int) ([]OpponentCandidate, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.GetLeaderboard(ctx, limit)
}

func (s *Service) Challenge(ctx context.Context, attackerID, defenderID string) (ChallengeResult, error) {
	if attackerID == "" || defenderID == "" {
		return ChallengeResult{}, ErrInvalidCharacterID
	}
	if attackerID == defenderID {
		return ChallengeResult{}, ErrCannotChallengeSelf
	}

	attacker, err := s.characters.FindByID(ctx, attackerID)
	if err != nil {
		return ChallengeResult{}, err
	}
	if attacker.Stats.HP <= 0 {
		return ChallengeResult{}, ErrCharacterDefeated
	}

	defender, err := s.characters.FindByID(ctx, defenderID)
	if err != nil {
		return ChallengeResult{}, err
	}

	// 1. Fetch ratings
	attRating, err := s.repo.GetOrCreateRating(ctx, attackerID)
	if err != nil {
		return ChallengeResult{}, fmt.Errorf("get attacker rating: %w", err)
	}
	defRating, err := s.repo.GetOrCreateRating(ctx, defenderID)
	if err != nil {
		return ChallengeResult{}, fmt.Errorf("get defender rating: %w", err)
	}

	// 2. Prepare Battle Engine request
	req := corebattle.Request{
		Participants: []corebattle.Participant{
			corebattle.NewParticipantFromCharacter(attacker),
			corebattle.NewParticipantFromCharacter(defender),
		},
	}

	battleResult, err := s.battle.Resolve(req)
	if err != nil {
		return ChallengeResult{}, fmt.Errorf("resolve battle: %w", err)
	}

	// 3. Determine match outcome from attacker perspective
	var outcome MatchOutcome
	var winnerID, loserID string

	if battleResult.Outcome == corebattle.OutcomeWin {
		if battleResult.WinnerID == attacker.ID {
			outcome = OutcomeWin
			winnerID = attacker.ID
			loserID = defender.ID
		} else {
			outcome = OutcomeLoss
			winnerID = defender.ID
			loserID = attacker.ID
		}
	} else {
		outcome = OutcomeDraw
	}

	// 4. Calculate Elo ratings
	deltaAttacker, deltaDefender := CalculateEloDelta(attRating.Rating, defRating.Rating, outcome)
	newAttackerRating := int(math.Max(float64(MinRating), float64(attRating.Rating+deltaAttacker)))
	newDefenderRating := int(math.Max(float64(MinRating), float64(defRating.Rating+deltaDefender)))

	// 5. Calculate Rewards and apply to attacker
	rewardExp, rewardGold := CalculateRewards(defender, outcome)
	attackerLeveledUp := false
	if rewardExp > 0 {
		levelsGained, err := progression.ApplyExperience(&attacker, rewardExp)
		if err != nil {
			return ChallengeResult{}, fmt.Errorf("apply exp: %w", err)
		}
		attackerLeveledUp = levelsGained > 0
	}
	attacker.Money += rewardGold

	// 6. Update rating objects
	now := time.Now().UTC()
	updatedAttRating := attRating
	updatedAttRating.Rating = newAttackerRating
	updatedAttRating.LastMatchedAt = &now
	updatedDefRating := defRating
	updatedDefRating.Rating = newDefenderRating
	updatedDefRating.LastMatchedAt = &now

	switch outcome {
	case OutcomeWin:
		updatedAttRating.Wins++
		updatedDefRating.Losses++
	case OutcomeLoss:
		updatedAttRating.Losses++
		updatedDefRating.Wins++
	case OutcomeDraw:
		updatedAttRating.Draws++
		updatedDefRating.Draws++
	}

	matchID, err := newMatchID()
	if err != nil {
		return ChallengeResult{}, err
	}

	match := MatchRecord{
		ID:                   matchID,
		AttackerID:           attacker.ID,
		DefenderID:           defender.ID,
		WinnerID:             winnerID,
		LoserID:              loserID,
		Outcome:              outcome,
		Turns:                battleResult.Turns,
		AttackerRatingBefore: attRating.Rating,
		AttackerRatingAfter:  newAttackerRating,
		DefenderRatingBefore: defRating.Rating,
		DefenderRatingAfter:  newDefenderRating,
		RewardGold:           rewardGold,
		RewardExp:            rewardExp,
		CreatedAt:            now,
	}

	// 7. Persist match record and rating updates atomically
	if err := s.repo.RecordMatchAndUpdateRatings(ctx, match, updatedAttRating, updatedDefRating, attacker); err != nil {
		return ChallengeResult{}, fmt.Errorf("record match: %w", err)
	}

	return ChallengeResult{
		Match:                match,
		BattleResult:         battleResult,
		AttackerLeveledUp:    attackerLeveledUp,
		AttackerCurrentLevel: attacker.Level,
	}, nil
}

func newMatchID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
