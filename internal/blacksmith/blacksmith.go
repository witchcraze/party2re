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

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
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
	return s, nil
}

func (s *Service) SetMaterialDefinitionID(materialID string) {
	if materialID != "" {
		s.materialID = materialID
	}
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

func (s *Service) Enhance(ctx context.Context, characterID string, itemInstanceID string) (Result, error) {
	if characterID == "" {
		return Result{}, ErrInvalidCharacterID
	}
	if itemInstanceID == "" {
		return Result{}, ErrInvalidItemInstanceID
	}

	var result Result
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		character, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return err
		}

		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		targetItem, found := inv.Find(itemInstanceID)
		if !found {
			return ErrItemNotFound
		}

		def, err := s.catalog.FindByID(targetItem.DefinitionID)
		if err != nil {
			return fmt.Errorf("lookup item definition: %w", err)
		}

		if def.Slot == item.SlotNone {
			return ErrItemNotEquipment
		}

		if targetItem.EnhancementLevel >= MaxEnhancementLevel {
			return ErrMaxEnhancementReached
		}

		goldCost, materialCost := CalculateCost(targetItem.EnhancementLevel, def.Price)
		if character.Money < goldCost {
			return ErrInsufficientFunds
		}

		availableMaterials := inv.Quantity(s.materialID)
		if availableMaterials < materialCost {
			return ErrInsufficientMaterials
		}

		// Deduct gold
		if err := character.DeductMoney(goldCost); err != nil {
			return ErrInsufficientFunds
		}

		// Consume material from inventory
		materialToConsume := materialCost
		for _, inst := range inv.Items {
			if inst.DefinitionID == s.materialID && inst.Quantity > 0 {
				toTake := inst.Quantity
				if toTake > materialToConsume {
					toTake = materialToConsume
				}
				_ = inv.Consume(inst.ID, toTake)
				materialToConsume -= toTake
				if materialToConsume <= 0 {
					break
				}
			}
		}

		// Roll success
		successRate := CalculateSuccessRate(targetItem.EnhancementLevel)
		roll := s.randSource.Float64()
		success := roll < successRate

		prevLevel := targetItem.EnhancementLevel
		newLevel := prevLevel
		if success {
			newLevel++
			targetItem.EnhancementLevel = newLevel
			// Update item instance in inventory
			_ = inv.Update(targetItem)
		}

		// Commit atomically if transaction repository is configured (and no txProvider)
		if s.txRepo != nil && s.txProvider == nil {
			if err := s.txRepo.CommitEnhancement(txCtx, character, inv); err != nil {
				return fmt.Errorf("commit enhancement transaction: %w", err)
			}
		} else {
			if err := s.characters.Update(txCtx, character); err != nil {
				return fmt.Errorf("update character: %w", err)
			}
			if err := s.inventories.Save(txCtx, inv); err != nil {
				return fmt.Errorf("save inventory: %w", err)
			}
		}

		result = Result{
			Success:       success,
			PreviousLevel: prevLevel,
			NewLevel:      newLevel,
			GoldCost:      goldCost,
			MaterialCost:  materialCost,
			ItemInstance:  targetItem,
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	return result, nil
}
