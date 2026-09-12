package depot

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	MinDepotCapacity     = 5
	MaxDepotCapacity     = 500
	DefaultDepotCapacity = 5
	MaxExDepot           = 20
	MaxOverDepot         = 5
)

var (
	ErrNotFound               = errors.New("depot not found")
	ErrDepotFull              = errors.New("depot is at full capacity")
	ErrInventoryFull          = errors.New("inventory is full")
	ErrItemNotFound           = errors.New("item not found")
	ErrInsufficientFunds      = errors.New("insufficient character funds")
	ErrInvalidAmount          = errors.New("amount must be positive")
	ErrInvalidCharacterID     = errors.New("invalid character ID")
	ErrInvalidItemInstanceID  = errors.New("invalid item instance ID")
	ErrDepotMaxExpanded       = errors.New("depot is already at maximum expansion")
	ErrSelfTransferNotAllowed = errors.New("cannot transfer to self")
	ErrRecipientNotFound      = errors.New("recipient character not found")
	ErrRecipientDepotFull     = errors.New("recipient depot is at full capacity")
	ErrEmptyItemList          = errors.New("item list cannot be empty")
	ErrInvalidQuantity        = errors.New("item quantity is invalid")
)

var ExpansionCosts = [...]int{
	200000, 200000, // 0, 1
	400000, 400000, // 2, 3
	600000, 600000, // 4, 5
	800000, 800000, // 6, 7
	999999, 999999, 999999, 999999, 999999, // 8..20
	999999, 999999, 999999, 999999, 999999,
	999999, 999999, 999999,
}

// ExpansionCost returns the gold cost to perform the next depot expansion given the current exDepot count.
func ExpansionCost(exDepot int) (int, error) {
	if exDepot < 0 || exDepot >= MaxExDepot {
		return 0, ErrDepotMaxExpanded
	}
	return ExpansionCosts[exDepot], nil
}

// CalculateCapacity calculates dynamic depot capacity according to legacy formula (system.cgi:get_depot_c):
// Base: jobLv >= 29 ? 150 : jobLv * 5 + 5 (5..150)
// ExDepot: exDepot * 5 (0..100, max 20 expansions)
// OverDepot: overDepot * 50 (0..250, max 5 god limit breaks)
// Total theoretical capacity range: [5, 500]
func CalculateCapacity(jobLv, exDepot, overDepot int) int {
	if jobLv < 0 {
		jobLv = 0
	}
	if exDepot < 0 {
		exDepot = 0
	} else if exDepot > MaxExDepot {
		exDepot = MaxExDepot
	}
	if overDepot < 0 {
		overDepot = 0
	} else if overDepot > MaxOverDepot {
		overDepot = MaxOverDepot
	}

	base := 5
	if jobLv >= 29 {
		base = 150
	} else if jobLv > 0 {
		base = jobLv*5 + 5
	}
	total := base + (exDepot * 5) + (overDepot * 50)
	if total > MaxDepotCapacity {
		total = MaxDepotCapacity
	}
	return total
}

type Depot struct {
	CharacterID string                  `json:"character_id"`
	ExDepot     int                     `json:"ex_depot"`
	Capacity    int                     `json:"capacity"`
	Items       []item.Instance         `json:"items"`
	ItemDefs    item.DefinitionProvider `json:"-"`
}

func NewDepot(characterID string) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	return Depot{
		CharacterID: characterID,
		ExDepot:     0,
		Capacity:    MinDepotCapacity,
		Items:       []item.Instance{},
	}, nil
}

func NewDepotWithCapacity(characterID string, jobLv, exDepot, overDepot int) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	cap := CalculateCapacity(jobLv, exDepot, overDepot)
	if exDepot > MaxExDepot {
		exDepot = MaxExDepot
	}
	if exDepot < 0 {
		exDepot = 0
	}
	return Depot{
		CharacterID: characterID,
		ExDepot:     exDepot,
		Capacity:    cap,
		Items:       []item.Instance{},
	}, nil
}

// SetItemDefinitionProvider configures the item definition provider for stackability resolution.
func (d *Depot) SetItemDefinitionProvider(provider item.DefinitionProvider) {
	d.ItemDefs = provider
}

// isStackable determines whether the given item definition ID represents a stackable item.
func (d *Depot) isStackable(definitionID string) bool {
	if d.ItemDefs != nil {
		if def, err := d.ItemDefs.FindByID(definitionID); err == nil {
			return def.IsStackable()
		}
	}
	return item.IsStackableID(definitionID)
}

// RefreshCapacity recalculates and updates the depot capacity based on the character's JobLevel and OverDepot.
func (d *Depot) RefreshCapacity(jobLv, overDepot int) {
	d.Capacity = CalculateCapacity(jobLv, d.ExDepot, overDepot)
}

