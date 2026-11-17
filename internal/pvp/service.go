package pvp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
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
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// BattleEngine defines the battle resolution contract.
type BattleEngine interface {
	ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

// ParticipantBuilder constructs a Participant from character ID with current HP.
type ParticipantBuilder interface {
	BuildParticipantWithCurrentHP(ctx context.Context, characterID string, currentHP int) (corebattle.Participant, error)
}

// Option configures Service dependencies.
type Option func(*Service)

// WithParticipantBuilder configures the ParticipantBuilder.
func WithParticipantBuilder(builder ParticipantBuilder) Option {
	return func(s *Service) {
		s.participantBuilder = builder
	}
}

type Service struct {
	repo               RoomRepository
	characters         CharacterRepository
	battleEngine       BattleEngine
	participantBuilder ParticipantBuilder
	txProvider         TransactionProvider
	transitions        TransitionRepository
	prizes             PrizeRemitter
	timers             TimerService
	clock              func() time.Time
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
	if svc.transitions == nil {
		return nil, ErrInvalidDependencies
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
	recovered, err := s.recoverCharacter(ctx, characterID)
	if err != nil {
		return RoomDetail{}, err
	}
	if recovered.Kind == "timeout" && slices.Contains(recovered.RemovedIDs, characterID) {
		return RoomDetail{}, ErrCharacterSleeping
	}
	if recovered.Kind == "create" && recovered.ActorID == characterID && recovered.Detail.Room.Status != StatusDisbanded {
		recovered.Detail.Room.PasswordHash = recovered.PasswordHash
		return recovered.Detail, nil
	}
	if sleeping, err := s.isCharacterSleeping(ctx, characterID); err != nil {
		return RoomDetail{}, err
	} else if sleeping {
		return RoomDetail{}, ErrCharacterSleeping
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return RoomDetail{}, err
	}
	if char.PendingWake {
		return RoomDetail{}, ErrCharacterSleeping
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
	if existingRoomID, err := s.repo.GetCharacterRoom(ctx, char.ID); err == nil && existingRoomID != "" {
		if existing, err := s.repo.GetRoom(ctx, existingRoomID); err == nil && existing.Room.Status != StatusDisbanded {
			return RoomDetail{}, ErrAlreadyInRoom
		}
	} else if err != nil && !errors.Is(err, ErrRoomNotFound) {
		return RoomDetail{}, err
	}

	now := s.now()
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
		LastActivityAt:    now,
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
	detail := RoomDetail{Room: room, Members: members}
	if err := s.repo.WithRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		return s.commitTransition(lockedCtx, Transition{Kind: "create", ActorID: char.ID, Detail: detail, AddedID: char.ID, LastActivityAt: now}, func(txCtx context.Context) error {
			return s.deductBet(txCtx, char.ID, req.Bet)
		})
	}); err != nil {
		return RoomDetail{}, err
	}

	detail.Room.FinancialVersion++
	return detail, nil
}

// JoinRoom adds a character to an active colosseum room (quest.cgi:type=4).
func (s *Service) JoinRoom(ctx context.Context, characterID string, roomID string, password string) (RoomDetail, error) {
	recovered, err := s.recoverCharacter(ctx, characterID)
	if err != nil {
		return RoomDetail{}, err
	}
	if recovered.Kind == "timeout" && slices.Contains(recovered.RemovedIDs, characterID) {
		return RoomDetail{}, ErrCharacterSleeping
	}
	if sleeping, err := s.isCharacterSleeping(ctx, characterID); err != nil {
		return RoomDetail{}, err
	} else if sleeping {
		return RoomDetail{}, ErrCharacterSleeping
	}
	var result RoomDetail
	err = s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		detail, err := s.checkInactiveLocked(lockedCtx, roomID)
		if err != nil {
			return err
		}
		char, err := s.characters.FindByID(lockedCtx, characterID)
		if err != nil {
			return err
		}
		if char.PendingWake {
			return ErrCharacterSleeping
		}
		if char.Stats.HP <= 0 {
			return ErrCharacterUnconscious
		}
		if char.Tired >= 100 {
			return ErrCharacterExhausted
		}
		if detail.Room.Status != StatusRecruiting {
			return ErrRoomNotRecruiting
		}
		for _, m := range detail.Members {
			if m.CharacterID == char.ID {
				result = detail
				return nil
			}
		}
		if len(detail.Members) >= detail.Room.MaxMembers {
			return ErrRoomFull
		}

		if existingRoomID, err := s.repo.GetCharacterRoom(lockedCtx, char.ID); err == nil && existingRoomID != "" && existingRoomID != roomID {
			if existing, err := s.repo.GetRoom(lockedCtx, existingRoomID); err == nil && existing.Room.Status != StatusDisbanded {
				return ErrAlreadyInRoom
			}
		} else if err != nil && !errors.Is(err, ErrRoomNotFound) {
			return err
		}

		if detail.Room.PasswordHash != "" && hashPassword(password) != detail.Room.PasswordHash {
			return ErrInvalidPassword
		}
		if err := checkNeedJoin(char, detail.Room.NeedJoin); err != nil {
			return err
		}

		now := s.now()
		last := lastTransitionFromContext(lockedCtx)
		lastActivity := last.LastActivityAt
		if lastActivity.IsZero() {
			lastActivity = detail.Room.LastActivityAt
		}
		if lastActivity.IsZero() {
			lastActivity = detail.Room.CreatedAt
		}
		detail.Room.PrizePool += detail.Room.Bet
		detail.Room.UpdatedAt = now
		// Legacy quest.cgi:add_member writes to quest.cgi, not the room's log.cgi.
		detail.Room.LastActivityAt = lastActivity

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
		if err := s.commitTransition(lockedCtx, Transition{Kind: "join", ActorID: char.ID, Detail: detail, AddedID: char.ID, LastActivityAt: lastActivity}, func(txCtx context.Context) error {
			return s.deductBet(txCtx, char.ID, detail.Room.Bet)
		}); err != nil {
			return err
		}
		detail.Room.FinancialVersion++

		result = detail
		return nil
	})
	return result, err
}

