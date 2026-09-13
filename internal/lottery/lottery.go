package lottery

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

const (
	RaffleTicketCostGold = 100
	StandardRaffleCost   = 3
	SpecialRaffleCost    = 300

	PrizeTier1st   = "1ST_PRIZE"
	PrizeTier2nd   = "2ND_PRIZE"
	PrizeTier3rd   = "3RD_PRIZE"
	PrizeTier4th   = "4TH_PRIZE"
	PrizeTierGrand = "GRAND_PRIZE"
	PrizeTierMiss  = "MISS"
)

var (
	ErrInvalidAmount       = errors.New("invalid amount or quantity")
	ErrInsufficientGold    = errors.New("insufficient gold")
	ErrInsufficientTickets = errors.New("insufficient raffle tickets")
)

type RaffleType string

const (
	RaffleStandard RaffleType = "STANDARD"
	RaffleSpecial  RaffleType = "SPECIAL"
)

type RafflePrize struct {
	Tier        string `json:"tier"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	RewardGold  int    `json:"reward_gold"`
	Description string `json:"description"`
}

type RaffleResult struct {
	RaffleType  RaffleType  `json:"raffle_type"`
	TicketsUsed int         `json:"tickets_used"`
	Roll        int         `json:"roll"`
	Prize       RafflePrize `json:"prize"`
}

// EvaluateRaffleRoll deterministically returns the prize for a roll.
func EvaluateRaffleRoll(raffleType RaffleType, roll int) RafflePrize {
	if raffleType == RaffleSpecial {
		switch {
		case roll < 3:
			return RafflePrize{Tier: PrizeTierGrand, Name: "Gold Orb", Color: "gold", RewardGold: 100000, Description: "Legendary Gold Orb Grand Prize!"}
		case roll < 15:
			return RafflePrize{Tier: PrizeTier1st, Name: "Silver Orb", Color: "silver", RewardGold: 20000, Description: "Miraculous Silver Orb!"}
		case roll < 30:
			return RafflePrize{Tier: PrizeTier2nd, Name: "Red Orb", Color: "red", RewardGold: 10000, Description: "Brilliant Red Orb!"}
		case roll < 40:
			return RafflePrize{Tier: PrizeTier3rd, Name: "Blue Orb", Color: "blue", RewardGold: 5000, Description: "Mystic Blue Orb!"}
		case roll < 50:
			return RafflePrize{Tier: PrizeTier4th, Name: "Green Orb", Color: "green", RewardGold: 3000, Description: "Verdant Green Orb!"}
		case roll < 60:
			return RafflePrize{Tier: "5TH_PRIZE", Name: "Yellow Orb", Color: "yellow", RewardGold: 2000, Description: "Radiant Yellow Orb!"}
		case roll < 70:
			return RafflePrize{Tier: "6TH_PRIZE", Name: "Purple Orb", Color: "purple", RewardGold: 1000, Description: "Deep Purple Orb!"}
		default:
			return RafflePrize{Tier: PrizeTierMiss, Name: "White Orb", Color: "white", RewardGold: 0, Description: "Miss... White Orb."}
		}
	}

	// Standard Raffle (out of 1000)
	switch {
	case roll < 1:
		return RafflePrize{Tier: PrizeTierGrand, Name: "Grand Golden Slime", Color: "gold", RewardGold: 5000, Description: "JACKPOT! Special Grand Prize!"}
	case roll < 4:
		return RafflePrize{Tier: PrizeTier1st, Name: "Red Slime", Color: "red", RewardGold: 2500, Description: "First Prize Red Slime!"}
	case roll < 8:
		return RafflePrize{Tier: PrizeTier2nd, Name: "Purple Slime", Color: "purple", RewardGold: 1000, Description: "Second Prize Purple Slime!"}
	case roll < 14:
		return RafflePrize{Tier: PrizeTier3rd, Name: "Yellow Slime", Color: "yellow", RewardGold: 500, Description: "Third Prize Yellow Slime!"}
	case roll < 45:
		return RafflePrize{Tier: PrizeTier4th, Name: "Pink Slime", Color: "pink", RewardGold: 200, Description: "Fourth Prize Pink Slime!"}
	case roll < 55:
		return RafflePrize{Tier: "5TH_PRIZE", Name: "Blue Slime", Color: "blue", RewardGold: 100, Description: "Fifth Prize Blue Slime!"}
	case roll < 75:
		return RafflePrize{Tier: "6TH_PRIZE", Name: "Green Slime", Color: "green", RewardGold: 50, Description: "Sixth Prize Green Slime!"}
	default:
		return RafflePrize{Tier: PrizeTierMiss, Name: "White Slime", Color: "white", RewardGold: 0, Description: "Miss... White Slime."}
	}
}

// RaffleRepository defines data access for character raffle tickets (Fukubiki).
type RaffleRepository interface {
	GetRaffleTickets(ctx context.Context, characterID string) (int, error)
	BuyRaffleTickets(ctx context.Context, characterID string, count int, goldCost int) (int, corecharacter.Character, error)
	UseRaffleTickets(ctx context.Context, characterID string, count int, rewardGold int) (int, corecharacter.Character, error)
}

// TakarakujiRepository defines data access for periodic Takarakuji lottery rounds and tickets.
type TakarakujiRepository interface {
	GetActiveTakarakujiRound(ctx context.Context) (TakarakujiRound, error)
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

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type CollectionRecorder interface {
	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
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
	depotRepo          DepotRepository
	charRepo           CharacterRepository
	itemDefProvider    ItemDefinitionProvider
	collectionRecorder CollectionRecorder
	clock              Clock
}

type Option func(*Service)

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

func (s *Service) GetRaffleTickets(ctx context.Context, characterID string) (int, error) {
	if characterID == "" {
		return 0, corecharacter.ErrNotFound
	}
	return s.repo.GetRaffleTickets(ctx, characterID)
}

func (s *Service) BuyRaffleTickets(ctx context.Context, characterID string, count int) (int, corecharacter.Character, error) {
	if count <= 0 {
		return 0, corecharacter.Character{}, ErrInvalidAmount
	}
	goldCost := count * RaffleTicketCostGold
	return s.repo.BuyRaffleTickets(ctx, characterID, count, goldCost)
}

func (s *Service) PlayRaffle(ctx context.Context, characterID string, raffleType RaffleType) (RaffleResult, int, corecharacter.Character, error) {
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

	prize := EvaluateRaffleRoll(raffleType, roll)
	remainingTickets, char, err := s.repo.UseRaffleTickets(ctx, characterID, cost, prize.RewardGold)
	if err != nil {
		return RaffleResult{}, tickets, corecharacter.Character{}, err
	}

	res := RaffleResult{
		RaffleType:  raffleType,
		TicketsUsed: cost,
		Roll:        roll,
		Prize:       prize,
	}
	return res, remainingTickets, char, nil
}