// AddItem adds an item to depot.
// Resolves Issue #452, #558, & #563: Stacking items (stackable consumables/materials with EnhancementLevel == 0)
// are merged into an existing slot first without consuming a new slot.
// Equipment items (non-stackable) or items with EnhancementLevel > 0 represent
// distinct gear instances and must never be collapsed, always occupying separate slots.
func (d *Depot) AddItem(instance item.Instance, stackable ...bool) error {
	isStack := true
	if len(stackable) > 0 {
		isStack = stackable[0]
	} else {
		isStack = d.isStackable(instance.DefinitionID)
	}

	if isStack && instance.EnhancementLevel == 0 {
		for i, existing := range d.Items {
			if existing.DefinitionID == instance.DefinitionID && existing.EnhancementLevel == 0 {
				d.Items[i].Quantity += instance.Quantity
				return nil
			}
		}
	}
	if len(d.Items) >= d.Capacity {
		return ErrDepotFull
	}
	d.Items = append(d.Items, instance)
	return nil
}

// Consume removes quantity of the item with the given instanceID.
// Returns ErrInvalidQuantity if quantity <= 0 or if the existing item quantity < quantity.
// If the remaining quantity reaches 0, the item slot is removed from depot storage.
// Returns a copy of the consumed item instance with Quantity = quantity.
func (d *Depot) Consume(instanceID string, quantity int) (item.Instance, error) {
	if quantity <= 0 {
		return item.Instance{}, ErrInvalidQuantity
	}
	for i, existing := range d.Items {
		if existing.ID == instanceID {
			if existing.Quantity < quantity {
				return item.Instance{}, ErrInvalidQuantity
			}
			consumed := existing
			consumed.Quantity = quantity

			if existing.Quantity == quantity {
				d.Items = append(d.Items[:i], d.Items[i+1:]...)
			} else {
				d.Items[i].Quantity -= quantity
			}
			return consumed, nil
		}
	}
	return item.Instance{}, ErrItemNotFound
}

// Quantity returns the total quantity of items matching definitionID in the depot.
func (d *Depot) Quantity(definitionID string) int {
	total := 0
	for _, inst := range d.Items {
		if inst.DefinitionID == definitionID {
			total += inst.Quantity
		}
	}
	return total
}

// ConsumeOne removes one unit of the item with the given instanceID.
// Delegates to Consume(instanceID, 1).
func (d *Depot) ConsumeOne(instanceID string) (item.Instance, error) {
	return d.Consume(instanceID, 1)
}

// PurgeSlot unconditionally removes an entire item slot regardless of Quantity.
// Use Consume or ConsumeOne for safe item consumption and quantity decrements.
func (d *Depot) PurgeSlot(instanceID string) (item.Instance, error) {
	for i, existing := range d.Items {
		if existing.ID == instanceID {
			d.Items = append(d.Items[:i], d.Items[i+1:]...)
			return existing, nil
		}
	}
	return item.Instance{}, ErrItemNotFound
}

// RemoveItem unconditionally removes the entire item slot regardless of Quantity.
// Deprecated: Use Consume or ConsumeOne for safe quantity decrements, or PurgeSlot if intentional whole-slot deletion is required.
func (d *Depot) RemoveItem(instanceID string) (item.Instance, error) {
	return d.PurgeSlot(instanceID)
}

type Repository interface {
	FindByCharacterID(ctx context.Context, characterID string) (Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (Depot, error)
	Save(ctx context.Context, value Depot) error
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

type TransactionRunner interface {
	ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type ItemDefinitionProvider = item.DefinitionProvider

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) { s.txProvider = txProvider }
}

func WithTransactionRunner(runner TransactionRunner) Option {
	return func(s *Service) { s.runner = runner }
}

func WithEconomy(eco *economy.Service) Option {
	return func(s *Service) { s.runner = eco }
}

func WithItemDefinitionProvider(provider ItemDefinitionProvider) Option {
	return func(s *Service) { s.itemDefs = provider }
}

func WithCollectionRecorder(recorder CollectionRecorder) Option {
	return func(s *Service) { s.collector = recorder }
}

func (s *Service) SetCollectionRecorder(recorder CollectionRecorder) {
	s.collector = recorder
}

type Service struct {
	depotRepo  Repository
	charRepo   CharacterRepository
	invRepo    InventoryRepository
	txProvider TransactionProvider
	runner     TransactionRunner
	itemDefs   ItemDefinitionProvider
	collector  CollectionRecorder
}

func NewService(depotRepo Repository, charRepo CharacterRepository, invRepo InventoryRepository, opts ...Option) (*Service, error) {
	if depotRepo == nil || charRepo == nil || invRepo == nil {
		return nil, errors.New("dependencies are nil")
	}
	s := &Service{depotRepo: depotRepo, charRepo: charRepo, invRepo: invRepo}
	for _, opt := range opts {
		opt(s)
	}
	if s.runner == nil {
		var ecoOpts []economy.Option
		if s.txProvider != nil {
			ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
		}
		eco, err := economy.NewService(charRepo, invRepo, ecoOpts...)
		if err != nil {
			return nil, err
		}
		s.runner = eco
	}
	return s, nil
}

func NewServiceWithTransaction(depotRepo Repository, charRepo CharacterRepository, invRepo InventoryRepository, txProvider TransactionProvider, opts ...Option) (*Service, error) {
	if txProvider == nil {
		return nil, errors.New("dependencies are nil")
	}
	return NewService(depotRepo, charRepo, invRepo, append([]Option{WithTransactionProvider(txProvider)}, opts...)...)
}
