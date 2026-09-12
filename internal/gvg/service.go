package gvg

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
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
)

// CharacterRepository defines persistence operations on Character.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// GuildRepository defines persistence operations on Guilds.
type GuildRepository interface {
	GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error)
	GetGuildByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error)
}

// BattleEngine defines the party battle resolution contract.
type BattleEngine interface {
	ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

// Service manages real-time GvG rooms and standings.
type Service struct {
	repo         RoomRepository
	standings    StandingRepository
	guilds       GuildRepository
	characters   CharacterRepository
	battleEngine BattleEngine
}

// NewService constructs a new GvG service.
func NewService(
	repo RoomRepository,
	standings StandingRepository,
	guilds GuildRepository,
	characters CharacterRepository,
	battleEngine BattleEngine,
) (*Service, error) {
	if repo == nil || standings == nil || guilds == nil || characters == nil || battleEngine == nil {
		return nil, ErrInvalidDependencies
	}
	return &Service{
		repo:         repo,
		standings:    standings,
		guilds:       guilds,
		characters:   characters,
		battleEngine: battleEngine,
	}, nil
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
		jobLv := char.JobLevel
		if op == "o" && jobLv < val {
			return ErrNeedJoinNotMet
		}
		if op == "u" && jobLv >= val {
			return ErrNeedJoinNotMet
		}
	}
	return nil
}

// CreateRoom creates a new live GvG room (quest.cgi:839 girudobatoru).
func (s *Service) CreateRoom(ctx context.Context, creatorCharID string, req CreateRoomRequest) (RoomDetail, error) {
	creatorCharID = strings.TrimSpace(creatorCharID)
	if creatorCharID == "" {
		return RoomDetail{}, ErrCharacterNotFound
	}

	char, err := s.characters.FindByID(ctx, creatorCharID)
	if err != nil {
		return RoomDetail{}, ErrCharacterNotFound
	}
	if char.Stats.HP <= 0 {
		return RoomDetail{}, ErrCharacterUnconscious
	}
	if char.Tired >= 100 {
		return RoomDetail{}, ErrCharacterExhausted
	}

	// Creator must belong to a non-friendly guild
	g, _, err := s.guilds.GetGuildByCharacter(ctx, creatorCharID)
	if err != nil {
		return RoomDetail{}, ErrActorNotInGuild
	}
	if strings.EqualFold(g.Color, DefaultColor) || g.Color == "" {
		return RoomDetail{}, ErrFriendlyGuildCannotBattle
	}

	if _, err := s.repo.GetCharacterRoom(ctx, creatorCharID); err == nil {
		return RoomDetail{}, ErrAlreadyInRoom
	}

	nameLen := utf8.RuneCountInString(strings.TrimSpace(req.Name))
	if nameLen < 1 || nameLen > 50 {
		return RoomDetail{}, ErrInvalidRoomName
	}

	maxMembers := req.MaxMembers
	if maxMembers == 0 {
		maxMembers = MaxMembers
	}
	if maxMembers < MinMembers || maxMembers > MaxMembers {
		return RoomDetail{}, ErrInvalidMaxMembers
	}

	targetWins := req.TargetWins
	if targetWins == 0 {
		targetWins = MinTargetWins
	}
	if targetWins < MinTargetWins || targetWins > MaxTargetWins {
		return RoomDetail{}, ErrInvalidTargetWins
	}

	speed := req.Speed
	if speed <= 0 {
		speed = DefaultSpeed
	}

	if err := checkNeedJoin(char, req.NeedJoin); err != nil {
		return RoomDetail{}, err
	}

	roomID := id.New()
	now := time.Now().UTC()

	room := GvGRoom{
		ID:                roomID,
		Name:              strings.TrimSpace(req.Name),
		LeaderCharacterID: char.ID,
		LeaderName:        char.Name,
		Speed:             speed,
		Stage:             req.Stage,
		MaxMembers:        maxMembers,
		TargetWins:        targetWins,
		PrizePool:         InitialPrizeGP, // Leader seeds initial 2 GP
		PasswordHash:      hashPassword(strings.TrimSpace(req.Password)),
		NeedJoin:          req.NeedJoin,
		Status:            StatusRecruiting,
		Round:             0,
		GuildScores:       make(map[string]int),
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	member := GvGMember{
		RoomID:        roomID,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		JobID:         char.JobID,
		Level:         char.Level,
		HP:            char.Stats.HP,
		MaxHP:         char.Stats.MaxHP,
		GuildID:       g.ID,
		GuildName:     g.Name,
		GuildColor:    g.Color,
		IsLeader:      true,
		JoinedAt:      now,
	}

	members := []GvGMember{member}

	if err := s.repo.SaveRoom(ctx, room, members); err != nil {
		return RoomDetail{}, fmt.Errorf("save gvg room: %w", err)
	}
	if err := s.repo.SetCharacterRoom(ctx, char.ID, roomID); err != nil {
		_ = s.repo.DeleteRoom(ctx, roomID)
		return RoomDetail{}, fmt.Errorf("map character to gvg room: %w", err)
	}

	return RoomDetail{Room: room, Members: members}, nil
}

// JoinRoom joins an existing GvG room (quest.cgi:1066 type=5).
func (s *Service) JoinRoom(ctx context.Context, charID string, roomID string, password string) (RoomDetail, error) {
	charID = strings.TrimSpace(charID)
	roomID = strings.TrimSpace(roomID)
	if charID == "" {
		return RoomDetail{}, ErrCharacterNotFound
	}
	if roomID == "" {
		return RoomDetail{}, ErrRoomNotFound
	}

	char, err := s.characters.FindByID(ctx, charID)
	if err != nil {
		return RoomDetail{}, ErrCharacterNotFound
	}
	if char.Stats.HP <= 0 {
		return RoomDetail{}, ErrCharacterUnconscious
	}
	if char.Tired >= 100 {
		return RoomDetail{}, ErrCharacterExhausted
	}

	g, _, err := s.guilds.GetGuildByCharacter(ctx, charID)
	if err != nil {
		return RoomDetail{}, ErrActorNotInGuild
	}
	if strings.EqualFold(g.Color, DefaultColor) || g.Color == "" {
		return RoomDetail{}, ErrFriendlyGuildCannotBattle
	}

	if _, err := s.repo.GetCharacterRoom(ctx, charID); err == nil {
		return RoomDetail{}, ErrAlreadyInRoom
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
	if detail.Room.PasswordHash != "" && detail.Room.PasswordHash != hashPassword(strings.TrimSpace(password)) {
		return RoomDetail{}, ErrInvalidPassword
	}
	if err := checkNeedJoin(char, detail.Room.NeedJoin); err != nil {
		return RoomDetail{}, err
	}

	for _, m := range detail.Members {
		if m.CharacterID == charID {
			return RoomDetail{}, ErrAlreadyInRoom
		}
	}

	now := time.Now().UTC()
	newMember := GvGMember{
		RoomID:        roomID,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		JobID:         char.JobID,
		Level:         char.Level,
		HP:            char.Stats.HP,
		MaxHP:         char.Stats.MaxHP,
		GuildID:       g.ID,
		GuildName:     g.Name,
		GuildColor:    g.Color,
		IsLeader:      false,
		JoinedAt:      now,
	}

	detail.Members = append(detail.Members, newMember)
	detail.Room.PrizePool += JoinPrizeGP // Joiner adds 1 GP
	detail.Room.UpdatedAt = now

	if err := s.repo.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		return RoomDetail{}, fmt.Errorf("save gvg room on join: %w", err)
	}
	if err := s.repo.SetCharacterRoom(ctx, char.ID, roomID); err != nil {
		return RoomDetail{}, fmt.Errorf("set character room on join: %w", err)
	}

	return detail, nil
}

// LeaveRoom exits a GvG room or disbands if caller is leader.
func (s *Service) LeaveRoom(ctx context.Context, charID string, roomID string) error {
	detail, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}

	isMember := false
	for _, m := range detail.Members {
		if m.CharacterID == charID {
			isMember = true
			break
		}
	}
	if !isMember {
		return ErrCharacterNotInRoom
	}

	// Disband room if leader leaves or match completed
	if detail.Room.LeaderCharacterID == charID || detail.Room.Status == StatusCompleted {
		for _, m := range detail.Members {
			_ = s.repo.DeleteCharacterRoom(ctx, m.CharacterID)
		}
		return s.repo.DeleteRoom(ctx, roomID)
	}

	if detail.Room.Status != StatusRecruiting {
		return ErrMatchNotInProgress
	}

	// Remove regular member
	newMembers := make([]GvGMember, 0, len(detail.Members)-1)
	for _, m := range detail.Members {
		if m.CharacterID != charID {
			newMembers = append(newMembers, m)
		}
	}
	detail.Members = newMembers
	if detail.Room.PrizePool > InitialPrizeGP {
		detail.Room.PrizePool -= JoinPrizeGP
	}
	detail.Room.UpdatedAt = time.Now().UTC()

	_ = s.repo.DeleteCharacterRoom(ctx, charID)
	return s.repo.SaveRoom(ctx, detail.Room, detail.Members)
}

