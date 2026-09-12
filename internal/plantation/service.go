package plantation

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
)

// RNG represents a random number generator for plantation outcomes.
type RNG interface {
	Intn(n int) int
}

type defaultRNG struct{}

func (defaultRNG) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.IntN(n)
}

// CharacterRepository defines required character operations.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// InventoryRepository defines required inventory operations.
type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inv coreinventory.Inventory) error
}

// DepotRepository defines required depot operations.
type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, dep depot.Depot) error
}

// Repository defines storage operations for plantation plots.
type Repository interface {
	GetPlot(ctx context.Context, characterID string) (Plot, error)
	GetPlotForUpdate(ctx context.Context, characterID string) (Plot, error)
	SavePlot(ctx context.Context, plot Plot) error
	DeletePlot(ctx context.Context, characterID string) error
}

// TransactionProvider defines ambient transaction execution.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

// Service coordinates plantation operations.
type Service struct {
	characters  CharacterRepository
	inventories InventoryRepository
	depots      DepotRepository
	plots       Repository
	items       coreitem.DefinitionProvider
	txProvider  TransactionProvider
	now         func() time.Time
	rng         RNG
}

// Option configures Service.
type Option func(*Service)

// WithNow configures custom clock for testing.
func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		s.now = now
	}
}

// WithRNG configures custom random generator for testing.
func WithRNG(rng RNG) Option {
	return func(s *Service) {
		s.rng = rng
	}
}

// WithTransactionProvider sets the transaction provider.
func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

// NewService constructs a new plantation Service.
func NewService(
	characters CharacterRepository,
	inventories InventoryRepository,
	depots DepotRepository,
	plots Repository,
	items coreitem.DefinitionProvider,
	opts ...Option,
) (*Service, error) {
	if characters == nil {
		return nil, errors.New("characters repository is nil")
	}
	if inventories == nil {
		return nil, errors.New("inventories repository is nil")
	}
	if depots == nil {
		return nil, errors.New("depots repository is nil")
	}
	if plots == nil {
		return nil, errors.New("plantation repository is nil")
	}
	if items == nil {
		return nil, errors.New("item definition provider is nil")
	}

	s := &Service{
		characters:  characters,
		inventories: inventories,
		depots:      depots,
		plots:       plots,
		items:       items,
		now:         time.Now,
		rng:         defaultRNG{},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

// GetStatus returns the current plantation status for a character.
func (s *Service) GetStatus(ctx context.Context, characterID string) (StatusResponse, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return StatusResponse{}, ErrInvalidCharacterID
	}

	plot, err := s.plots.GetPlot(ctx, characterID)
	if err != nil && !errors.Is(err, ErrPlotNotFound) {
		return StatusResponse{}, err
	}

	status := StatusNone
	var plotPtr *Plot
	if err == nil {
		plotPtr = &plot
		now := s.now()
		if now.Before(plot.MaturesAt) {
			status = StatusGrowing
		} else {
			status = StatusReady
		}
	}

	dialogueIndex := s.rng.Intn(len(DefaultLotusDialogues))
	dialogue := DefaultLotusDialogues[dialogueIndex]

	return StatusResponse{
		Plot:        plotPtr,
		Status:      status,
		Seeds:       AllSeeds(),
		Fertilizers: AllFertilizers(),
		Dialogue:    dialogue,
	}, nil
}

// Sow seeds a plot for the character after deducting seed purchase cost.
func (s *Service) Sow(ctx context.Context, characterID string, seedID string) (SowResult, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return SowResult{}, ErrInvalidCharacterID
	}

	seed, ok := FindSeed(seedID)
	if !ok {
		return SowResult{}, ErrInvalidSeedID
	}

	var res SowResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Lock Character
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 8: Lock Plantation Plot
		_, err = s.plots.GetPlotForUpdate(txCtx, characterID)
		if err == nil {
			return ErrPlotAlreadySown
		}
		if !errors.Is(err, ErrPlotNotFound) {
			return err
		}

		if !char.HasMoney(seed.Price) {
			return ErrInsufficientGold
		}

		if err := char.DeductMoney(seed.Price); err != nil {
			return ErrInsufficientGold
		}
		if err := s.characters.Update(txCtx, char); err != nil {
			return fmt.Errorf("update character wallet: %w", err)
		}

		now := s.now()
		maturesAt := timer.NextMidnightJST(now)
		plot := Plot{
			CharacterID: characterID,
			SeedID:      seed.ID,
			SownAt:      now,
			MaturesAt:   maturesAt,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := s.plots.SavePlot(txCtx, plot); err != nil {
			return fmt.Errorf("save plantation plot: %w", err)
		}

		res = SowResult{
			Plot:    plot,
			Message: fmt.Sprintf("%sをまいたよ！", seed.Name),
		}
		return nil
	})
	if err != nil {
		return SowResult{}, err
	}

	return res, nil
}

