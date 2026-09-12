package pvp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

// CharacterRepository defines persistence operations on Character.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// BattleEngine defines the battle resolution contract.
type BattleEngine interface {
	ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

// VictoryHook is called when a character or team wins a Colosseum match.
type VictoryHook func(ctx context.Context, winnerID string, loserID string) error

// Option configures Service dependencies.
type Option func(*Service)

func WithVictoryHook(hook VictoryHook) Option {
	return func(s *Service) {
		s.victoryHook = hook
	}
}

type Service struct {
	repo         RoomRepository
	characters   CharacterRepository
	battleEngine BattleEngine
	victoryHook  VictoryHook
}

func (s *Service) SetVictoryHook(hook VictoryHook) {
	s.victoryHook = hook
}

func NewService(
	repo RoomRepository,
	characters CharacterRepository,
	battleEngine BattleEngine,
	opts ...Option,
) (*Service, error) {
	if repo == nil || characters == nil || battleEngine == nil {
		return nil, ErrInvalidDependencies
	}

	svc := &Service{
		repo:         repo,
		characters:   characters,
		battleEngine: battleEngine,
	}

	for _, opt := range opts {
		opt(svc)
	}

	return svc, nil
}

func hashPassword(pw string) string {
	if pw == "" {
		return ""
	}
	h := sha256.Sum256([]byte(pw))
	return hex.EncodeToString(h[:])
}

func checkNeedJoin(char corecharacter.Character, needJoin string) error {
	if needJoin == "" {
		return nil
	}
	parts := strings.Split(needJoin, "_")
	if len(parts) != 3 {
		return nil
	}
	key, valStr, op := parts[0], parts[1], parts[2]
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return nil
	}

	switch key {
	case "hp":
		if op == "o" && char.Stats.MaxHP < val {
			return ErrNeedJoinNotMet
		}
		if op == "u" && char.Stats.MaxHP >= val {
			return ErrNeedJoinNotMet
		}
	case "joblv":
		if op == "o" && char.JobLevel < val {
			return ErrNeedJoinNotMet
		}
		if op == "u" && char.JobLevel >= val {
			return ErrNeedJoinNotMet
		}
	}
	return nil
}

// CreateRoom creates a new colosseum room (quest.cgi:type=4).
func (s *Service) CreateRoom(ctx context.Context, characterID string, req CreateRoomRequest) (RoomDetail, error) {
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return RoomDetail{}, err
	}
	if char.Stats.HP <= 0 {
		return RoomDetail{}, ErrCharacterUnconscious
	}
	if char.Tired >= 100 {
		return RoomDetail{}, ErrCharacterExhausted
	}

	nameLen := utf8.RuneCountInString(strings.TrimSpace(req.Name))
	if nameLen < 1 || nameLen > 50 {
		return RoomDetail{}, ErrInvalidRoomName
	}
	if req.Bet < MinBet {
		return RoomDetail{}, ErrInvalidBet
	}
	if req.MaxMembers == 0 {
		req.MaxMembers = MaxMembers
	}
	if req.MaxMembers < MinMembers || req.MaxMembers > MaxMembers {
		return RoomDetail{}, ErrInvalidMaxMembers
	}
	if req.TargetWins == 0 {
		req.TargetWins = MinTargetWins
	}
	if req.TargetWins < MinTargetWins || req.TargetWins > MaxTargetWins {
		return RoomDetail{}, ErrInvalidTargetWins
	}
	if req.Speed == 0 {
		req.Speed = DefaultSpeed
	}
	if err := checkNeedJoin(char, req.NeedJoin); err != nil {
		return RoomDetail{}, err
	}

	// Verify existing room
	if existingRoomID, _ := s.repo.GetCharacterRoom(ctx, char.ID); existingRoomID != "" {
		if existing, err := s.repo.GetRoom(ctx, existingRoomID); err == nil && existing.Room.Status != StatusCompleted && existing.Room.Status != StatusDisbanded {
			return RoomDetail{}, ErrAlreadyInRoom
		}
	}

	// Deduct Bet from leader wallet
	if err := char.DeductMoney(req.Bet); err != nil {
		return RoomDetail{}, ErrInsufficientBetFunds
	}
	if err := s.characters.Update(ctx, char); err != nil {
		return RoomDetail{}, fmt.Errorf("deduct bet from leader: %w", err)
	}

	now := time.Now().UTC()
	roomID := id.New()
	room := ColosseumRoom{
		ID:                roomID,
		Name:              strings.TrimSpace(req.Name),
		LeaderCharacterID: char.ID,
		LeaderName:        char.Name,
		Speed:             req.Speed,
		Stage:             req.Stage,
		MaxMembers:        req.MaxMembers,
		TargetWins:        req.TargetWins,
		Bet:               req.Bet,
		PrizePool:         req.Bet,
		PasswordHash:      hashPassword(req.Password),
		NeedJoin:          req.NeedJoin,
		Status:            StatusRecruiting,
		Round:             0,
		TeamScores:        make(map[string]int),
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	leaderMember := RoomMember{
		RoomID:        roomID,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		JobID:         char.JobID,
		Level:         char.Level,
		HP:            char.Stats.HP,
		MaxHP:         char.Stats.MaxHP,
		TeamColor:     "",
		IsLeader:      true,
		ReadyState:    false,
		JoinedAt:      now,
	}

	members := []RoomMember{leaderMember}
	if err := s.repo.SaveRoom(ctx, room, members); err != nil {
		return RoomDetail{}, err
	}
	_ = s.repo.SetCharacterRoom(ctx, char.ID, roomID)

	return RoomDetail{Room: room, Members: members}, nil
}

