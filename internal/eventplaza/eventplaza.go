package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	Tier1Threshold = 10
	Tier2Threshold = 20
	Tier3Threshold = 30

	DefaultBanquetDuration = 24 * time.Hour
	DefaultToastGoldReward = 300
	MaxPurchaseQuantity    = 99
)

var (
	ErrNilDependency     = errors.New("eventplaza dependency is nil")
	ErrCharacterNotFound = errors.New("character not found")
	ErrInsufficientGold  = errors.New("insufficient gold to complete purchase")
	ErrItemNotFound      = errors.New("bazaar item not found in catalog")
	ErrItemTierLocked    = errors.New("bazaar item requires higher town population merchant tier")
	ErrInvalidQuantity   = errors.New("invalid purchase quantity")
	ErrPriceOverflow     = errors.New("price calculation overflow")
	ErrBanquetNotFound   = errors.New("celebration banquet not found")
	ErrBanquetExpired    = errors.New("celebration banquet has already ended")
	ErrAlreadyToasted    = errors.New("character has already toasted this victory celebration banquet")
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

// CelebrationBanquet represents a victory feast in honor of slaying a legendary boss.
type CelebrationBanquet struct {
	ID                  string    `json:"id"`
	BossID              string    `json:"boss_id"`
	BossName            string    `json:"boss_name"`
	SlayerCharacterID   string    `json:"slayer_character_id"`
	SlayerCharacterName string    `json:"slayer_character_name"`
	Tier                int       `json:"tier"`
	ToastCount          int       `json:"toast_count"`
	CelebratedAt        time.Time `json:"celebrated_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// BanquetToastResult represents the outcome of raising a commemorative toast at a banquet.
type BanquetToastResult struct {
	BanquetID            string `json:"banquet_id"`
	CharacterID          string `json:"character_id"`
	GoldAwarded          int    `json:"gold_awarded"`
	CurrentCharacterGold int    `json:"current_character_gold"`
	ToastCount           int    `json:"toast_count"`
	Message              string `json:"message"`
}

// BazaarPurchaseResult represents the result of buying goods from the traveling merchant.
type BazaarPurchaseResult struct {
	CharacterID         string     `json:"character_id"`
	Item                BazaarItem `json:"item"`
	Quantity            int        `json:"quantity"`
	TotalPrice          int        `json:"total_price"`
	RemainingGold       int        `json:"remaining_gold"`
	InventoryInstanceID string     `json:"inventory_instance_id"`
}

type Repository interface {
	CountActiveParticipants(ctx context.Context) (int, error)
	SaveBanquet(ctx context.Context, banquet CelebrationBanquet) error
	FindBanquetByID(ctx context.Context, id string) (CelebrationBanquet, error)
	ListActiveBanquets(ctx context.Context, now time.Time) ([]CelebrationBanquet, error)
	RecordToast(ctx context.Context, banquetID string, characterID string, toastedAt time.Time) error
	HasToasted(ctx context.Context, banquetID string, characterID string) (bool, error)
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
	repo          Repository
	characterRepo CharacterRepository
	inventoryRepo InventoryRepository
	txProvider    TransactionProvider
	clock         Clock
	bazaarCatalog []BazaarItem
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

	return svc, nil
}

// CalculateMerchantTier calculates the active traveling merchant tier based on population.
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

func (s *Service) GetPlazaStatus(ctx context.Context) (PlazaStatus, error) {
	participants, err := s.repo.CountActiveParticipants(ctx)
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
	participants, err := s.repo.CountActiveParticipants(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count active participants: %w", err)
	}

	tier, _, _ := CalculateMerchantTier(participants)
	if tier <= 0 {
		return []BazaarItem{}, 0, nil
	}

	var available []BazaarItem
	for _, item := range s.bazaarCatalog {
		if item.TierRequired <= tier {
			available = append(available, item)
		}
	}

	return available, tier, nil
}

func (s *Service) PurchaseBazaarItem(
	ctx context.Context,
	characterID string,
	itemID string,
	quantity int,
) (BazaarPurchaseResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return BazaarPurchaseResult{}, ErrCharacterNotFound
	}
	if quantity <= 0 || quantity > MaxPurchaseQuantity {
		return BazaarPurchaseResult{}, ErrInvalidQuantity
	}

	var targetItem *BazaarItem
	for i := range s.bazaarCatalog {
		if s.bazaarCatalog[i].ID == itemID {
			targetItem = &s.bazaarCatalog[i]
			break
		}
	}
	if targetItem == nil {
		return BazaarPurchaseResult{}, ErrItemNotFound
	}

	participants, err := s.repo.CountActiveParticipants(ctx)
	if err != nil {
		return BazaarPurchaseResult{}, fmt.Errorf("failed to count active participants: %w", err)
	}

	currentTier, _, _ := CalculateMerchantTier(participants)
	if targetItem.TierRequired > currentTier {
		return BazaarPurchaseResult{}, ErrItemTierLocked
	}

	if targetItem.Price > math.MaxInt/quantity {
		return BazaarPurchaseResult{}, ErrPriceOverflow
	}
	totalCost := targetItem.Price * quantity

	var result BazaarPurchaseResult
	runInTx := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if char.Money < totalCost {
			return ErrInsufficientGold
		}

		inv, err := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			inv, _ = coreinventory.New(characterID)
		}

		inst, err := coreitem.NewInstance(targetItem.ItemDefinitionID, quantity)
		if err != nil {
			return fmt.Errorf("failed to create item instance: %w", err)
		}

		if err := inv.Add(inst); err != nil {
			return fmt.Errorf("failed to add item to inventory: %w", err)
		}

		if err := char.DeductMoney(totalCost); err != nil {
			return ErrInsufficientGold
		}

		if err := s.characterRepo.Update(txCtx, char); err != nil {
			return fmt.Errorf("failed to update character money: %w", err)
		}

		if err := s.inventoryRepo.Save(txCtx, inv); err != nil {
			return fmt.Errorf("failed to save inventory: %w", err)
		}

		result = BazaarPurchaseResult{
			CharacterID:         characterID,
			Item:                *targetItem,
			Quantity:            quantity,
			TotalPrice:          totalCost,
			RemainingGold:       char.Money,
			InventoryInstanceID: inst.ID,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, runInTx); err != nil {
			return BazaarPurchaseResult{}, err
		}
	} else {
		if err := runInTx(ctx); err != nil {
			return BazaarPurchaseResult{}, err
		}
	}

	return result, nil
}

func (s *Service) RecordVictoryBanquet(
	ctx context.Context,
	bossID, bossName, slayerID, slayerName string,
	tier int,
) (CelebrationBanquet, error) {
	if strings.TrimSpace(bossID) == "" || strings.TrimSpace(bossName) == "" ||
		strings.TrimSpace(slayerID) == "" || strings.TrimSpace(slayerName) == "" {
		return CelebrationBanquet{}, errors.New("invalid victory banquet parameters")
	}

	now := s.clock.Now()
	banquet := CelebrationBanquet{
		ID:                  id.New(),
		BossID:              strings.TrimSpace(bossID),
		BossName:            strings.TrimSpace(bossName),
		SlayerCharacterID:   strings.TrimSpace(slayerID),
		SlayerCharacterName: strings.TrimSpace(slayerName),
		Tier:                tier,
		ToastCount:          0,
		CelebratedAt:        now,
		ExpiresAt:           now.Add(DefaultBanquetDuration),
	}

	if err := s.repo.SaveBanquet(ctx, banquet); err != nil {
		return CelebrationBanquet{}, fmt.Errorf("failed to save celebration banquet: %w", err)
	}

	return banquet, nil
}

func (s *Service) ListActiveBanquets(ctx context.Context) ([]CelebrationBanquet, error) {
	now := s.clock.Now()
	return s.repo.ListActiveBanquets(ctx, now)
}

func (s *Service) ToastBanquet(ctx context.Context, banquetID string, characterID string) (BanquetToastResult, error) {
	if strings.TrimSpace(banquetID) == "" {
		return BanquetToastResult{}, ErrBanquetNotFound
	}
	if strings.TrimSpace(characterID) == "" {
		return BanquetToastResult{}, ErrCharacterNotFound
	}

	banquet, err := s.repo.FindBanquetByID(ctx, banquetID)
	if err != nil {
		return BanquetToastResult{}, ErrBanquetNotFound
	}

	now := s.clock.Now()
	if now.After(banquet.ExpiresAt) {
		return BanquetToastResult{}, ErrBanquetExpired
	}

	hasToasted, err := s.repo.HasToasted(ctx, banquetID, characterID)
	if err != nil {
		return BanquetToastResult{}, fmt.Errorf("failed to check toast status: %w", err)
	}
	if hasToasted {
		return BanquetToastResult{}, ErrAlreadyToasted
	}

	var result BanquetToastResult
	runInTx := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if err := s.repo.RecordToast(txCtx, banquetID, characterID, now); err != nil {
			return ErrAlreadyToasted
		}

		rewardGold := DefaultToastGoldReward * banquet.Tier
		if rewardGold <= 0 {
			rewardGold = DefaultToastGoldReward
		}
		_ = char.AddMoney(rewardGold)

		if err := s.characterRepo.Update(txCtx, char); err != nil {
			return fmt.Errorf("failed to update character money on toast: %w", err)
		}

		result = BanquetToastResult{
			BanquetID:            banquetID,
			CharacterID:          characterID,
			GoldAwarded:          rewardGold,
			CurrentCharacterGold: char.Money,
			ToastCount:           banquet.ToastCount + 1,
			Message:              fmt.Sprintf("英雄 %s の討伐偉業を称えて乾杯しました！祝宴の引き出物として %d G を受け取りました。", banquet.SlayerCharacterName, rewardGold),
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, runInTx); err != nil {
			return BanquetToastResult{}, err
		}
	} else {
		if err := runInTx(ctx); err != nil {
			return BanquetToastResult{}, err
		}
	}

	return result, nil
}
