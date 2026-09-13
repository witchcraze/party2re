package guild

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

type CharacterReader interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type LetterSender interface {
	SendLetter(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error
}

type ServiceOption func(*Service)

func WithCharacterReader(cr CharacterReader) ServiceOption {
	return func(s *Service) {
		s.charReader = cr
	}
}

func WithLetterSender(ls LetterSender) ServiceOption {
	return func(s *Service) {
		s.letterSender = ls
	}
}

func WithClock(clock func() time.Time) ServiceOption {
	return func(s *Service) {
		s.nowFunc = clock
	}
}

type Service struct {
	repo         Repository
	charReader   CharacterReader
	letterSender LetterSender
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
	now := s.nowFunc().UTC()
	g := Guild{
		ID:                guildID,
		Name:              name,
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
		IsPending:   false,
		JoinedAt:    s.nowFunc().UTC(),
	}

	res, err := s.repo.AddMember(ctx, m)
	if err != nil {
		return Member{}, err
	}
	_ = s.repo.TouchActive(ctx, guildID)
	return res, nil
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

	if err := s.repo.RemoveMember(ctx, guildID, characterID); err != nil {
		return err
	}
	_ = s.repo.TouchActive(ctx, guildID)
	return nil
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

	g, members, err := s.repo.GetGuild(ctx, guildID)
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

	if target.IsPending {
		return s.RejectApplication(ctx, guildID, requesterCharID, targetCharID)
	}

	if err := s.repo.RemoveMember(ctx, guildID, targetCharID); err != nil {
		return err
	}
	_ = s.repo.TouchActive(ctx, guildID)

	if s.letterSender != nil && s.charReader != nil {
		leaderChar, errL := s.charReader.FindByID(ctx, requesterCharID)
		targetChar, errT := s.charReader.FindByID(ctx, targetCharID)
		if errL == nil && errT == nil {
			content := "【＋追放＋】" + g.Name + " (ギルマス " + leaderChar.Name + ") から追放されました"
			_ = s.letterSender.SendLetter(ctx, requesterCharID, leaderChar.Name, targetCharID, targetChar.Name, content, leaderChar.Color)
		}
	}

	return nil
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

	if err := s.repo.TransferLeadership(ctx, guildID, currentLeaderCharID, newLeaderCharID); err != nil {
		return err
	}
	_ = s.repo.TouchActive(ctx, guildID)
	return nil
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

	if target.IsPending {
		return s.ApproveApplication(ctx, guildID, requesterCharID, targetCharID, title)
	}

	if err := s.repo.AssignCustomRole(ctx, guildID, targetCharID, title); err != nil {
		return err
	}
	_ = s.repo.TouchActive(ctx, guildID)
	return nil
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

	if err := s.repo.UpdateColor(ctx, guildID, normalizedColor); err != nil {
		return err
	}
	_ = s.repo.TouchActive(ctx, guildID)
	return nil
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

	if err := s.repo.UpdateNotice(ctx, guildID, notice); err != nil {
		return err
	}
	_ = s.repo.TouchActive(ctx, guildID)
	return nil
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
