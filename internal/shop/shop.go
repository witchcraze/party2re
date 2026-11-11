package shop

import (
	"context"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	MaxTransactionQuantity = 9999
)

var (
	ErrNilDependency     = errors.New("shop dependency is nil")
	ErrInsufficientFunds = errors.New("insufficient funds to purchase item")
	ErrItemNotFound      = errors.New("item not found in catalog")
	ErrUnownedItem       = errors.New("item is not owned in inventory")
	ErrInvalidQuantity   = errors.New("invalid transaction quantity")
	ErrNotInShopHand     = errors.New("item is not in the hand this shop buys from")
	ErrPriceOverflow     = economy.ErrGoldOverflow
)

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

type EquipmentRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreequipment.Equipment, error)
	Save(ctx context.Context, value coreequipment.Equipment) error
}

type HelperProvider interface {
	GetActiveHelperItemIDs(ctx context.Context, now time.Time) ([]string, error)
}

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
}

// Deprecated: TransactionRepository is no longer used by shop.Service.
type TransactionRepository interface {
	CommitTransaction(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithDepotRepository(depotRepo DepotRepository) Option {
	return func(s *Service) {
		s.depots = depotRepo
	}
}

func WithEquipmentRepository(equipRepo EquipmentRepository) Option {
	return func(s *Service) {
		s.equipment = equipRepo
	}
}

func WithHelperProvider(helper HelperProvider) Option {
	return func(s *Service) {
		s.helper = helper
	}
}

func WithCollectionRecorder(recorder CollectionRecorder) Option {
	return func(s *Service) {
		s.recorder = recorder
	}
}

func WithTimeSource(timeSource func() time.Time) Option {
	return func(s *Service) {
		s.now = timeSource
	}
}

type SaleResult struct {
	Character    corecharacter.Character
	Inventory    coreinventory.Inventory
	SoldInstance item.Instance
	TotalPayout  int
}

type Service struct {
	characters  CharacterRepository
	inventories InventoryRepository
	depots      DepotRepository
	equipment   EquipmentRepository
	helper      HelperProvider
	recorder    CollectionRecorder
	txProvider  TransactionProvider
	catalog     item.DefinitionProvider
	economy     *economy.Service
	now         func() time.Time
}

func NewService(characters CharacterRepository, inventories InventoryRepository, catalog item.DefinitionProvider, opts ...Option) (*Service, error) {
	if characters == nil || inventories == nil || catalog == nil {
		return nil, ErrNilDependency
	}
	s := &Service{
		characters:  characters,
		inventories: inventories,
		catalog:     catalog,
		now:         time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	var ecoOpts []economy.Option
	if s.txProvider != nil {
		ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
	}
	eco, err := economy.NewService(characters, inventories, ecoOpts...)
	if err != nil {
		return nil, err
	}
	s.economy = eco
	return s, nil
}

// Deprecated: TransactionRepository is no longer used by shop.Service since economy.Service handles transactions.
// Use NewService with WithTransactionProvider option instead.
func NewServiceWithTransaction(characters CharacterRepository, inventories InventoryRepository, _ TransactionRepository, catalog item.DefinitionProvider, opts ...Option) (*Service, error) {
	return NewService(characters, inventories, catalog, opts...)
}

func (s *Service) SetHelperProvider(helper HelperProvider) {
	s.helper = helper
}

func (s *Service) SetCollectionRecorder(recorder CollectionRecorder) {
	s.recorder = recorder
}

func (s *Service) CalculateRetailPrice(shopType ShopType, itemID string, basePrice int) (int, error) {
	if basePrice <= 0 {
		return 0, nil
	}
	multiplier := 2
	if shopType == ShopTypeAccessory {
		if itemID == "item-150" || itemID == "item-151" {
			multiplier = 1000
		} else {
			multiplier = 10
		}
	}
	return safeMultiply(basePrice, multiplier)
}

func (s *Service) CalculateSellPrice(basePrice int) int {
	if basePrice <= 0 {
		return 0
	}
	return basePrice / 2
}

func safeMultiply(a, b int) (int, error) {
	return economy.SafeMultiply(a, b)
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) findCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if s.txProvider != nil {
		return s.characters.FindByIDForUpdate(ctx, characterID)
	}
	return s.characters.FindByID(ctx, characterID)
}

func (s *Service) findInventory(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if s.txProvider != nil {
		return s.inventories.FindByCharacterIDForUpdate(ctx, characterID)
	}
	return s.inventories.FindByCharacterID(ctx, characterID)
}

// GetCatalog returns the shop title, NPC name, and the available items with shop-specific retail pricing,
// filtering out items requested in active helper quests.
func (s *Service) GetCatalog(ctx context.Context, shopType ShopType, characterID string) (ShopCatalog, error) {
	if !ValidateShopType(shopType) {
		return ShopCatalog{}, ErrInvalidShopType
	}

	jobLv := 0
	if characterID != "" {
		char, err := s.characters.FindByID(ctx, characterID)
		if err != nil {
			return ShopCatalog{}, err
		}
		jobLv = char.JobLevel
	}

	salesIDs, err := GetSalesItemIDs(shopType, jobLv)
	if err != nil {
		return ShopCatalog{}, err
	}

	// Filter out items in active helper quests
	var excludedIDs map[string]bool
	if s.helper != nil {
		active, err := s.helper.GetActiveHelperItemIDs(ctx, s.now())
		if err != nil {
			return ShopCatalog{}, err
		}
		if len(active) > 0 {
			excludedIDs = make(map[string]bool, len(active))
			for _, id := range active {
				excludedIDs[id] = true
			}
		}
	}

	items := make([]CatalogItem, 0, len(salesIDs))
	for _, id := range salesIDs {
		if excludedIDs != nil && excludedIDs[id] {
			continue
		}
		def, err := s.catalog.FindByID(id)
		if err != nil {
			return ShopCatalog{}, fmt.Errorf("read catalog item %s: %w", id, err)
		}
		retailPrice, err := s.CalculateRetailPrice(shopType, def.ID, def.Price)
		if err != nil {
			return ShopCatalog{}, fmt.Errorf("price catalog item %s: %w", id, err)
		}
		items = append(items, CatalogItem{
			ID:          def.ID,
			Name:        def.Name,
			BasePrice:   def.Price,
			RetailPrice: retailPrice,
			Slot:        def.Slot,
		})
		switch shopType {
		case ShopTypeWeapon:
			items[len(items)-1].Power, items[len(items)-1].Weight = item.NominalWeaponStats(def.ID)
		case ShopTypeArmor:
			items[len(items)-1].Power, items[len(items)-1].Weight = item.NominalArmorStats(def.ID)
		}
	}

	title, npc := GetShopMeta(shopType)
	return ShopCatalog{
		ShopType: shopType,
		Title:    title,
		NPCName:  npc,
		Items:    items,
	}, nil
}

// SaleOffer is the held item a shop buys back and its unit payout.
type SaleOffer struct {
	ItemInstanceID   string `json:"item_instance_id"`
	ItemDefinitionID string `json:"item_definition_id"`
	Name             string `json:"name"`
	Payout           int    `json:"payout"`
}

// handKind is the legacy hand a shop buys from: weapon 1, armor 2, otherwise the tool hand 3.
func handKind(shopType ShopType) int {
	switch shopType {
	case ShopTypeWeapon:
		return 1
	case ShopTypeArmor:
		return 2
	default:
		return 3
	}
}

// GetSaleOffer returns the item in the shop's hand, or nil when that hand is empty.
func (s *Service) GetSaleOffer(ctx context.Context, shopType ShopType, characterID string) (*SaleOffer, error) {
	if !ValidateShopType(shopType) {
		return nil, ErrInvalidShopType
	}
	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return nil, err
	}
	for _, inst := range inv.Items {
		if item.LegacyHandKindByID(inst.DefinitionID) != handKind(shopType) {
			continue
		}
		def, err := s.catalog.FindByID(inst.DefinitionID)
		if err != nil {
			return nil, fmt.Errorf("read held item %s: %w", inst.DefinitionID, err)
		}
		return &SaleOffer{ItemInstanceID: inst.ID, ItemDefinitionID: def.ID, Name: def.Name, Payout: s.CalculateSellPrice(def.Price)}, nil
	}
	return nil, nil
}

// Sell buys back an item from the shop's hand only (legacy uru).
func (s *Service) Sell(ctx context.Context, characterID string, shopType ShopType, itemInstanceID string, quantity int) (SaleResult, error) {
	if !ValidateShopType(shopType) {
		return SaleResult{}, ErrInvalidShopType
	}
	if quantity <= 0 || quantity > MaxTransactionQuantity {
		return SaleResult{}, ErrInvalidQuantity
	}
	if characterID == "" {
		return SaleResult{}, corecharacter.ErrNotFound
	}

	var result SaleResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character first (Deterministic lock order: characters -> inventory_items)
		char, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return err
		}

		// 2. Lock Inventory next
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		instance, found := inv.Find(itemInstanceID)
		if !found {
			return ErrUnownedItem
		}
		if item.LegacyHandKindByID(instance.DefinitionID) != handKind(shopType) {
			return ErrNotInShopHand
		}
		if instance.Quantity < quantity {
			return ErrInvalidQuantity
		}

		definition, err := s.catalog.FindByID(instance.DefinitionID)
		if err != nil {
			return ErrItemNotFound
		}

		sellUnitPrice := s.CalculateSellPrice(definition.Price)
		totalPayout, err := safeMultiply(sellUnitPrice, quantity)
		if err != nil {
			return err
		}

		if s.equipment != nil {
			eq, err := s.equipment.FindByCharacterIDForUpdate(txCtx, characterID)
			if err != nil {
				return err
			}
			unlinked, err := eq.Detach(&char, itemInstanceID)
			if err != nil {
				return err
			}
			if unlinked {
				if err := s.characters.Update(txCtx, char); err != nil {
					return err
				}
				if err := s.equipment.Save(txCtx, eq); err != nil {
					return err
				}
			}
		}

		res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID:        characterID,
			AddGold:            totalPayout,
			ConsumeInstanceID:  itemInstanceID,
			ConsumeInstanceQty: quantity,
		})
		if err != nil {
			if errors.Is(err, economy.ErrGoldOverflow) {
				return ErrPriceOverflow
			}
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return corecharacter.ErrNotFound
			}
			if errors.Is(err, economy.ErrItemNotFound) {
				return ErrUnownedItem
			}
			if errors.Is(err, economy.ErrInsufficientItemQuantity) {
				return ErrInvalidQuantity
			}
			return err
		}

		soldInstance := instance
		soldInstance.Quantity = quantity

		result = SaleResult{
			Character:    res.Character,
			Inventory:    res.Inventory,
			SoldInstance: soldInstance,
			TotalPayout:  totalPayout,
		}
		return nil
	})
	if err != nil {
		return SaleResult{}, err
	}

	return result, nil
}
