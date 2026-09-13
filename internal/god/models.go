package god

import (
	"context"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/guild"
)

var (
	ErrNilDependency      = errors.New("god dependency is nil")
	ErrCharacterNotFound  = errors.New("character not found")
	ErrInvalidCharacterID = errors.New("invalid character ID")
	ErrInvalidWishID      = errors.New("invalid wish ID")
	ErrInvalidRealm       = errors.New("invalid realm, must be 'heaven' or 'underworld'")
	ErrWishNotFound       = errors.New("wish not found in catalog")
	ErrWishRequirement    = errors.New("character does not meet requirements for this wish")
	ErrLimitBreakMaxed    = errors.New("limit break is already at maximum tier")
)

type Realm string

const (
	RealmHeaven     Realm = "heaven"
	RealmUnderworld Realm = "underworld"
)

const (
	MaxLimitBreakTier = 5
)

// Legacy Heaven Wish IDs (party2/lib/god.cgi)
const (
	WishStats             = "wish_stats"
	WishSP                = "wish_sp"
	WishMoney             = "wish_money"
	WishCasinoCoins       = "wish_casino_coins"
	WishSmallMedals       = "wish_small_medals"
	WishLotteryTickets    = "wish_lottery_tickets"
	WishGuildRank         = "wish_guild_rank"
	WishGuildGorgeous     = "wish_guild_gorgeous"
	WishRefresh           = "wish_refresh"
	WishFullRecovery      = "wish_full_recovery" // alias for wish_refresh
	WishAllOrbs           = "wish_all_orbs"
	WishCelestialDragon   = "wish_celestial_dragon"
	WishGodOfNewWorld     = "wish_god_of_new_world"
	WishOrtega            = "wish_ortega"
	WishCat               = "wish_cat"
	WishEroticBook        = "wish_erotic_book"
	WishAlchemyRecipe     = "wish_alchemy_recipe"
	WishLover             = "wish_lover"
	WishSecretMaid        = "wish_secret_maid"
	WishLimitBreakLevel   = "wish_limit_break_level"
	WishRestoreLevelLimit = "wish_restore_level_limit"
)

// Underworld Wish IDs
const (
	WishExpandDepot      = "wish_expand_depot"
	WishExpandMonster    = "wish_expand_monster"
	WishExpandJobMemory  = "wish_expand_job_memory"
	WishExpandFleaMarket = "wish_expand_flea_market"
	WishExpandShopStore  = "wish_expand_shop_store"
)

// Wish represents a wish available in Heaven or Underworld.
type Wish struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Realm       Realm  `json:"realm"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	CurrentTier int    `json:"current_tier,omitempty"`
	MaxTier     int    `json:"max_tier,omitempty"`
	Note        string `json:"note,omitempty"`
}

// WishResult represents the outcome of having a wish granted.
type WishResult struct {
	Character    corecharacter.Character `json:"character"`
	Wish         Wish                    `json:"wish"`
	Message      string                  `json:"message"`
	NPCSpeech    string                  `json:"npc_speech"`
	NextLocation string                  `json:"next_location,omitempty"`
}

// HomeMember represents a companion/resident at the player's home.
type HomeMember struct {
	ID          string    `json:"id"`
	CharacterID string    `json:"character_id"`
	IsNPC       bool      `json:"is_npc"`
	Name        string    `json:"name"`
	Icon        string    `json:"icon"`
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"created_at"`
}

// CharacterRepository defines character persistence methods required by the god service.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// DepotRepository defines depot persistence methods for capacity adjustments.
type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, dep depot.Depot) error
}

// InventoryRepository defines inventory persistence methods.
type InventoryRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inv coreinventory.Inventory) error
}

// TransactionProvider executes operations within a database transaction.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// CasinoRepository provides casino account operations for wishes.
type CasinoRepository interface {
	AdjustCoins(ctx context.Context, characterID string, delta int64) (casino.Account, error)
}

// LotteryRepository provides lottery operations for wishes.
type LotteryRepository interface {
	AddRaffleTickets(ctx context.Context, characterID string, count int) (int, error)
}

// GuildRepository provides guild operations for wishes.
type GuildRepository interface {
	GetGuildByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error)
	AddPoints(ctx context.Context, guildID string, points int64) error
	UpdateBgimg(ctx context.Context, guildID string, bgimg string) error
}

// HomeMemberRepository manages home members.
type HomeMemberRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) ([]HomeMember, error)
	AddMember(ctx context.Context, member HomeMember) error
}

// ProfileRepository provides avatar updates for wishes.
type ProfileRepository interface {
	UpdateAvatar(ctx context.Context, characterID string, avatarURL string) error
}

// HomeRepository provides home background updates for wishes.
type HomeRepository interface {
	UpdateBgimg(ctx context.Context, characterID string, bgimg string) error
}
