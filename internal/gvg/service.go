package gvg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/validation"
)

// CharacterRepository defines persistence operations on Character.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
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

// ParticipantBuilder constructs a Participant from character ID with current HP.
type ParticipantBuilder interface {
	BuildParticipantWithCurrentHP(ctx context.Context, characterID string, currentHP int) (corebattle.Participant, error)
	BuildBattleMaxima(ctx context.Context, char corecharacter.Character) (hp, mp int, err error)
}

// StatefulParticipantBuilder builds a combat participant with current vitality and buffered item consumption.
type StatefulParticipantBuilder interface {
	BuildParticipantWithState(ctx context.Context, characterID string, hp int, mp *int, consumed []corebattle.ConsumedItem) (corebattle.Participant, error)
}

// PostBattleSettler persists post-exit character vitality, fatigue, item consumption, and accessory bonuses.
type PostBattleSettler interface {
	ApplyPostBattleResult(ctx context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error)
}

// TransactionProvider executes a callback within a database transaction.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// ExitClaimState tracks idempotent exit settlement per character within a room.
type ExitClaimState struct {
	AlreadyClaimed bool
	DeliveryDone   bool
}

// ExitClaimRepository records durable per-join GvG exit claims for retry safety.
type ExitClaimRepository interface {
	ClaimRoomExitForUpdate(ctx context.Context, roomID, characterID, joinID string) (ExitClaimState, error)
	MarkExitDeliveryDone(ctx context.Context, roomID, characterID, joinID string) error
}

// PostExitHook resets meal fullness and settles any standing Tavern delivery upon room exit.
// It returns nil when no delivery was standing.
type PostExitHook func(ctx context.Context, characterID string) (*DeliveryOutcome, error)

// Option configures Service dependencies.
type Option func(*Service)

// WithParticipantBuilder configures the ParticipantBuilder.
func WithParticipantBuilder(builder ParticipantBuilder) Option {
	return func(s *Service) {
		s.participantBuilder = builder
	}
}

// WithPostBattleSettler configures the PostBattleSettler for GvG exit side-effects.
func WithPostBattleSettler(settler PostBattleSettler) Option {
	return func(s *Service) {
		s.settler = settler
	}
}

// WithTransactionProvider configures the TransactionProvider for exit claim atomicity.
func WithTransactionProvider(provider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = provider
	}
}

// WithExitClaimRepository configures the ExitClaimRepository for idempotent exit side-effects.
func WithExitClaimRepository(repo ExitClaimRepository) Option {
	return func(s *Service) {
		s.exitClaims = repo
	}
}

// WithPostExitHook configures the post-exit hook (e.g., tavern fullness reset and delivery claim).
func WithPostExitHook(hook PostExitHook) Option {
	return func(s *Service) {
		s.postExitHook = hook
	}
}

// Service manages real-time GvG rooms and standings.
type Service struct {
	repo               RoomRepository
	standings          StandingRepository
	guilds             GuildRepository
	characters         CharacterRepository
	battleEngine       BattleEngine
	participantBuilder ParticipantBuilder
	settler            PostBattleSettler
	txProvider         TransactionProvider
	exitClaims         ExitClaimRepository
	postExitHook       PostExitHook
}

// SetPostExitHook configures the post-exit hook after service construction.
func (s *Service) SetPostExitHook(hook PostExitHook) {
	s.postExitHook = hook
}

