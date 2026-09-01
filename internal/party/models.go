package party

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

const (
	MaxPartyMembers = 4
	MinPartyMembers = 1
	MinPartyNameLen = 1
	MaxPartyNameLen = 50

	StatusRecruiting = "recruiting"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusDisbanded  = "disbanded"

	DefaultSpeed = 3
)

var (
	ErrNotFound               = errors.New("party not found")
	ErrAlreadyInParty         = errors.New("character is already in an active party")
	ErrPartyFull              = errors.New("party is already full")
	ErrPartyNotRecruiting     = errors.New("party is not currently recruiting")
	ErrInvalidPassword        = errors.New("invalid party password")
	ErrLevelRequirementNotMet = errors.New("character level does not meet party requirement")
	ErrHPRequirementNotMet    = errors.New("character max HP does not meet party requirement")
	ErrNotPartyLeader         = errors.New("only the party leader can perform this action")
	ErrCannotKickSelf         = errors.New("party leader cannot kick themselves")
	ErrCharacterNotInParty    = errors.New("character is not a member of this party")
	ErrPartyNotReady          = errors.New("all party members must be ready before starting")
	ErrInvalidPartyName       = errors.New("party name must be between 1 and 50 characters")
	ErrInvalidMaxMembers      = errors.New("max members must be between 1 and 4")
	ErrCharacterUnconscious   = errors.New("character is unconscious (HP <= 0) and cannot adventure")
	ErrStageNotFound          = errors.New("adventure stage not found")
	ErrForbidden              = errors.New("forbidden: character does not belong to player")
)

type Party struct {
	ID                string    `json:"id"`
	LeaderCharacterID string    `json:"leader_character_id"`
	Name              string    `json:"name"`
	PasswordHash      string    `json:"-"`
	StageID           string    `json:"stage_id"`
	Speed             int       `json:"speed"`
	MaxMembers        int       `json:"max_members"`
	MinLevel          int       `json:"min_level"`
	MaxLevel          int       `json:"max_level"`
	MinHP             int       `json:"min_hp"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Member struct {
	PartyID       string    `json:"party_id"`
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	JobID         string    `json:"job_id"`
	Level         int       `json:"level"`
	HP            int       `json:"hp"`
	MaxHP         int       `json:"max_hp"`
	IsLeader      bool      `json:"is_leader"`
	ReadyState    bool      `json:"ready_state"`
	JoinedAt      time.Time `json:"joined_at"`
}

type PartyDetail struct {
	Party   Party    `json:"party"`
	Members []Member `json:"members"`
}

type PartySummary struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	LeaderCharacterID string    `json:"leader_character_id"`
	LeaderName        string    `json:"leader_name"`
	StageID           string    `json:"stage_id"`
	StageName         string    `json:"stage_name"`
	Speed             int       `json:"speed"`
	CurrentMembers    int       `json:"current_members"`
	MaxMembers        int       `json:"max_members"`
	HasPassword       bool      `json:"has_password"`
	MinLevel          int       `json:"min_level"`
	MaxLevel          int       `json:"max_level"`
	MinHP             int       `json:"min_hp"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

type CreatePartyRequest struct {
	Name       string `json:"name"`
	StageID    string `json:"stage_id"`
	Password   string `json:"password,omitempty"`
	Speed      int    `json:"speed,omitempty"`
	MaxMembers int    `json:"max_members,omitempty"`
	MinLevel   int    `json:"min_level,omitempty"`
	MaxLevel   int    `json:"max_level,omitempty"`
	MinHP      int    `json:"min_hp,omitempty"`
}

type MemberRewardSummary struct {
	CharacterID string              `json:"character_id"`
	Name        string              `json:"name"`
	GainedEXP   int                 `json:"gained_exp"`
	GainedGold  int                 `json:"gained_gold"`
	LevelBefore int                 `json:"level_before"`
	LevelAfter  int                 `json:"level_after"`
	Drops       []coreitem.Instance `json:"drops,omitempty"`
}

type PartyAdventureResult struct {
	PartyID             string                   `json:"party_id"`
	StageID             string                   `json:"stage_id"`
	Outcome             string                   `json:"outcome"`
	Turns               int                      `json:"turns"`
	TotalEXP            int                      `json:"total_exp"`
	TotalGold           int                      `json:"total_gold"`
	SynergyBonusPercent int                      `json:"synergy_bonus_percent"`
	Rewards             []MemberRewardSummary    `json:"rewards"`
	BattleResult        battle.PartyBattleResult `json:"battle_result"`
}

type PartyAdventureLog struct {
	ID                  string    `json:"id"`
	PartyID             string    `json:"party_id"`
	StageID             string    `json:"stage_id"`
	Outcome             string    `json:"outcome"`
	Turns               int       `json:"turns"`
	TotalEXP            int       `json:"total_exp"`
	TotalGold           int       `json:"total_gold"`
	SynergyBonusPercent int       `json:"synergy_bonus_percent"`
	DetailsJSON         string    `json:"details_json,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// ValidatePartyName verifies party name constraints.
func ValidatePartyName(name string) error {
	trimmed := strings.TrimSpace(name)
	runes := utf8.RuneCountInString(trimmed)
	if runes < MinPartyNameLen || runes > MaxPartyNameLen {
		return ErrInvalidPartyName
	}
	return nil
}

// Repository defines persistence operations for parties and members.
type Repository interface {
	SaveParty(ctx context.Context, p Party) error
	GetParty(ctx context.Context, id string) (Party, error)
	GetPartyForUpdate(ctx context.Context, id string) (Party, error)
	ListParties(ctx context.Context, status string, limit, offset int) ([]PartySummary, error)
	UpdateParty(ctx context.Context, p Party) error
	DeleteParty(ctx context.Context, id string) error

	AddMember(ctx context.Context, m Member) error
	GetMembers(ctx context.Context, partyID string) ([]Member, error)
	GetMember(ctx context.Context, partyID, characterID string) (Member, error)
	GetActivePartyByCharacter(ctx context.Context, characterID string) (Party, Member, error)
	RemoveMember(ctx context.Context, partyID, characterID string) error
	UpdateMemberReady(ctx context.Context, partyID, characterID string, ready bool) error
	CountMembers(ctx context.Context, partyID string) (int, error)

	SaveAdventureLog(ctx context.Context, log PartyAdventureLog) error
}

// CharacterRepository defines character persistence required by Party service.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

// InventoryRepository defines inventory item persistence.
type InventoryRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

// StageProvider resolves stage definitions.
type StageProvider interface {
	FindByID(id string) (adventure.Stage, error)
}

// MonsterProvider resolves monster definitions.
type MonsterProvider interface {
	FindByID(id string) (adventure.Monster, error)
}

// BattleEngine resolves multiplayer party combat.
type BattleEngine interface {
	ResolvePartyBattle(req battle.PartyBattleRequest) (battle.PartyBattleResult, error)
}

// NewsPublisher broadcasts announcements.
type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

// TransactionProvider executes atomic transactional operations.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error
}
