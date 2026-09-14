package lottery

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

var (
	ErrInvalidAmount          = errors.New("invalid amount or quantity")
	ErrInsufficientGold       = errors.New("insufficient gold")
	ErrInsufficientTickets    = errors.New("insufficient raffle tickets")
	ErrDepotNotConfigured     = errors.New("depot repository not configured")
	ErrInventoryNotConfigured = errors.New("inventory repository not configured")
)

// RaffleRepository defines data access for character raffle tickets (Fukubiki).
type RaffleRepository interface {
	GetRaffleTickets(ctx context.Context, characterID string) (int, error)
	UseRaffleTickets(ctx context.Context, characterID string, count int) (int, error)
}

// TakarakujiRepository defines data access for periodic Takarakuji lottery rounds and tickets.
type TakarakujiRepository interface {
	GetActiveTakarakujiRound(ctx context.Context) (TakarakujiRound, error)
	GetActiveTakarakujiRoundForUpdate(ctx context.Context) (TakarakujiRound, error)
	CreateTakarakujiRound(ctx context.Context, round TakarakujiRound) (TakarakujiRound, error)
	CountTakarakujiTickets(ctx context.Context, roundID int) (int, error)
	HasCharacterPurchasedTakarakuji(ctx context.Context, roundID int, characterID string) (bool, error)
	PurchaseTakarakujiTicket(ctx context.Context, roundID int, characterID string, goldCost int) (TakarakujiTicket, corecharacter.Character, error)
	GetCharacterTakarakujiTicket(ctx context.Context, roundID int, characterID string) (TakarakujiTicket, error)
	ListCharacterTakarakujiTickets(ctx context.Context, characterID string) ([]TakarakujiTicket, error)
	ListRoundTakarakujiTickets(ctx context.Context, roundID int) ([]TakarakujiTicket, error)
	SettleTakarakujiRound(ctx context.Context, roundID int, drawnAt time.Time, winningTickets []TakarakujiTicket) error
}

// Repository is the composite persistence interface for lottery and raffle systems.
type Repository interface {
	RaffleRepository
	TakarakujiRepository
}

type ItemDefinitionProvider interface {
	FindByID(id string) (coreitem.Definition, error)
}

type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, value depot.Depot) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
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

// Service provides high-level lottery and raffle operations.
type Service struct {
	repo               Repository
	inventoryRepo      InventoryRepository
	depotRepo          DepotRepository
	charRepo           CharacterRepository
	itemDefProvider    ItemDefinitionProvider
	collectionRecorder CollectionRecorder
	txProvider         TransactionProvider
	clock              Clock
}

type Option func(*Service)

func WithInventoryRepository(r InventoryRepository) Option {
	return func(s *Service) { s.inventoryRepo = r }
}

func WithDepotRepository(r DepotRepository) Option {
	return func(s *Service) { s.depotRepo = r }
}

func WithCharacterRepository(r CharacterRepository) Option {
	return func(s *Service) { s.charRepo = r }
}

func WithItemDefinitionProvider(p ItemDefinitionProvider) Option {
	return func(s *Service) { s.itemDefProvider = p }
}

func WithCollectionRecorder(r CollectionRecorder) Option {
	return func(s *Service) { s.collectionRecorder = r }
}

func WithTransactionProvider(tp TransactionProvider) Option {
	return func(s *Service) { s.txProvider = tp }
}

func WithClock(c Clock) Option {
	return func(s *Service) { s.clock = c }
}

func NewService(repo Repository, opts ...Option) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	s := &Service{
		repo:  repo,
		clock: realClock{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock.Now()
	}
	return time.Now().UTC()
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) GetRaffleTickets(ctx context.Context, characterID string) (int, error) {
	if characterID == "" {
		return 0, corecharacter.ErrNotFound
	}
	return s.repo.GetRaffleTickets(ctx, characterID)
}

