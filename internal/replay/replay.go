package replay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

var (
	ErrReplayNotFound         = errors.New("battle replay not found")
	ErrInvalidReplay          = errors.New("invalid replay data")
	ErrInvalidCombatType      = errors.New("invalid combat type")
	ErrInvalidParticipantData = errors.New("invalid participant data")
)

const (
	CombatTypePvP       = "pvp"
	CombatTypeGvG       = "gvg"
	CombatTypeBoss      = "boss"
	CombatTypeDungeon   = "dungeon"
	CombatTypeAdventure = "adventure"
	CombatTypeChallenge = "challenge"
)

type ParticipantSnapshot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	MaxHP   int    `json:"max_hp"`
	Attack  int    `json:"attack"`
	Defense int    `json:"defense"`
	Agility int    `json:"agility,omitempty"`
	JobID   string `json:"job_id,omitempty"`
	Level   int    `json:"level,omitempty"`
}

type BattleReplay struct {
	ID                  string                `json:"id"`
	CombatType          string                `json:"combat_type"`
	InitiatorID         string                `json:"initiator_id"`
	InitiatorName       string                `json:"initiator_name"`
	OpponentID          string                `json:"opponent_id"`
	OpponentName        string                `json:"opponent_name"`
	Outcome             corebattle.Outcome    `json:"outcome"`
	WinnerID            string                `json:"winner_id,omitempty"`
	LoserID             string                `json:"loser_id,omitempty"`
	TotalTurns          int                   `json:"total_turns"`
	InitialParticipants []ParticipantSnapshot `json:"initial_participants"`
	TurnLogs            []corebattle.TurnLog  `json:"turn_logs"`
	CreatedAt           time.Time             `json:"created_at"`
}

type ReplayHeader struct {
	ID            string             `json:"id"`
	CombatType    string             `json:"combat_type"`
	InitiatorID   string             `json:"initiator_id"`
	InitiatorName string             `json:"initiator_name"`
	OpponentID    string             `json:"opponent_id"`
	OpponentName  string             `json:"opponent_name"`
	Outcome       corebattle.Outcome `json:"outcome"`
	WinnerID      string             `json:"winner_id,omitempty"`
	TotalTurns    int                `json:"total_turns"`
	CreatedAt     time.Time          `json:"created_at"`
}

type SaveReplayRequest struct {
	CombatType   string
	Initiator    ParticipantSnapshot
	Opponent     ParticipantSnapshot
	BattleResult corebattle.Result
}

type Repository interface {
	Save(ctx context.Context, replay BattleReplay) error
	FindByID(ctx context.Context, id string) (*BattleReplay, error)
	FindByCharacter(ctx context.Context, characterID string, combatType string, limit int) ([]ReplayHeader, error)
	FindRecent(ctx context.Context, combatType string, limit int) ([]ReplayHeader, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("replay repository is required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) RecordBattle(ctx context.Context, req SaveReplayRequest) (string, error) {
	if strings.TrimSpace(req.CombatType) == "" {
		return "", ErrInvalidCombatType
	}
	if req.Initiator.ID == "" || req.Opponent.ID == "" {
		return "", ErrInvalidParticipantData
	}

	replayID, err := generateID()
	if err != nil {
		return "", err
	}

	initName := req.Initiator.Name
	if initName == "" {
		initName = req.Initiator.ID
	}
	oppName := req.Opponent.Name
	if oppName == "" {
		oppName = req.Opponent.ID
	}

	now := time.Now().UTC()
	replay := BattleReplay{
		ID:            replayID,
		CombatType:    req.CombatType,
		InitiatorID:   req.Initiator.ID,
		InitiatorName: initName,
		OpponentID:    req.Opponent.ID,
		OpponentName:  oppName,
		Outcome:       req.BattleResult.Outcome,
		WinnerID:      req.BattleResult.WinnerID,
		LoserID:       req.BattleResult.LoserID,
		TotalTurns:    req.BattleResult.Turns,
		InitialParticipants: []ParticipantSnapshot{
			req.Initiator,
			req.Opponent,
		},
		TurnLogs:  req.BattleResult.Logs,
		CreatedAt: now,
	}

	if err := s.repo.Save(ctx, replay); err != nil {
		return "", err
	}

	return replayID, nil
}

func (s *Service) GetReplay(ctx context.Context, id string) (*BattleReplay, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrReplayNotFound
	}
	return s.repo.FindByID(ctx, id)
}

func (s *Service) GetCharacterHistory(ctx context.Context, characterID string, combatType string, limit int) ([]ReplayHeader, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, errors.New("character id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.FindByCharacter(ctx, characterID, combatType, limit)
}

func (s *Service) GetRecentReplays(ctx context.Context, combatType string, limit int) ([]ReplayHeader, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.FindRecent(ctx, combatType, limit)
}

func (s *Service) PruneOldReplays(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	return s.repo.DeleteOlderThan(ctx, cutoff)
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
