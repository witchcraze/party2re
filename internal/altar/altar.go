package altar

import (
	"context"
	"errors"
	"slices"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

const (
	// RamiaStayDuration is 30 minutes, reproducing legacy reborn.cgi ($time + 1800).
	RamiaStayDuration = 30 * time.Minute

	// DefaultMaxInventoryCapacity is 1, matching legacy $m{ite} single slot check.
	DefaultMaxInventoryCapacity = 1

	// Otherworld travel item definition IDs (legacy items 66..69).
	ItemMirrorOfTruth  = "item-066" // 真実の鏡 (ラーの鏡)
	ItemMadamsInvite   = "item-067" // マダムの招待状
	ItemTreasureMap    = "item-068" // 宝の地図
	ItemLampOfDarkness = "item-069" // 闇のランプ

)

// Legacy dialogue & messages from reborn.cgi
const (
	MsgOrbsInsufficient = "オーブが足りません。オーブが足りません。オーブを６つ集めてください"
	MsgRamiaAwakening   = "時は来たれり。今こそ目覚める時。大空はお前のもの。舞い上がれ空高く！"
	MsgWishInventoryFmt = "%s ですね。冒険中に使うことで未知の世界へと行くことができるでしょう"
	MsgWishDepotFmt     = "%s を%sの預かり所に送っておきました。冒険中に使うことで未知の世界へと行くことができるでしょう"
)

var (
	ErrCharacterNotFound = errors.New("character not found")
	ErrInsufficientOrbs  = errors.New(MsgOrbsInsufficient)
	ErrRamiaNotAwakened  = errors.New("Ramia is not awakened; cannot make a wish")
	ErrInvalidWishItem   = errors.New("invalid travel item chosen for wish")
	ErrRamiaAwakened     = errors.New("Ramia is already awakened; make a wish instead")
	ErrNilDependency     = errors.New("required dependency is nil")
)

var AllowedWishItems = []string{
	ItemMirrorOfTruth,
	ItemMadamsInvite,
	ItemTreasureMap,
	ItemLampOfDarkness,
}

func IsValidWishItem(itemID string) bool {
	return slices.Contains(AllowedWishItems, itemID)
}

// NPCName is the altar's shrine maiden (reborn.cgi $npc_name).
const NPCName = "@巫女"

// RamiaMemberName is Ramia's place-member name while she is present.
const RamiaMemberName = "ラーミァ@"

// reborn.cgi @words, chosen by the character's orb state.
var (
	mikoGuardingWords = []string{
		"私達は。私達は。卵を守っています。卵を守っています", "世界中にちらばる６つのオーブをささげたとき....",
		"伝説の不死鳥ラーミァはよみがえりましょう", "手に入れられるオーブは曜日によって変わるようです",
	}
	mikoReadyWords    = []string{"私達。私達。この日をどんなに。この日をどんなに。待ちわびたことでしょう", "さぁ、祈りましょう。さぁ、祈りましょう"}
	mikoAwakenedWords = []string{
		"このアイテムを冒険中に使えば、今まで見たことのない世界へと行くことができます",
		"未知の世界に連れて行くことができるのは、アイテムを使った一回だけです",
		"未知の世界にへは、その場にいた仲間と一緒に行くことができます",
	}
)

// MikoWords returns the lines the shrine maiden may say to char.
func MikoWords(char corecharacter.Character) []string {
	switch {
	case char.IsRamiaAwakened():
		return mikoAwakenedWords
	case char.HasAllOrbs():
		return mikoReadyWords
	default:
		return mikoGuardingWords
	}
}

// RamiaAwakening represents a persistent record of Ramia being awakened at the altar.
type RamiaAwakening struct {
	ID            string    `json:"id"`
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	AwakenedAt    time.Time `json:"awakened_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (a *RamiaAwakening) IsActive(now time.Time) bool {
	if a == nil {
		return false
	}
	return now.Before(a.ExpiresAt)
}

// AltarStatus represents the state of the Altar of Rebirth queried by a character.
type AltarStatus struct {
	CharacterID    string         `json:"character_id"`
	Orbs           string         `json:"orbs"`
	OrbCount       int            `json:"orb_count"`
	HasAllOrbs     bool           `json:"has_all_orbs"`
	RamiaAwakened  bool           `json:"ramia_awakened"`
	RamiaPresent   bool           `json:"ramia_present"`
	RamiaExpiresAt *time.Time     `json:"ramia_expires_at,omitempty"`
	AwakenedBy     string         `json:"awakened_by,omitempty"`
	AvailableItems []WishItemInfo `json:"available_items"`
}

type WishItemInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PrayResult struct {
	CharacterID   string    `json:"character_id"`
	Message       string    `json:"message"`
	RamiaAwakened bool      `json:"ramia_awakened"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type WishResult struct {
	CharacterID string `json:"character_id"`
	ItemID      string `json:"item_id"`
	ItemName    string `json:"item_name"`
	DeliveredTo string `json:"delivered_to"` // "inventory" or "depot"
	Message     string `json:"message"`
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inv coreinventory.Inventory) error
}

type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, dep depot.Depot) error
}

type AltarRepository interface {
	SaveAwakening(ctx context.Context, awakening RamiaAwakening) error
	GetLatestAwakening(ctx context.Context) (*RamiaAwakening, error)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type ItemDefinitionProvider interface {
	FindByID(id string) (coreitem.Definition, error)
}

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
}

type Option func(*Service)

func WithMaxInventoryCapacity(cap int) Option {
	return func(s *Service) {
		if cap > 0 {
			s.maxInvCap = cap
		}
	}
}

func WithCollectionRecorder(recorder CollectionRecorder) Option {
	return func(s *Service) {
		s.collector = recorder
	}
}

func WithNowFunc(fn func() time.Time) Option {
	return func(s *Service) {
		if fn != nil {
			s.nowFunc = fn
		}
	}
}

type Service struct {
	chars      CharacterRepository
	invs       InventoryRepository
	depots     DepotRepository
	altars     AltarRepository
	itemDefs   ItemDefinitionProvider
	txProvider TransactionProvider
	collector  CollectionRecorder
	maxInvCap  int
	nowFunc    func() time.Time
}

func NewService(
	chars CharacterRepository,
	invs InventoryRepository,
	depots DepotRepository,
	altars AltarRepository,
	itemDefs ItemDefinitionProvider,
	txProvider TransactionProvider,
	opts ...Option,
) (*Service, error) {
	if chars == nil || invs == nil || depots == nil || altars == nil || itemDefs == nil || txProvider == nil {
		return nil, ErrNilDependency
	}
	s := &Service{
		chars:      chars,
		invs:       invs,
		depots:     depots,
		altars:     altars,
		itemDefs:   itemDefs,
		txProvider: txProvider,
		maxInvCap:  DefaultMaxInventoryCapacity,
		nowFunc:    time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}