// SelectTeam assigns a participant to one of the 9 team colors (@ぱーてぃー).
func (s *Service) SelectTeam(ctx context.Context, characterID string, roomID string, teamColor string) (RoomDetail, error) {
	normColor := strings.ToUpper(strings.TrimSpace(teamColor))
	if !IsValidTeamColor(normColor) {
		return RoomDetail{}, ErrInvalidTeamColor
	}

	var result RoomDetail
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		detail, err := s.checkInactiveLocked(lockedCtx, roomID)
		if err != nil {
			return err
		}
		if detail.Room.Round > 0 || detail.Room.Status != StatusRecruiting {
			return ErrMatchAlreadyStarted
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
			return ErrCharacterNotInRoom
		}

		now := s.now()
		detail.Room.UpdatedAt = now
		detail.Room.LastActivityAt = now
		if err := s.transitions.TouchActivity(lockedCtx, roomID, detail.Room.FinancialVersion, now); err != nil {
			if errors.Is(err, ErrRoomInactive) {
				if _, cleanErr := s.cleanupInactiveRoom(lockedCtx, lastTransitionFromContext(lockedCtx), now); cleanErr != nil {
					return cleanErr
				}
				return ErrRoomNotFound
			}
			return err
		}
		if err := s.saveLiveRoom(lockedCtx, detail); err != nil {
			return err
		}
		result = detail
		return nil
	})
	return result, err
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
	detail, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		return RoomDetail{}, err
	}
	if detail.Room.Status == StatusCompleted && !hasRoomMember(detail.Members, characterID) {
		return RoomDetail{}, ErrRoomNotFound
	}
	return detail, nil
}
