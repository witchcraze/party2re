package guild

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

type Service struct {
	repo         Repository
	charReader   CharacterReader
	letterSender LetterSender
	newsPub      NewsPublisher
	txProvider   TransactionProvider
	nowFunc      func() time.Time
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
		if _, _, err := s.repo.GetGuildByCharacter(txCtx, creatorCharID); err == nil {
			return ErrCharacterAlreadyInGuild
		} else if !errors.Is(err, ErrCharacterNotInGuild) {
			return err
		}
		var cErr error
		createdGuild, createdMember, updatedChar, cErr = s.repo.CreateGuild(txCtx, g, m, CreationFee)
		return cErr
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
	return Detail{Guild: g, Members: members}, nil
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

func (s *Service) Join(ctx context.Context, guildID string, characterID string) (Member, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return Member{}, ErrInvalidGuildID
	}
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return Member{}, ErrCharacterNotFound
	}

	// Verify character is not already in a guild
	if _, _, err := s.repo.GetGuildByCharacter(ctx, characterID); err == nil {
		return Member{}, ErrCharacterAlreadyInGuild
	} else if !errors.Is(err, ErrCharacterNotInGuild) {
		return Member{}, err
	}

	var res Member
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockCharacterForUpdate(txCtx, characterID); err != nil {
			return err
		}
		if _, _, err := s.repo.GetGuildForUpdate(txCtx, guildID); err != nil {
			return err
		}

		// Verify character is not already in a guild
		if _, _, err := s.repo.GetGuildByCharacter(txCtx, characterID); err == nil {
			return ErrCharacterAlreadyInGuild
		} else if !errors.Is(err, ErrCharacterNotInGuild) {
			return err
		}

		m := Member{
			GuildID:     guildID,
			CharacterID: characterID,
			Role:        RoleMember,
			Title:       "",
			IsPending:   false,
			JoinedAt:    s.nowFunc().UTC(),
		}

		var addErr error
		res, addErr = s.repo.AddMember(txCtx, m)
		if addErr != nil {
			return addErr
		}
		s.touchActive(txCtx, guildID)
		return nil
	})
	if err != nil {
		return Member{}, err
	}
	return res, nil
}

// Kick removes an active member or delegates pending removal to the rejection workflow.
// Transaction: RunInTx (authorization, classification and removal; notification follows success).
// Lock Order: guilds(7) -> guild_members(7).
func (s *Service) Kick(ctx context.Context, guildID string, requesterCharID string, targetCharID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	requesterCharID = strings.TrimSpace(requesterCharID)
	targetCharID = strings.TrimSpace(targetCharID)
	if requesterCharID == "" || targetCharID == "" {
		return ErrCharacterNotFound
	}
	if requesterCharID == targetCharID {
		return errors.New("cannot kick self; use leave")
	}

	return s.removeAuthorizedMember(ctx, guildID, requesterCharID, targetCharID, false)
}

func (s *Service) TransferLeadership(ctx context.Context, guildID string, currentLeaderCharID string, newLeaderCharID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	currentLeaderCharID = strings.TrimSpace(currentLeaderCharID)
	newLeaderCharID = strings.TrimSpace(newLeaderCharID)
	if currentLeaderCharID == "" || newLeaderCharID == "" {
		return ErrCharacterNotFound
	}
	if currentLeaderCharID == newLeaderCharID {
		return nil
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockCharacterForUpdate(txCtx, newLeaderCharID); err != nil {
			if errors.Is(err, ErrCharacterNotFound) {
				return ErrTargetNotMember
			}
			return err
		}

		_, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}

		var currentLeader, newLeader *Member
		for i := range members {
			if members[i].CharacterID == currentLeaderCharID {
				currentLeader = &members[i]
			}
			if members[i].CharacterID == newLeaderCharID {
				newLeader = &members[i]
			}
		}

		if currentLeader == nil || currentLeader.Role != RoleLeader {
			return ErrUnauthorized
		}
		if newLeader == nil {
			return ErrTargetNotMember
		}
		if newLeader.IsPending {
			if activeGuild, _, err := s.repo.GetGuildByCharacter(txCtx, newLeaderCharID); err == nil {
				if activeGuild.ID != guildID {
					return ErrCharacterAlreadyInGuild
				}
			} else if !errors.Is(err, ErrCharacterNotInGuild) {
				return err
			}
		}

		if err := s.repo.TransferLeadership(txCtx, guildID, currentLeaderCharID, newLeaderCharID); err != nil {
			return err
		}
		s.touchActive(txCtx, guildID)
		return nil
	})
}