// NewService constructs a new GvG service.
func NewService(
	repo RoomRepository,
	standings StandingRepository,
	guilds GuildRepository,
	characters CharacterRepository,
	battleEngine BattleEngine,
	opts ...Option,
) (*Service, error) {
	if repo == nil || standings == nil || guilds == nil || characters == nil || battleEngine == nil {
		return nil, ErrInvalidDependencies
	}
	s := &Service{
		repo:         repo,
		standings:    standings,
		guilds:       guilds,
		characters:   characters,
		battleEngine: battleEngine,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func hashPassword(pw string) string {
	if pw == "" {
		return ""
	}
	h := sha256.Sum256([]byte(pw))
	return hex.EncodeToString(h[:])
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
	} else if !errors.Is(err, ErrRoomNotFound) {
		return RoomDetail{}, err
	}

	name, err := validation.ValidateQuestName(req.Name)
	if err != nil {
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

	if corecharacter.ValidateNeedJoin(req.NeedJoin, char) != nil {
		return RoomDetail{}, ErrNeedJoinNotMet
	}

	roomID := id.New()
	now := time.Now().UTC()

	room := GvGRoom{
		ID:                roomID,
		Name:              name,
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

	leaderMaxHP, leaderMaxMP, err := s.battleMaxima(ctx, char)
	if err != nil {
		return RoomDetail{}, err
	}
	leaderMP := max(0, char.Stats.MP)
	member := GvGMember{
		RoomID:        roomID,
		CharacterID:   char.ID,
		JoinID:        id.New(),
		CharacterName: char.Name,
		JobID:         char.JobID,
		Level:         char.Level,
		HP:            char.Stats.HP,
		MaxHP:         leaderMaxHP,
		MP:            &leaderMP,
		MaxMP:         leaderMaxMP,
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
		//lint:ignore error-swallow best-effort room cleanup on mapping failure
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

	var result RoomDetail
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		char, err := s.characters.FindByID(lockedCtx, charID)
		if err != nil {
			return ErrCharacterNotFound
		}
		if char.Stats.HP <= 0 {
			return ErrCharacterUnconscious
		}
		if char.Tired >= 100 {
			return ErrCharacterExhausted
		}

		g, _, err := s.guilds.GetGuildByCharacter(lockedCtx, charID)
		if err != nil {
			return ErrActorNotInGuild
		}
		if strings.EqualFold(g.Color, DefaultColor) || g.Color == "" {
			return ErrFriendlyGuildCannotBattle
		}

		if _, err := s.repo.GetCharacterRoom(lockedCtx, charID); err == nil {
			return ErrAlreadyInRoom
		} else if !errors.Is(err, ErrRoomNotFound) {
			return err
		}

		detail, err := s.repo.GetRoom(lockedCtx, roomID)
		if err != nil {
			return err
		}

		if detail.Room.Status != StatusRecruiting {
			return ErrRoomNotRecruiting
		}
		if len(detail.Members) >= detail.Room.MaxMembers {
			return ErrRoomFull
		}
		if detail.Room.PasswordHash != "" && detail.Room.PasswordHash != hashPassword(strings.TrimSpace(password)) {
			return ErrInvalidPassword
		}
		if corecharacter.ValidateNeedJoin(detail.Room.NeedJoin, char) != nil {
			return ErrNeedJoinNotMet
		}

		guildMembers := 0
		for _, m := range detail.Members {
			if m.CharacterID == charID {
				return ErrAlreadyInRoom
			}
			if m.GuildID == g.ID {
				guildMembers++
			}
		}
		if guildMembers >= MaxGuildMembers {
			return ErrGuildMemberLimitExceeded
		}

		now := time.Now().UTC()
		maxHP, maxMP, err := s.battleMaxima(lockedCtx, char)
		if err != nil {
			return err
		}
		memberMP := max(0, char.Stats.MP)
		newMember := GvGMember{
			RoomID:        roomID,
			CharacterID:   char.ID,
			JoinID:        id.New(),
			CharacterName: char.Name,
			JobID:         char.JobID,
			Level:         char.Level,
			HP:            char.Stats.HP,
			MaxHP:         maxHP,
			MP:            &memberMP,
			MaxMP:         maxMP,
			GuildID:       g.ID,
			GuildName:     g.Name,
			GuildColor:    g.Color,
			IsLeader:      false,
			JoinedAt:      now,
		}

		detail.Members = append(detail.Members, newMember)
		detail.Room.PrizePool += JoinPrizeGP // Joiner adds 1 GP
		detail.Room.UpdatedAt = now

		if err := s.repo.SaveRoom(lockedCtx, detail.Room, detail.Members); err != nil {
			return fmt.Errorf("save gvg room on join: %w", err)
		}
		if err := s.repo.SetCharacterRoom(lockedCtx, char.ID, roomID); err != nil {
			return fmt.Errorf("set character room on join: %w", err)
		}

		result = detail
		return nil
	})
	return result, err
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

func (s *Service) battleMaxima(ctx context.Context, char corecharacter.Character) (int, *int, error) {
	hp, mp := char.Stats.MaxHP, char.Stats.MaxMP
	if s.participantBuilder != nil {
		var err error
		hp, mp, err = s.participantBuilder.BuildBattleMaxima(ctx, char)
		if err != nil {
			return 0, nil, err
		}
	}
	return hp, &mp, nil
}
