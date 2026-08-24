package guild

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type Role string

const (
	RoleLeader  Role = "leader"
	RoleOfficer Role = "officer"
	RoleMember  Role = "member"
)

func (r Role) Valid() bool {
	switch r {
	case RoleLeader, RoleOfficer, RoleMember:
		return true
	default:
		return false
	}
}

const (
	CreationFee      = 5000 // Gold required to create a guild (standard reference value)
	BaseCapacity     = 10   // Base member capacity at Level 1
	CapacityPerLevel = 2    // Capacity increase per level
	MaxLevel         = 10   // Maximum guild level
	MaxNameLength    = 32   // Maximum characters for guild name
	MaxNoticeLength  = 200  // Maximum characters for guild notice
)

var (
	ErrInvalidGuildID               = errors.New("invalid guild ID")
	ErrInvalidGuildName             = errors.New("guild name must be between 1 and 32 characters")
	ErrNoticeTooLong                = errors.New("guild notice exceeds maximum allowed length")
	ErrGuildNotFound                = errors.New("guild not found")
	ErrGuildNameTaken               = errors.New("guild name is already taken")
	ErrCharacterNotFound            = errors.New("character not found")
	ErrCharacterAlreadyInGuild      = errors.New("character is already a member of a guild")
	ErrCharacterNotInGuild          = errors.New("character is not a member of this guild")
	ErrInsufficientFunds            = errors.New("character does not have enough gold")
	ErrGuildFull                    = errors.New("guild has reached maximum member capacity")
	ErrUnauthorized                 = errors.New("unauthorized to perform this action in the guild")
	ErrCannotKickLeader             = errors.New("cannot kick the guild leader")
	ErrCannotKickEqualOrHigherRole  = errors.New("officers cannot kick other officers or the leader")
	ErrLeaderCannotLeaveWithMembers = errors.New("guild leader cannot leave while other members remain; transfer leadership or disband")
	ErrInvalidDonationAmount        = errors.New("donation amount must be positive")
	ErrTargetNotMember              = errors.New("target character is not a member of the guild")
	ErrCannotDemoteLeader           = errors.New("cannot demote leader; transfer leadership instead")
	ErrInvalidRole                  = errors.New("invalid role")
)

