package secretshop

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	MinAccessJobLevel   = 7
	MaxPurchaseQuantity = 99
	NPCName             = "@ヒミツジ"
	LocationName        = "秘密の店"
)

var (
	ErrNilDependency                = errors.New("secret shop dependency is nil")
	ErrCharacterNotFound            = errors.New("character not found")
	ErrAccessDenied                 = errors.New("secret shop access denied: character does not meet discovery requirements")
	ErrItemNotFound                 = errors.New("item not found in secret shop catalog")
	ErrItemUnavailableInHelperQuest = errors.New("item is temporarily unavailable due to active helper request")
	ErrInsufficientFunds            = errors.New("insufficient funds to purchase secret shop item")
	ErrInvalidQuantity              = errors.New("invalid purchase quantity")
	ErrPriceOverflow                = economy.ErrGoldOverflow
	ErrDepotFull                    = depot.ErrDepotFull
	ErrDepotNotConfigured           = errors.New("depot repository not configured")
)

var DefaultTalkDialogues = []string{
	"バレちゃったメェ〜。他の人には秘密だメェ〜。",
	"値段は高いメェ〜けれど、他では手に入らないレアものだメェ〜。",
	"メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜。",
	"ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜。",
	"＠ぱふぱふはサービスだメェ〜。",
}

const (
	InspectDialogue        = "@ヒミツジ「オイラは羊の@ヒミツジだメェ〜。羊の国から来たよ…ゴホッゴホッ…羊の国から来たメェ〜」"
	PuffPuffDialogue       = "パフパフ♥ パフパフ♥ パフパフ♥ ……… どうだ わしのパフパフは気持ちいいだろう"
	PuffPuffDialogueFormat = "パフパフ♥ パフパフ♥ パフパフ♥ ……… どうだ %s わしのパフパフは気持ちいいだろう"
)

// FormatPuffPuffMessage formats the puff-puff dialogue with the character name, matching legacy CGI (secret.cgi).
func FormatPuffPuffMessage(charName string) string {
	if strings.TrimSpace(charName) == "" {
		return PuffPuffDialogue
	}
	return fmt.Sprintf(PuffPuffDialogueFormat, charName)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, value depot.Depot) error
}

type ItemDefinitionProvider interface {
	FindByID(id string) (coreitem.Definition, error)
}

