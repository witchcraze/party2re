package blacksmith

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	MaxEnhancementLevel         = 10
	DefaultMaterialDefinitionID = "item-084" // 魔石のカケラ
)

var (
	ErrInvalidCharacterID    = errors.New("invalid character ID")
	ErrInvalidItemInstanceID = errors.New("invalid item instance ID")
	ErrItemNotFound          = errors.New("item instance not found in inventory")
	ErrItemNotEquipment      = errors.New("item is not equipment")
	ErrMaxEnhancementReached = errors.New("item has reached maximum enhancement level")
	ErrInsufficientFunds     = errors.New("insufficient character funds for enhancement")
	ErrInsufficientMaterials = errors.New("insufficient materials for enhancement")
)

type RandomSource interface {
	Float64() float64
}

type cryptoRandomSource struct{}

func (cryptoRandomSource) Float64() float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return 0.5
	}
	return float64(n.Int64()) / 1000000.0
}

type Result struct {
	Success       bool
	PreviousLevel int
	NewLevel      int
	GoldCost      int
	MaterialCost  int
	ItemInstance  item.Instance
}

func CalculateCost(currentLevel int, basePrice int) (goldCost int, materialCost int) {
	if basePrice < 50 {
		basePrice = 50
	}
	goldCost = basePrice * (currentLevel + 1) / 2
	if goldCost < 50 {
		goldCost = 50
	}
	materialCost = 1 + currentLevel/3
	return goldCost, materialCost
}

func CalculateSuccessRate(currentLevel int) float64 {
	switch currentLevel {
	case 0:
		return 1.00
	case 1:
		return 0.95
	case 2:
		return 0.90
	case 3:
		return 0.80
	case 4:
		return 0.70
	case 5:
		return 0.60
	case 6:
		return 0.50
	case 7:
		return 0.40
	case 8:
		return 0.30
	case 9:
		return 0.20
	default:
		return 0.00
	}
}

func CalculateStatsBonus(currentLevel int, baseAttack int, baseDefense int) (bonusAttack int, bonusDefense int) {
	bonusAttack = baseAttack*currentLevel/10 + currentLevel*2
	bonusDefense = baseDefense*currentLevel/10 + currentLevel*2
	return bonusAttack, bonusDefense
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

type TransactionRepository interface {
	CommitEnhancement(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type TransactionRunner interface {
	ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error)
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithTransactionRunner(runner TransactionRunner) Option {
	return func(s *Service) {
		s.runner = runner
	}
}

func WithEconomy(eco *economy.Service) Option {
	return func(s *Service) {
		s.runner = eco
	}
}

func WithRandomSource(randSource RandomSource) Option {
	return func(s *Service) {
		if randSource != nil {
			s.randSource = randSource
		}
	}
}

type Service struct {
	characters  CharacterRepository
	inventories InventoryRepository
	txRepo      TransactionRepository
	txProvider  TransactionProvider
	runner      TransactionRunner
	catalog     item.DefinitionProvider
	materialID  string
	randSource  RandomSource
}

func NewService(
	characters CharacterRepository,
	inventories InventoryRepository,
	catalog item.DefinitionProvider,
	opts ...Option,
) (*Service, error) {
	if characters == nil || inventories == nil || catalog == nil {
		return nil, errors.New("dependencies are nil")
	}
	s := &Service{
		characters:  characters,
		inventories: inventories,
		catalog:     catalog,
		materialID:  DefaultMaterialDefinitionID,
		randSource:  cryptoRandomSource{},
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.runner == nil {
		var ecoOpts []economy.Option
		if s.txProvider != nil {
			ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
		}
		eco, err := economy.NewService(characters, inventories, ecoOpts...)
		if err != nil {
			return nil, err
		}
		s.runner = eco
	}
	return s, nil
}

func NewServiceWithTransaction(
	characters CharacterRepository,
	inventories InventoryRepository,
	txRepo TransactionRepository,
	catalog item.DefinitionProvider,
	randSource RandomSource,
	opts ...Option,
) (*Service, error) {
	if characters == nil || inventories == nil || catalog == nil {
		return nil, errors.New("dependencies are nil")
	}
	if randSource == nil {
		randSource = cryptoRandomSource{}
	}
	s := &Service{
		characters:  characters,
		inventories: inventories,
		txRepo:      txRepo,
		catalog:     catalog,
		materialID:  DefaultMaterialDefinitionID,
		randSource:  randSource,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.runner == nil {
		var ecoOpts []economy.Option
		if s.txProvider != nil {
			ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
		}
		eco, err := economy.NewService(characters, inventories, ecoOpts...)
		if err != nil {
			return nil, err
		}
		s.runner = eco
	}
	return s, nil
}

func (s *Service) SetMaterialDefinitionID(materialID string) {
	if materialID != "" {
		s.materialID = materialID
	}
}

func (s *Service) Enhance(ctx context.Context, characterID string, itemInstanceID string) (Result, error) {
	return s.EnhanceEquipment(ctx, characterID, itemInstanceID)
}

func (s *Service) EnhanceEquipment(ctx context.Context, characterID string, itemInstanceID string) (Result, error) {
	if characterID == "" {
		return Result{}, ErrInvalidCharacterID
	}
	if itemInstanceID == "" {
		return Result{}, ErrInvalidItemInstanceID
	}

	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return Result{}, err
	}

	targetItem, found := inv.Find(itemInstanceID)
	if !found {
		return Result{}, ErrItemNotFound
	}

	def, err := s.catalog.FindByID(targetItem.DefinitionID)
	if err != nil {
		return Result{}, fmt.Errorf("lookup item definition: %w", err)
	}

	if def.Slot == item.SlotNone {
		return Result{}, ErrItemNotEquipment
	}

	if targetItem.EnhancementLevel >= MaxEnhancementLevel {
		return Result{}, ErrMaxEnhancementReached
	}

	goldCost, materialCost := CalculateCost(targetItem.EnhancementLevel, def.Price)

	req := economy.TransactionRequest{
		CharacterID: characterID,
		Cost: economy.ResourceCost{
			Gold:              goldCost,
			ItemDefinitionID:  s.materialID,
			ItemDefinitionQty: materialCost,
		},
		LockInventory: true,
	}

	var result Result
	_, err = s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		lockedItem, found := tc.Inventory.Find(itemInstanceID)
		if !found {
			return ErrItemNotFound
		}
		if lockedItem.EnhancementLevel >= MaxEnhancementLevel {
			return ErrMaxEnhancementReached
		}

		successRate := CalculateSuccessRate(lockedItem.EnhancementLevel)
		roll := s.randSource.Float64()
		success := roll < successRate

		prevLevel := lockedItem.EnhancementLevel
		newLevel := prevLevel
		if success {
			newLevel++
			lockedItem.EnhancementLevel = newLevel
			if err := tc.Inventory.Update(lockedItem); err != nil {
				return err
			}
		}

		result = Result{
			Success:       success,
			PreviousLevel: prevLevel,
			NewLevel:      newLevel,
			GoldCost:      goldCost,
			MaterialCost:  materialCost,
			ItemInstance:  lockedItem,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, economy.ErrInsufficientGold) {
			return Result{}, ErrInsufficientFunds
		}
		if errors.Is(err, economy.ErrInsufficientItemQuantity) {
			return Result{}, ErrInsufficientMaterials
		}
		if errors.Is(err, economy.ErrCharacterNotFound) {
			return Result{}, corecharacter.ErrNotFound
		}
		return Result{}, err
	}

	return result, nil
}