type Guild struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	LeaderCharacterID string    `json:"leader_character_id"`
	Level             int       `json:"level"`
	Exp               int64     `json:"exp"`
	Gold              int64     `json:"gold"`
	Notice            string    `json:"notice"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Capacity returns the member capacity based on the guild's level.
func (g Guild) Capacity() int {
	lvl := g.Level
	if lvl < 1 {
		lvl = 1
	}
	return BaseCapacity + (lvl-1)*CapacityPerLevel
}

// ExpForLevel returns the cumulative exp required to reach level L.
// Formula: Level 1 requires 0 exp. Level L (L >= 2) requires (L - 1) * (L - 1) * 10000 exp.
func ExpForLevel(lvl int) int64 {
	if lvl <= 1 {
		return 0
	}
	diff := int64(lvl - 1)
	return diff * diff * 10000
}

// CalculateLevel returns the guild level for a given cumulative EXP.
func CalculateLevel(exp int64) int {
	lvl := 1
	for lvl < MaxLevel {
		nextReq := ExpForLevel(lvl + 1)
		if exp < nextReq {
			break
		}
		lvl++
	}
	return lvl
}

type Member struct {
	GuildID          string    `json:"guild_id"`
	CharacterID      string    `json:"character_id"`
	Role             Role      `json:"role"`
	JoinedAt         time.Time `json:"joined_at"`
	TotalDonatedGold int64     `json:"total_donated_gold"`
}

type Detail struct {
	Guild   Guild    `json:"guild"`
	Members []Member `json:"members"`
}

type Repository interface {
	CreateGuild(ctx context.Context, g Guild, creator Member, fee int) (Guild, Member, corecharacter.Character, error)
	GetGuild(ctx context.Context, guildID string) (Guild, []Member, error)
	GetGuildByCharacter(ctx context.Context, characterID string) (Guild, Member, error)
	ListGuilds(ctx context.Context, offset, limit int) ([]Guild, error)
	AddMember(ctx context.Context, member Member) (Member, error)
	RemoveMember(ctx context.Context, guildID string, characterID string) error
	TransferLeadership(ctx context.Context, guildID string, oldLeaderCharID string, newLeaderCharID string) error
	UpdateMemberRole(ctx context.Context, guildID string, targetCharID string, newRole Role) error
	UpdateNotice(ctx context.Context, guildID string, notice string) error
	Donate(ctx context.Context, guildID string, characterID string, amount int, newLevel int, newExp int64) (Guild, Member, corecharacter.Character, error)
	DisbandGuild(ctx context.Context, guildID string) error
}

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

	guildID, err := generateID()
	if err != nil {
		return Guild{}, Member{}, corecharacter.Character{}, fmt.Errorf("generate guild id: %w", err)
	}

	now := time.Now().UTC()
	g := Guild{
		ID:                guildID,
		Name:              name,
		LeaderCharacterID: creatorCharID,
		Level:             1,
		Exp:               0,
		Gold:              0,
		Notice:            "",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	m := Member{
		GuildID:          guildID,
		CharacterID:      creatorCharID,
		Role:             RoleLeader,
		JoinedAt:         now,
		TotalDonatedGold: 0,
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

	g, members, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return Member{}, err
	}

	if len(members) >= g.Capacity() {
		return Member{}, ErrGuildFull
	}

	m := Member{
		GuildID:          guildID,
		CharacterID:      characterID,
		Role:             RoleMember,
		JoinedAt:         time.Now().UTC(),
		TotalDonatedGold: 0,
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
	if requester == nil {
		return ErrUnauthorized
	}
	if target == nil {
		return ErrTargetNotMember
	}

	if target.Role == RoleLeader {
		return ErrCannotKickLeader
	}

	switch requester.Role {
	case RoleLeader:
		// Leader can kick officer and member
	case RoleOfficer:
		if target.Role != RoleMember {
			return ErrCannotKickEqualOrHigherRole
		}
	default:
		return ErrUnauthorized
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

func (s *Service) UpdateRole(ctx context.Context, guildID string, requesterCharID string, targetCharID string, newRole Role) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	requesterCharID = strings.TrimSpace(requesterCharID)
	targetCharID = strings.TrimSpace(targetCharID)
	if requesterCharID == "" || targetCharID == "" {
		return ErrCharacterNotFound
	}
	if !newRole.Valid() || newRole == RoleLeader {
		return ErrInvalidRole
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
		return ErrCannotDemoteLeader
	}

	return s.repo.UpdateMemberRole(ctx, guildID, targetCharID, newRole)
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
	if member.GuildID != guildID {
		return ErrUnauthorized
	}
	if member.Role != RoleLeader && member.Role != RoleOfficer {
		return ErrUnauthorized
	}

	return s.repo.UpdateNotice(ctx, guildID, notice)
}

func (s *Service) Donate(ctx context.Context, guildID string, characterID string, amount int) (Guild, Member, corecharacter.Character, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return Guild{}, Member{}, corecharacter.Character{}, ErrInvalidGuildID
	}
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return Guild{}, Member{}, corecharacter.Character{}, ErrCharacterNotFound
	}
	if amount <= 0 {
		return Guild{}, Member{}, corecharacter.Character{}, ErrInvalidDonationAmount
	}

	g, member, err := s.repo.GetGuildByCharacter(ctx, characterID)
	if err != nil {
		return Guild{}, Member{}, corecharacter.Character{}, err
	}
	if member.GuildID != guildID {
		return Guild{}, Member{}, corecharacter.Character{}, ErrCharacterNotInGuild
	}

	newExp := g.Exp + int64(amount)
	newLevel := CalculateLevel(newExp)

	return s.repo.Donate(ctx, guildID, characterID, amount, newLevel, newExp)
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

func generateID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