type HelperQuestFilter interface {
	GetActiveHelperItemIDs(ctx context.Context) ([]string, error)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithHelperFilter(filter HelperQuestFilter) Option {
	return func(s *Service) {
		s.helperFilter = filter
	}
}

func WithDepotRepository(repo DepotRepository) Option {
	return func(s *Service) {
		s.depotRepo = repo
	}
}

func WithItemDefinitionProvider(provider ItemDefinitionProvider) Option {
	return func(s *Service) {
		s.itemDefProvider = provider
	}
}

func WithCollectionRecorder(recorder CollectionRecorder) Option {
	return func(s *Service) {
		s.collectionRecorder = recorder
	}
}

type Service struct {
	characterRepo      CharacterRepository
	inventoryRepo      InventoryRepository
	catalog            *Catalog
	helperFilter       HelperQuestFilter
	txProvider         TransactionProvider
	economy            *economy.Service
	depotRepo          DepotRepository
	itemDefProvider    ItemDefinitionProvider
	collectionRecorder CollectionRecorder
}

func (s *Service) SetCollectionRecorder(recorder CollectionRecorder) {
	s.collectionRecorder = recorder
}

func NewService(
	characterRepo CharacterRepository,
	inventoryRepo InventoryRepository,
	catalog *Catalog,
	opts ...Option,
) (*Service, error) {
	if characterRepo == nil {
		return nil, fmt.Errorf("%w: character repository", ErrNilDependency)
	}
	if inventoryRepo == nil {
		return nil, fmt.Errorf("%w: inventory repository", ErrNilDependency)
	}
	if catalog == nil {
		return nil, fmt.Errorf("%w: catalog", ErrNilDependency)
	}

	s := &Service{
		characterRepo: characterRepo,
		inventoryRepo: inventoryRepo,
		catalog:       catalog,
	}
	for _, opt := range opts {
		opt(s)
	}
	var ecoOpts []economy.Option
	if s.txProvider != nil {
		ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
	}
	eco, err := economy.NewService(characterRepo, inventoryRepo, ecoOpts...)
	if err != nil {
		return nil, err
	}
	s.economy = eco
	return s, nil
}

// ShopStatus contains current secret shop status for a character.
type ShopStatus struct {
	CharacterID  string `json:"character_id"`
	LocationName string `json:"location_name"`
	NPCName      string `json:"npc_name"`
	IsEligible   bool   `json:"is_eligible"`
	Items        []Item `json:"items"`
}

// PuffPuffResult contains the result of the NPC puff-puff interaction.
// Under legacy parity (secret.cgi), puff-puff provides flavor dialogue only with no healing.
type PuffPuffResult struct {
	CharacterID string `json:"character_id"`
	NPCName     string `json:"npc_name"`
	Message     string `json:"message"`
}

// CheckEligibility returns true if the character meets secret shop discovery qualifications.
// The original CGI (item.cgi:himitsunomise) is accessible only when job_lv >= 7.
func CheckEligibility(c corecharacter.Character) bool {
	return c.JobLevel >= MinAccessJobLevel
}

// GetShopStatus checks access and returns available secret shop items.
func (s *Service) GetShopStatus(ctx context.Context, characterID string) (*ShopStatus, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	if !CheckEligibility(char) {
		return nil, ErrAccessDenied
	}

	items, err := s.getAvailableItems(ctx)
	if err != nil {
		return nil, err
	}

	return &ShopStatus{
		CharacterID:  char.ID,
		LocationName: LocationName,
		NPCName:      NPCName,
		IsEligible:   true,
		Items:        items,
	}, nil
}

// Talk returns a secretive sheep dialogue from @ヒミツジ.
func (s *Service) Talk(ctx context.Context, characterID string) (string, error) {
	if strings.TrimSpace(characterID) == "" {
		return "", ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return "", ErrCharacterNotFound
	}

	if !CheckEligibility(char) {
		return "", ErrAccessDenied
	}

	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(DefaultTalkDialogues))))
	if err != nil {
		return DefaultTalkDialogues[0], nil
	}
	return DefaultTalkDialogues[nBig.Int64()], nil
}

// Inspect returns NPC background description.
func (s *Service) Inspect(ctx context.Context, characterID string) (string, error) {
	if strings.TrimSpace(characterID) == "" {
		return "", ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return "", ErrCharacterNotFound
	}

	if !CheckEligibility(char) {
		return "", ErrAccessDenied
	}

	return InspectDialogue, nil
}

// PuffPuff provides the playful secret puff-puff service with flavor text only (no stat modifications).
func (s *Service) PuffPuff(ctx context.Context, characterID string) (*PuffPuffResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	if !CheckEligibility(char) {
		return nil, ErrAccessDenied
	}

	return &PuffPuffResult{
		CharacterID: char.ID,
		NPCName:     NPCName,
		Message:     FormatPuffPuffMessage(char.Name),
	}, nil
}

func (s *Service) getAvailableItems(ctx context.Context) ([]Item, error) {
	var excludedDefIDs map[string]bool
	if s.helperFilter != nil {
		activeHelperItemIDs, err := s.helperFilter.GetActiveHelperItemIDs(ctx)
		if err != nil {
			return nil, err
		}
		excludedDefIDs = make(map[string]bool, len(activeHelperItemIDs))
		for _, defID := range activeHelperItemIDs {
			excludedDefIDs[defID] = true
		}
	}

	items := s.catalog.Items()
	if len(excludedDefIDs) == 0 {
		return items, nil
	}

	var filtered []Item
	for _, item := range items {
		if !excludedDefIDs[item.ItemDefinitionID] {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}