// Fertilize applies fertilizer to an existing plot.
func (s *Service) Fertilize(ctx context.Context, characterID string, fertilizerID string) (FertilizeResult, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return FertilizeResult{}, ErrInvalidCharacterID
	}

	fert, ok := FindFertilizer(fertilizerID)
	if !ok {
		return FertilizeResult{}, ErrInvalidFertilizerID
	}

	var res FertilizeResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Lock Character
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 3: Lock Inventory
		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 5: Lock Depot
		dep, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 8: Lock Plantation Plot
		plot, err := s.plots.GetPlotForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, ErrPlotNotFound) {
				return ErrNoActivePlot
			}
			return err
		}

		if plot.FertilizerID != nil {
			applied, _ := FindFertilizer(*plot.FertilizerID)
			return fmt.Errorf("%w: もう%sをまいてあるよ", ErrFertilizerAlreadyApplied, applied.Name)
		}

		if !fert.IsItem {
			// Purchased with gold
			if !char.HasMoney(fert.Price) {
				return ErrInsufficientGold
			}
			if err := char.DeductMoney(fert.Price); err != nil {
				return ErrInsufficientGold
			}
			if err := s.characters.Update(txCtx, char); err != nil {
				return fmt.Errorf("update character wallet: %w", err)
			}
		} else {
			// Item consumed from Depot first, then Inventory (legacy parity)
			consumedFromDepot := false
			for _, inst := range dep.Items {
				if inst.DefinitionID == fert.ItemID {
					if _, err := dep.Consume(inst.ID, 1); err != nil {
						return fmt.Errorf("consume fertilizer from depot: %w", err)
					}
					consumedFromDepot = true
					break
				}
			}

			if consumedFromDepot {
				if err := s.depots.Save(txCtx, dep); err != nil {
					return fmt.Errorf("save depot: %w", err)
				}
			} else {
				consumedFromInv := false
				for _, inst := range inv.Items {
					if inst.DefinitionID == fert.ItemID {
						if err := inv.Consume(inst.ID, 1); err != nil {
							return fmt.Errorf("consume fertilizer from inventory: %w", err)
						}
						consumedFromInv = true
						break
					}
				}

				if !consumedFromInv {
					return ErrMissingFertilizerItem
				}
				if err := s.inventories.Save(txCtx, inv); err != nil {
					return fmt.Errorf("save inventory: %w", err)
				}
			}
		}

		plot.FertilizerID = &fert.ID
		plot.UpdatedAt = s.now()
		if err := s.plots.SavePlot(txCtx, plot); err != nil {
			return fmt.Errorf("save plantation plot: %w", err)
		}

		res = FertilizeResult{
			Plot:    plot,
			Message: fmt.Sprintf("%sをまくよ！", fert.Name),
		}
		return nil
	})
	if err != nil {
		return FertilizeResult{}, err
	}

	return res, nil
}