// JoinRoom adds a character to an active colosseum room (quest.cgi:type=4).
func (s *Service) JoinRoom(ctx context.Context, characterID string, roomID string, password string) (RoomDetail, error) {
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return RoomDetail{}, err
	}
	if char.Stats.HP <= 0 {
		return RoomDetail{}, ErrCharacterUnconscious
	}
	if char.Tired >= 100 {
		return RoomDetail{}, ErrCharacterExhausted
	}

	detail, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		return RoomDetail{}, err
	}
	if detail.Room.Status != StatusRecruiting {
		return RoomDetail{}, ErrRoomNotRecruiting
	}
	if len(detail.Members) >= detail.Room.MaxMembers {
		return RoomDetail{}, ErrRoomFull
	}

	for _, m := range detail.Members {
		if m.CharacterID == char.ID {
			return detail, nil
		}
	}

	if existingRoomID, _ := s.repo.GetCharacterRoom(ctx, char.ID); existingRoomID != "" && existingRoomID != roomID {
		if existing, err := s.repo.GetRoom(ctx, existingRoomID); err == nil && existing.Room.Status != StatusCompleted && existing.Room.Status != StatusDisbanded {
			return RoomDetail{}, ErrAlreadyInRoom
		}
	}

	if detail.Room.PasswordHash != "" && hashPassword(password) != detail.Room.PasswordHash {
		return RoomDetail{}, ErrInvalidPassword
	}
	if err := checkNeedJoin(char, detail.Room.NeedJoin); err != nil {
		return RoomDetail{}, err
	}

	// Deduct Bet from participant wallet
	if err := char.DeductMoney(detail.Room.Bet); err != nil {
		return RoomDetail{}, ErrInsufficientBetFunds
	}
	if err := s.characters.Update(ctx, char); err != nil {
		return RoomDetail{}, fmt.Errorf("deduct bet from participant: %w", err)
	}

	now := time.Now().UTC()
	detail.Room.PrizePool += detail.Room.Bet
	detail.Room.UpdatedAt = now

	newMember := RoomMember{
		RoomID:        roomID,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		JobID:         char.JobID,
		Level:         char.Level,
		HP:            char.Stats.HP,
		MaxHP:         char.Stats.MaxHP,
		TeamColor:     "",
		IsLeader:      false,
		ReadyState:    false,
		JoinedAt:      now,
	}

	detail.Members = append(detail.Members, newMember)
	if err := s.repo.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		return RoomDetail{}, err
	}
	_ = s.repo.SetCharacterRoom(ctx, char.ID, roomID)

	return detail, nil
}

