package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	PresenceWindow = 5 * time.Minute

	Tier1Threshold = 10
	Tier2Threshold = 20
	Tier3Threshold = 30

	DefaultBanquetDuration = 24 * time.Hour
	DefaultToastGoldReward = 300
	MaxPurchaseQuantity    = 99
)

var (
	ErrNilDependency      = errors.New("eventplaza dependency is nil")
	ErrCharacterNotFound  = errors.New("character not found")
	ErrInsufficientGold   = errors.New("insufficient gold to complete purchase")
	ErrItemNotFound       = errors.New("bazaar item not found in catalog")
	ErrItemTierLocked     = errors.New("bazaar item requires higher town population merchant tier")
	ErrItemUnavailable    = errors.New("item is currently unavailable (active helper quest)")
	ErrDepotFull          = errors.New("depot storage is full")
	ErrDepotNotConfigured = errors.New("depot repository not configured")
	ErrInvalidQuantity    = errors.New("invalid purchase quantity")
	ErrPriceOverflow      = errors.New("price calculation overflow")
	ErrBanquetNotFound    = errors.New("celebration banquet not found")
	ErrBanquetExpired     = errors.New("celebration banquet has already ended")
	ErrAlreadyToasted     = errors.New("character has already toasted this victory celebration banquet")
	ErrMerchantNotPresent = errors.New("traveling merchant is not present in event plaza")
)

// BazaarItem represents an item offered in the Traveling Merchant Bazaar.
type BazaarItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Price            int    `json:"price"`
	TierRequired     int    `json:"tier_required"`
	Description      string `json:"description"`
	ItemDefinitionID string `json:"item_definition_id"`
}

// PlazaStatus represents the public state and unlocked features of the Event Plaza.
type PlazaStatus struct {
	ActiveParticipants  int                  `json:"active_participants"`
	MerchantTier        int                  `json:"merchant_tier"`
	MerchantTierName    string               `json:"merchant_tier_name"`
	NextTierThreshold   int                  `json:"next_tier_threshold"`
	ActiveBanquetsCount int                  `json:"active_banquets_count"`
	ActiveBanquets      []CelebrationBanquet `json:"active_banquets,omitempty"`
}

type Repository interface {
	RecordPresence(ctx context.Context, characterID string, at time.Time) error
	CountActiveParticipants(ctx context.Context, cutoff time.Time) (int, error)
	SaveBanquet(ctx context.Context, banquet CelebrationBanquet) error
	FindBanquetByID(ctx context.Context, id string) (CelebrationBanquet, error)
	ListActiveBanquets(ctx context.Context, now time.Time) ([]CelebrationBanquet, error)
	RecordToast(ctx context.Context, banquetID string, characterID string, toastedAt time.Time) error
	HasToasted(ctx context.Context, banquetID string, characterID string) (bool, error)
}

type PresenceTracker interface {
	RecordPresence(ctx context.Context, characterID string, at time.Time) error
	RecordBanquetPresence(ctx context.Context, banquetID string, count int, duration time.Duration) error
	CountActiveParticipants(ctx context.Context, cutoff time.Time) (int, error)
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
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, value depot.Depot) error
}

type HelperProvider interface {
	GetActiveHelperItemIDs(ctx context.Context, now time.Time) ([]string, error)
}

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
}

