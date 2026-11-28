package guild

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/validation"
)

type Service struct {
	mutationCheck func(context.Context, string) error
	repo          Repository
	charReader    CharacterReader
	letterSender  LetterSender
	newsPub       NewsPublisher
	memories      corecharacter.MemoryRecorder
	txProvider    TransactionProvider
	nowFunc       func() time.Time
}

func NewService(repo Repository, opts ...ServiceOption) (*Service, error) {
	if repo == nil {
		return nil, errors.New("guild repository is nil")
	}
	s := &Service{
		repo: repo,
		nowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

// touchActive performs a best-effort update of the guild's last_active_at timestamp.
func (s *Service) touchActive(ctx context.Context, guildID string) {
	//lint:ignore error-swallow best-effort guild activity touch
	_ = s.repo.TouchActive(ctx, guildID)
}

func (s *Service) Create(ctx context.Context, creatorCharID string, name string) (Guild, Member, corecharacter.Character, error) {
	creatorCharID = strings.TrimSpace(creatorCharID)
	if creatorCharID == "" {
		return Guild{}, Member{}, corecharacter.Character{}, ErrCharacterNotFound
	}
	validName, err := ValidateGuildName(name)
	if err != nil {
		return Guild{}, Member{}, corecharacter.Character{}, ErrInvalidGuildName
	}

	// Verify creator is not currently in any guild
	if _, _, err := s.repo.GetGuildByCharacter(ctx, creatorCharID); err == nil {
		return Guild{}, Member{}, corecharacter.Character{}, ErrCharacterAlreadyInGuild
	} else if !errors.Is(err, ErrCharacterNotInGuild) {
		return Guild{}, Member{}, corecharacter.Character{}, err
	}

	guildID := id.New()
	now := s.nowFunc().UTC()
	g := Guild{
		ID:                guildID,
		Name:              validName,
		LeaderCharacterID: creatorCharID,
		Points:            0,
		Notice:            "",
		Color:             DefaultColor,
		Mark:              DefaultMark,
		LastActiveAt:      now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	m := Member{
		GuildID:     guildID,
		CharacterID: creatorCharID,
		Role:        RoleLeader,
		Title:       DefaultTitleLeader,
		IsPending:   false,
		JoinedAt:    now,
	}

	var (
		createdGuild  Guild
		createdMember Member
		updatedChar   corecharacter.Character
	)
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockCharacterForUpdate(txCtx, creatorCharID); err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, creatorCharID); err != nil {
			return err
		}
		if _, _, err := s.repo.GetGuildByCharacter(txCtx, creatorCharID); err == nil {
			return ErrCharacterAlreadyInGuild
		} else if !errors.Is(err, ErrCharacterNotInGuild) {
			return err
		}
		var cErr error
		createdGuild, createdMember, updatedChar, cErr = s.repo.CreateGuild(txCtx, g, m, CreationFee)
		if cErr != nil {
			return cErr
		}
		return s.memories.Record(txCtx, creatorCharID, corecharacter.MemoryGuildFound, "新ギルド"+validName+"を設立！")
	})
	if err != nil {
		return Guild{}, Member{}, corecharacter.Character{}, err
	}
	return createdGuild, createdMember, updatedChar, nil
}

func (s *Service) Get(ctx context.Context, guildID string) (Detail, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return Detail{}, ErrInvalidGuildID
	}
	g, members, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return Detail{}, err
	}
	decorations, err := s.repo.ListDecorations(ctx, guildID)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Guild: g, Members: members, Decorations: decorations}, nil
}

func (s *Service) GetByCharacter(ctx context.Context, characterID string) (Guild, Member, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return Guild{}, Member{}, ErrCharacterNotFound
	}
	return s.repo.GetGuildByCharacter(ctx, characterID)
}

func (s *Service) List(ctx context.Context, offset, limit int) ([]Guild, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListGuilds(ctx, offset, limit)
}

// RecordRoomPresence keeps the guild active while members stay in its room, as
// legacy refreshed log_member.cgi, whose age decides inactivity (join_guild.cgi:423-433).
func (s *Service) RecordRoomPresence(ctx context.Context, guildID string) error {
	return s.repo.TouchActive(ctx, guildID)
}

// UpdateColor changes the guild's hex color (guild.cgi:color).
// Only the guild leader can change color.
// Validates hex format, NPC color prohibition, and server-wide uniqueness.
// Transaction: RunInTx (authorization under Rank-7 lock and color uniqueness/update).
// Lock Order: guilds(7) -> guild_members(7).
func (s *Service) UpdateColor(ctx context.Context, guildID string, requesterCharID string, color string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	requesterCharID = strings.TrimSpace(requesterCharID)
	if requesterCharID == "" {
		return ErrCharacterNotFound
	}

	normalizedColor, err := ValidateColorFormat(color)
	if err != nil {
		return err
	}

	// Legacy rule: White (#FFFFFF) is allowed and can be shared, but non-white colors must be unique.
	// NPC color (#FF69B4) is reserved and cannot be selected.
	if normalizedColor == NPCColor {
		return ErrColorTaken
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		lockedGuild, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}

		if err := authorizeCurrentLeader(lockedGuild, members, requesterCharID); err != nil {
			return err
		}

		if normalizedColor != DefaultColor {
			taken, err := s.repo.IsColorTaken(txCtx, normalizedColor, lockedGuild.ID)
			if err != nil {
				return err
			}
			if taken {
				return ErrColorTaken
			}
		}

		if err := s.repo.UpdateColor(txCtx, guildID, normalizedColor); err != nil {
			return err
		}
		s.touchActive(txCtx, guildID)
		return nil
	})
}

// UpdateNotice changes the guild's notice message (guild.cgi:message).
// Only the guild leader can update notice.
// Transaction: RunInTx (authorization under Rank-7 lock and notice update).
// Lock Order: guilds(7) -> guild_members(7).
func (s *Service) UpdateNotice(ctx context.Context, guildID string, requesterCharID string, notice string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	requesterCharID = strings.TrimSpace(requesterCharID)
	if requesterCharID == "" {
		return ErrCharacterNotFound
	}
	notice, err := validation.ValidatePlainLine(notice, MaxNoticeLength)
	if errors.Is(err, validation.ErrTooLong) {
		return ErrNoticeTooLong
	} else if err != nil {
		return ErrInvalidNotice
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		lockedGuild, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}

		if err := authorizeCurrentLeader(lockedGuild, members, requesterCharID); err != nil {
			return err
		}

		if err := s.repo.UpdateNotice(txCtx, guildID, notice); err != nil {
			return err
		}
		s.touchActive(txCtx, guildID)
		return nil
	})
}