// AssignCustomRole sets a custom role title on a guild member (guild.cgi:ataeru).
// Only the guild leader can assign role titles.
// Transaction: RunInTx (Rank 2 target character lock, Rank 7 guild lock and title assignment; pending applicant approved atomically).
// Lock Order: characters(2) -> guilds(7) -> guild_members(7).
func (s *Service) AssignCustomRole(ctx context.Context, guildID string, requesterCharID string, targetCharID string, title string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	requesterCharID = strings.TrimSpace(requesterCharID)
	targetCharID = strings.TrimSpace(targetCharID)
	if requesterCharID == "" || targetCharID == "" {
		return ErrCharacterNotFound
	}

	normalizedTitle, err := ValidateRoleTitle(title)
	if err != nil {
		return err
	}

	var (
		g        Guild
		approved bool
	)
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Acquire character lock before guild lock
		if err := s.repo.LockCharacterForUpdate(txCtx, targetCharID); err != nil {
			if errors.Is(err, ErrCharacterNotFound) {
				return ErrTargetNotMember
			}
			return err
		}

		// Rank 7: Acquire guild lock
		lockedGuild, lockedMembers, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}

		var requester, target *Member
		for i := range lockedMembers {
			if lockedMembers[i].CharacterID == requesterCharID {
				requester = &lockedMembers[i]
			}
			if lockedMembers[i].CharacterID == targetCharID {
				target = &lockedMembers[i]
			}
		}

		if lockedGuild.LeaderCharacterID != requesterCharID || requester == nil || requester.Role != RoleLeader || requester.IsPending {
			return ErrUnauthorized
		}
		if target == nil {
			return ErrTargetNotMember
		}
		if target.Role == RoleLeader {
			return ErrCannotAssignToLeader
		}

		if target.IsPending {
			// Re-verify that the applicant is not already an active member of another guild (guild.cgi:ataeru)
			if activeGuild, _, err := s.repo.GetGuildByCharacter(txCtx, targetCharID); err == nil {
				if activeGuild.ID != guildID {
					return ErrCharacterAlreadyInGuild
				}
				return ErrMemberNotPending
			} else if !errors.Is(err, ErrCharacterNotInGuild) {
				return err
			}

			if err := s.repo.ApproveMember(txCtx, guildID, targetCharID, normalizedTitle); err != nil {
				return err
			}
			s.touchActive(txCtx, guildID)
			g = lockedGuild
			approved = true
			return nil
		}

		if err := s.repo.AssignCustomRole(txCtx, guildID, targetCharID, normalizedTitle); err != nil {
			return err
		}
		s.touchActive(txCtx, guildID)
		return nil
	})
	if err != nil {
		return err
	}

	// Send acceptance letter to applicant if pending was approved (guild.cgi:236-237)
	if approved && s.letterSender != nil && s.charReader != nil {
		leaderChar, errL := s.charReader.FindByID(ctx, requesterCharID)
		applicantChar, errA := s.charReader.FindByID(ctx, targetCharID)
		if errL == nil && errA == nil {
			content := "【＋参加許可証＋】" + g.Name + " (ギルマス " + leaderChar.Name + ") から参加許可をもらいました"
			//lint:ignore error-swallow existing best-effort membership notification
			_ = s.letterSender.SendLetter(ctx, requesterCharID, leaderChar.Name, targetCharID, applicantChar.Name, content, leaderChar.Color)
		}
	}

	return nil
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
	if len([]rune(notice)) > MaxNoticeLength {
		return ErrNoticeTooLong
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