type ItemDefinitionProvider interface {
	FindByID(id string) (coreitem.Definition, error)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type Service struct {
	repo            Repository
	presenceTracker PresenceTracker
	characterRepo   CharacterRepository
	inventoryRepo   InventoryRepository
	depotRepo       DepotRepository
	helper          HelperProvider
	recorder        CollectionRecorder
	itemDefProvider ItemDefinitionProvider
	txProvider      TransactionProvider
	clock           Clock
	bazaarCatalog   []BazaarItem
	economy         *economy.Service
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithClock(clock Clock) Option {
	return func(s *Service) {
		s.clock = clock
	}
}

func WithBazaarCatalog(catalog []BazaarItem) Option {
	return func(s *Service) {
		s.bazaarCatalog = catalog
	}
}

func WithPresenceTracker(tracker PresenceTracker) Option {
	return func(s *Service) {
		s.presenceTracker = tracker
	}
}

func WithHelperProvider(helper HelperProvider) Option {
	return func(s *Service) {
		s.helper = helper
	}
}

func WithDepotRepository(depotRepo DepotRepository) Option {
	return func(s *Service) {
		s.depotRepo = depotRepo
	}
}

func WithCollectionRecorder(recorder CollectionRecorder) Option {
	return func(s *Service) {
		s.recorder = recorder
	}
}

func WithItemDefinitionProvider(provider ItemDefinitionProvider) Option {
	return func(s *Service) {
		s.itemDefProvider = provider
	}
}

func (s *Service) SetHelperProvider(helper HelperProvider) {
	s.helper = helper
}

func (s *Service) SetCollectionRecorder(recorder CollectionRecorder) {
	s.recorder = recorder
}

func NewService(
	repo Repository,
	characterRepo CharacterRepository,
	inventoryRepo InventoryRepository,
	opts ...Option,
) (*Service, error) {
	if repo == nil || characterRepo == nil || inventoryRepo == nil {
		return nil, ErrNilDependency
	}

	catalog, err := LoadDefaultBazaarCatalog()
	if err != nil {
		return nil, err
	}

	svc := &Service{
		repo:          repo,
		characterRepo: characterRepo,
		inventoryRepo: inventoryRepo,
		clock:         realClock{},
		bazaarCatalog: catalog,
	}

	for _, opt := range opts {
		opt(svc)
	}

	var ecoOpts []economy.Option
	if svc.txProvider != nil {
		ecoOpts = append(ecoOpts, economy.WithTransactionProvider(svc.txProvider))
	}
	eco, err := economy.NewService(characterRepo, inventoryRepo, ecoOpts...)
	if err != nil {
		return nil, err
	}
	svc.economy = eco

	return svc, nil
}

func (s *Service) countParticipants(ctx context.Context, cutoff time.Time) (int, error) {
	if s.presenceTracker != nil {
		return s.presenceTracker.CountActiveParticipants(ctx, cutoff)
	}
	return s.repo.CountActiveParticipants(ctx, cutoff)
}

func (s *Service) recordPresence(ctx context.Context, characterID string, at time.Time) error {
	if s.presenceTracker != nil {
		return s.presenceTracker.RecordPresence(ctx, characterID, at)
	}
	return s.repo.RecordPresence(ctx, characterID, at)
}

// CalculateMerchantTier calculates the active traveling merchant tier based on real-time plaza concurrency.
func CalculateMerchantTier(participants int) (tier int, tierName string, nextThreshold int) {
	if participants >= Tier3Threshold {
		return 3, "Gold Traveling Merchant (至高の行商人バザー)", 0
	}
	if participants >= Tier2Threshold {
		return 2, "Silver Traveling Merchant (熟練の行商人バザー)", Tier3Threshold
	}
	if participants >= Tier1Threshold {
		return 1, "Bronze Traveling Merchant (旅の行商人バザー)", Tier2Threshold
	}
	return 0, "Traveling Merchant On Journey (行商人巡回中)", Tier1Threshold
}

// BanquetAttendeesForTier returns the number of virtual celebration attendees (10–30) for a boss tier.
// Defeating a King Boss tier yields:
// - Tier 1: 10 attendees -> unlocks Tier 1 Traveling Merchant (Bronze)
// - Tier 2: 20 attendees -> unlocks Tier 2 Traveling Merchant (Silver)
// - Tier 3+: 30 attendees -> unlocks Tier 3 Traveling Merchant (Gold)
func BanquetAttendeesForTier(tier int) int {
	if tier <= 1 {
		return Tier1Threshold
	}
	if tier == 2 {
		return Tier2Threshold
	}
	return Tier3Threshold
}

func (s *Service) RecordPresence(ctx context.Context, characterID string) error {
	if strings.TrimSpace(characterID) == "" {
		return ErrCharacterNotFound
	}
	return s.recordPresence(ctx, strings.TrimSpace(characterID), s.clock.Now())
}

// RecordBanquetPresence records virtual celebration banquet attendees into the presence tracker.
func (s *Service) RecordBanquetPresence(ctx context.Context, banquetID string, count int, duration time.Duration) error {
	if s.presenceTracker != nil {
		if trackerWithClock, ok := s.presenceTracker.(interface {
			RecordBanquetPresenceAt(ctx context.Context, banquetID string, count int, at time.Time, duration time.Duration) error
		}); ok {
			return trackerWithClock.RecordBanquetPresenceAt(ctx, banquetID, count, s.clock.Now(), duration)
		}
		return s.presenceTracker.RecordBanquetPresence(ctx, banquetID, count, duration)
	}
	return nil
}

func (s *Service) GetPlazaStatus(ctx context.Context) (PlazaStatus, error) {
	cutoff := s.clock.Now().Add(-PresenceWindow)
	participants, err := s.countParticipants(ctx, cutoff)
	if err != nil {
		return PlazaStatus{}, fmt.Errorf("failed to count active participants: %w", err)
	}

	tier, tierName, nextThreshold := CalculateMerchantTier(participants)

	now := s.clock.Now()
	banquets, err := s.repo.ListActiveBanquets(ctx, now)
	if err != nil {
		return PlazaStatus{}, fmt.Errorf("failed to list active banquets: %w", err)
	}

	return PlazaStatus{
		ActiveParticipants:  participants,
		MerchantTier:        tier,
		MerchantTierName:    tierName,
		NextTierThreshold:   nextThreshold,
		ActiveBanquetsCount: len(banquets),
		ActiveBanquets:      banquets,
	}, nil
}

func (s *Service) ListAvailableBazaarItems(ctx context.Context) ([]BazaarItem, int, error) {
	cutoff := s.clock.Now().Add(-PresenceWindow)
	participants, err := s.countParticipants(ctx, cutoff)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count active participants: %w", err)
	}

	tier, _, _ := CalculateMerchantTier(participants)
	if tier <= 0 {
		return []BazaarItem{}, 0, nil
	}

	var activeHelperIDs []string
	if s.helper != nil {
		ids, err := s.helper.GetActiveHelperItemIDs(ctx, s.clock.Now())
		if err != nil {
			return nil, 0, fmt.Errorf("failed to get active helper item ids: %w", err)
		}
		activeHelperIDs = ids
	}

	isHelperItem := make(map[string]bool, len(activeHelperIDs))
	for _, id := range activeHelperIDs {
		isHelperItem[id] = true
	}

	var available []BazaarItem
	for _, item := range s.bazaarCatalog {
		if item.TierRequired == tier {
			if isHelperItem[item.ItemDefinitionID] {
				continue
			}
			available = append(available, item)
		}
	}

	return available, tier, nil
}
