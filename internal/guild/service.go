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
	repo Repository
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("guild repository is nil")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) Create(ctx context.Context, creatorCharID string, name string) (Guild, Member, corecharacter.Character, error) {
	creatorCharID = strings.TrimSpace(creatorCharID)
	if creatorCharID == "" {
		return Guild{}, Member{}, corecharacter.Character{}, ErrCharacterNotFound
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > MaxNameLength {
		return Guild{}, Member{}, corecharacter.Character{}, ErrInvalidGuildName
	}

	// Verify creator is not currently in any guild
	if _, _, err := s.repo.GetGuildByCharacter(ctx, creatorCharID); err == nil {
		return Guild{}, Member{}, corecharacter.Character{}, ErrCharacterAlreadyInGuild
	}

	guildID := id.New()
	now := time.Now().UTC()
	g := Guild{
		ID:                guildID,
		Name:              name,
		LeaderCharacterID: creatorCharID,
		Points:            0,
		Notice:            "",
		Color:             DefaultColor,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	m := Member{
		GuildID:     guildID,
		CharacterID: creatorCharID,
		Role:        RoleLeader,
		Title:       DefaultTitleLeader,
		JoinedAt:    now,
	}

	return s.repo.CreateGuild(ctx, g, m, CreationFee)
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
	}

	_, _, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return Member{}, err
	}

	m := Member{
		GuildID:     guildID,
		CharacterID: characterID,
		Role:        RoleMember,
		Title:       "",
		JoinedAt:    time.Now().UTC(),
	}

	return s.repo.AddMember(ctx, m)
}

func (s *Service) Leave(ctx context.Context, guildID string, characterID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrCharacterNotFound
	}

	_, members, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return err
	}

	var currentMember *Member
	for i := range members {
		if members[i].CharacterID == characterID {
			currentMember = &members[i]
			break
		}
	}
	if currentMember == nil {
		return ErrCharacterNotInGuild
	}

	if currentMember.Role == RoleLeader {
		if len(members) > 1 {
			return ErrLeaderCannotLeaveWithMembers
		}
		// Sole member leaving disbands the guild
		return s.repo.DisbandGuild(ctx, guildID)
	}

	return s.repo.RemoveMember(ctx, guildID, characterID)
}

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

	_, members, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return err
	}

	var requester, target *Member
	for i := range members {
		if members[i].CharacterID == requesterCharID {
			requester = &members[i]
		}
		if members[i].CharacterID == targetCharID {
			target = &members[i]
		}
	}
	if requester == nil || requester.Role != RoleLeader {
		return ErrUnauthorized
	}
	if target == nil {
		return ErrTargetNotMember
	}
	if target.Role == RoleLeader {
		return ErrCannotKickLeader
	}

	return s.repo.RemoveMember(ctx, guildID, targetCharID)
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

	_, members, err := s.repo.GetGuild(ctx, guildID)
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

	return s.repo.TransferLeadership(ctx, guildID, currentLeaderCharID, newLeaderCharID)
}

// AssignCustomRole sets a custom role title on a guild member (guild.cgi:ataeru).
// Only the guild leader can assign role titles.
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

	if err := ValidateRoleTitle(title); err != nil {
		return err
	}

	_, members, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return err
	}

	var requester, target *Member
	for i := range members {
		if members[i].CharacterID == requesterCharID {
			requester = &members[i]
		}
		if members[i].CharacterID == targetCharID {
			target = &members[i]
		}
	}

	if requester == nil || requester.Role != RoleLeader {
		return ErrUnauthorized
	}
	if target == nil {
		return ErrTargetNotMember
	}
	if target.Role == RoleLeader {
		return ErrCannotAssignToLeader
	}

	return s.repo.AssignCustomRole(ctx, guildID, targetCharID, title)
}

// UpdateColor changes the guild's hex color (guild.cgi:color).
// Only the guild leader can change color.
// Validates hex format, NPC color prohibition, and server-wide uniqueness.
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

	g, members, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return err
	}

	var requester *Member
	for i := range members {
		if members[i].CharacterID == requesterCharID {
			requester = &members[i]
			break
		}
	}
	if requester == nil || requester.Role != RoleLeader {
		return ErrUnauthorized
	}

	// Legacy rule: White (#FFFFFF) is allowed and can be shared, but non-white colors must be unique.
	// NPC color (#FF69B4) is reserved and cannot be selected.
	if normalizedColor == NPCColor {
		return ErrColorTaken
	}

	if normalizedColor != DefaultColor {
		taken, err := s.repo.IsColorTaken(ctx, normalizedColor, g.ID)
		if err != nil {
			return err
		}
		if taken {
			return ErrColorTaken
		}
	}

	return s.repo.UpdateColor(ctx, guildID, normalizedColor)
}

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

	_, member, err := s.repo.GetGuildByCharacter(ctx, requesterCharID)
	if err != nil {
		return err
	}
	if member.GuildID != guildID || member.Role != RoleLeader {
		return ErrUnauthorized
	}

	return s.repo.UpdateNotice(ctx, guildID, notice)
}

// AddPoints increments guild points directly by guild ID.
func (s *Service) AddPoints(ctx context.Context, guildID string, points int64) error {
	if guildID == "" || points <= 0 {
		return nil
	}
	return s.repo.AddPoints(ctx, guildID, points)
}

// AddGuildPoints awards guild points if the character is in a guild.
func (s *Service) AddGuildPoints(ctx context.Context, characterID string, points int) error {
	if characterID == "" || points <= 0 {
		return nil
	}
	return s.repo.AddGuildPoints(ctx, characterID, points)
}

func (s *Service) Disband(ctx context.Context, guildID string, leaderCharID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	leaderCharID = strings.TrimSpace(leaderCharID)
	if leaderCharID == "" {
		return ErrCharacterNotFound
	}

	g, member, err := s.repo.GetGuildByCharacter(ctx, leaderCharID)
	if err != nil {
		return err
	}
	if g.ID != guildID || member.Role != RoleLeader {
		return ErrUnauthorized
	}

	return s.repo.DisbandGuild(ctx, guildID)
}