// GetRoom retrieves details of a specific GvG room.
func (s *Service) GetRoom(ctx context.Context, roomID string) (RoomDetail, error) {
	if strings.TrimSpace(roomID) == "" {
		return RoomDetail{}, ErrRoomNotFound
	}
	return s.repo.GetRoom(ctx, roomID)
}

// ListRooms lists all active recruiting GvG rooms.
func (s *Service) ListRooms(ctx context.Context) ([]RoomSummary, error) {
	return s.repo.ListRooms(ctx)
}

// GetCharacterRoom retrieves the active GvG room for a character.
func (s *Service) GetCharacterRoom(ctx context.Context, characterID string) (RoomDetail, error) {
	roomID, err := s.repo.GetCharacterRoom(ctx, characterID)
	if err != nil {
		return RoomDetail{}, err
	}
	return s.repo.GetRoom(ctx, roomID)
}

// GetStanding retrieves standing for a guild.
func (s *Service) GetStanding(ctx context.Context, guildID string) (GvGStanding, error) {
	if strings.TrimSpace(guildID) == "" {
		return GvGStanding{}, ErrInvalidGuildID
	}
	return s.standings.GetOrCreateStanding(ctx, guildID)
}

// GetLeaderboard retrieves top guild standings.
func (s *Service) GetLeaderboard(ctx context.Context, limit int) ([]GvGStanding, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.standings.GetLeaderboard(ctx, limit)
}
