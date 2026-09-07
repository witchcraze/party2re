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
	CharacterID string          `json:"character_id"`
	ExDepot     int             `json:"ex_depot"`
	Capacity    int             `json:"capacity"`
	Items       []item.Instance `json:"items"`
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

// AddItem adds an item to depot.
// Resolves Issue #452: Stacking items with existing identical definition ID
// are merged into existing slot first without consuming a new slot.
// Only non-stacking new items require len(Items) < Capacity.
func (d *Depot) AddItem(instance item.Instance) error {
	for i, existing := range d.Items {
		if existing.DefinitionID == instance.DefinitionID {
			d.Items[i].Quantity += instance.Quantity
			return nil
		}
	}
	if len(d.Items) >= d.Capacity {
		return ErrDepotFull
	}
	d.Items = append(d.Items, instance)
	return nil
}

func (d *Depot) RemoveItem(instanceID string) (item.Instance, error) {
	for i, existing := range d.Items {
		if existing.ID == instanceID {
			d.Items = append(d.Items[:i], d.Items[i+1:]...)
			return existing, nil
		}
	}
	return item.Instance{}, ErrItemNotFound
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