func (s *Service) PlayRaffle(ctx context.Context, characterID string, raffleType RaffleType) (RaffleResult, int, corecharacter.Character, error) {
	if characterID == "" {
		return RaffleResult{}, 0, corecharacter.Character{}, corecharacter.ErrNotFound
	}

	cost := StandardRaffleCost
	maxRoll := int64(1000)
	if raffleType == RaffleSpecial {
		cost = SpecialRaffleCost
		maxRoll = 100
	}

	tickets, err := s.repo.GetRaffleTickets(ctx, characterID)
	if err != nil {
		return RaffleResult{}, 0, corecharacter.Character{}, err
	}
	if tickets < cost {
		return RaffleResult{}, tickets, corecharacter.Character{}, ErrInsufficientTickets
	}

	rollBig, err := rand.Int(rand.Reader, big.NewInt(maxRoll))
	if err != nil {
		return RaffleResult{}, tickets, corecharacter.Character{}, fmt.Errorf("failed rolling raffle: %w", err)
	}
	roll := int(rollBig.Int64())

	subRollBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(SpecialGrandPrizeItems))))
	if err != nil {
		return RaffleResult{}, tickets, corecharacter.Character{}, fmt.Errorf("failed rolling special prize: %w", err)
	}
	subRoll := int(subRollBig.Int64())

	now := s.now()
	jst := time.FixedZone("JST", 9*60*60)
	wday := int(now.In(jst).Weekday())

	prize := EvaluateRaffleRoll(raffleType, roll, wday, subRoll)

	var (
		remainingTickets   int
		updatedChar        corecharacter.Character
		transferredToDepot bool
	)

	err = s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character (Rank 2)
		if s.charRepo != nil {
			var charErr error
			updatedChar, charErr = s.charRepo.FindByIDForUpdate(txCtx, characterID)
			if charErr != nil {
				return charErr
			}
		}

		// 2. Lock Inventory (Rank 3) if prize grants an item
		var inv coreinventory.Inventory
		occupied := false
		if prize.ItemDefinitionID != "" {
			if s.inventoryRepo == nil {
				return ErrInventoryNotConfigured
			}
			var invErr error
			inv, invErr = s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, characterID)
			if invErr != nil {
				return invErr
			}

			for _, inst := range inv.Items {
				if s.itemDefProvider != nil {
					def, err := s.itemDefProvider.FindByID(inst.DefinitionID)
					if err == nil && (def.Slot == coreitem.SlotNone || def.Slot == "" || strings.HasPrefix(inst.DefinitionID, "item-")) {
						occupied = true
						break
					}
				} else {
					if strings.HasPrefix(inst.DefinitionID, "item-") {
						occupied = true
						break
					}
				}
			}
		}

		// 3. Lock Depot (Rank 5) if item must go to depot
		var dep depot.Depot
		if prize.ItemDefinitionID != "" && occupied {
			if s.depotRepo == nil {
				return ErrDepotNotConfigured
			}
			var depErr error
			dep, depErr = s.depotRepo.FindByCharacterIDForUpdate(txCtx, characterID)
			if depErr != nil {
				if !errors.Is(depErr, depot.ErrNotFound) {
					return depErr
				}
				dep, depErr = depot.NewDepotWithCapacity(characterID, updatedChar.JobLevel, 0, updatedChar.OverDepot)
				if depErr != nil {
					return depErr
				}
			}
			dep.RefreshCapacity(updatedChar.JobLevel, updatedChar.OverDepot)
		}

		// 4. Consume raffle tickets (Rank 8 / Secondary record)
		var ticketErr error
		remainingTickets, ticketErr = s.repo.UseRaffleTickets(txCtx, characterID, cost)
		if ticketErr != nil {
			return ticketErr
		}

		// 5. Deliver item if won
		if prize.ItemDefinitionID != "" {
			inst, err := coreitem.NewInstance(prize.ItemDefinitionID, 1)
			if err != nil {
				return err
			}

			if !occupied {
				if err := inv.Add(inst); err != nil {
					return err
				}
				if err := s.inventoryRepo.Save(txCtx, inv); err != nil {
					return err
				}
				transferredToDepot = false
			} else {
				if err := dep.AddItem(inst); err != nil {
					return err
				}
				if err := s.depotRepo.Save(txCtx, dep); err != nil {
					return err
				}
				transferredToDepot = true
			}

			if s.collectionRecorder != nil {
				_ = s.collectionRecorder.RecordItemDiscovered(txCtx, characterID, prize.ItemDefinitionID, prize.Name, "item")
			}
		}

		return nil
	})
	if err != nil {
		return RaffleResult{}, tickets, corecharacter.Character{}, err
	}

	remainingAttempts := remainingTickets / cost
	message := FormatLegacyRaffleMessage(raffleType, prize, transferredToDepot, remainingAttempts)

	res := RaffleResult{
		RaffleType:         raffleType,
		TicketsUsed:        cost,
		Roll:               roll,
		Prize:              prize,
		TransferredToDepot: transferredToDepot,
		Message:            message,
	}
	return res, remainingTickets, updatedChar, nil
}