// SelectTeam assigns a participant to one of the 9 team colors (@ぱーてぃー).
func (s *Service) SelectTeam(ctx context.Context, characterID string, roomID string, teamColor string) (RoomDetail, error) {
	normColor := strings.ToUpper(strings.TrimSpace(teamColor))
	if !IsValidTeamColor(normColor) {
		return RoomDetail{}, ErrInvalidTeamColor
	}

	detail, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		return RoomDetail{}, err
	}
	if detail.Room.Round > 0 || detail.Room.Status != StatusRecruiting {
		return RoomDetail{}, ErrMatchAlreadyStarted
	}

	found := false
	for i := range detail.Members {
		if detail.Members[i].CharacterID == characterID {
			detail.Members[i].TeamColor = normColor
			found = true
			break
		}
	}
	if !found {
		return RoomDetail{}, ErrCharacterNotInRoom
	}

	detail.Room.UpdatedAt = time.Now().UTC()
	if err := s.repo.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		return RoomDetail{}, err
	}
	return detail, nil
}

// LeaveRoom removes a participant or disbands the room (@にげる).
func (s *Service) LeaveRoom(ctx context.Context, characterID string, roomID string) error {
	detail, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return err
	}

	found := false
	isLeader := false
	for _, m := range detail.Members {
		if m.CharacterID == characterID {
			found = true
			isLeader = m.IsLeader
			break
		}
	}
	if !found {
		return ErrCharacterNotInRoom
	}
	if detail.Room.Status == StatusInProgress {
		return ErrMatchAlreadyStarted
	}

	now := time.Now().UTC()

	// If recruiting: leader leaving disbands and refunds everyone; non-leader leaving refunds themselves.
	if detail.Room.Status == StatusRecruiting {
		if isLeader {
			// Disband and refund all members
			for _, m := range detail.Members {
				if mChar, err := s.characters.FindByID(ctx, m.CharacterID); err == nil {
					_ = mChar.AddMoney(detail.Room.Bet)
					_ = s.characters.Update(ctx, mChar)
				}
				_ = s.repo.DeleteCharacterRoom(ctx, m.CharacterID)
			}
			detail.Room.Status = StatusDisbanded
			detail.Room.PrizePool = 0
			detail.Room.UpdatedAt = now
			return s.repo.DeleteRoom(ctx, roomID)
		}

		// Non-leader leaving: refund bet
		_ = char.AddMoney(detail.Room.Bet)
		_ = s.characters.Update(ctx, char)
		_ = s.repo.DeleteCharacterRoom(ctx, char.ID)

		var updatedMembers []RoomMember
		for _, m := range detail.Members {
			if m.CharacterID != characterID {
				updatedMembers = append(updatedMembers, m)
			}
		}
		detail.Members = updatedMembers
		detail.Room.PrizePool -= detail.Room.Bet
		if detail.Room.PrizePool < 0 {
			detail.Room.PrizePool = 0
		}
		detail.Room.UpdatedAt = now
		return s.repo.SaveRoom(ctx, detail.Room, detail.Members)
	}

	// In completed or disbanded status: simply remove mapping
	_ = s.repo.DeleteCharacterRoom(ctx, char.ID)
	return nil
}

func (s *Service) GetRoom(ctx context.Context, roomID string) (RoomDetail, error) {
	return s.repo.GetRoom(ctx, roomID)
}

func (s *Service) ListRooms(ctx context.Context) ([]RoomSummary, error) {
	return s.repo.ListRooms(ctx)
}

func (s *Service) GetCharacterRoom(ctx context.Context, characterID string) (RoomDetail, error) {
	roomID, err := s.repo.GetCharacterRoom(ctx, characterID)
	if err != nil {
		return RoomDetail{}, err
	}
	return s.repo.GetRoom(ctx, roomID)
}
