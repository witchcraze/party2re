package lottery

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

const (
	RaffleTicketCostGold  = 100
	StandardRaffleCost    = 3
	SpecialRaffleCost     = 300
	LotteryTicketCostGold = 300

	PrizeTier1st   = "1ST_PRIZE"
	PrizeTier2nd   = "2ND_PRIZE"
	PrizeTier3rd   = "3RD_PRIZE"
	PrizeTier4th   = "4TH_PRIZE"
	PrizeTierGrand = "GRAND_PRIZE"
	PrizeTierMiss  = "MISS"
)

var (
	ErrInvalidAmount        = errors.New("invalid amount or quantity")
	ErrInsufficientGold     = errors.New("insufficient gold")
	ErrInsufficientTickets  = errors.New("insufficient raffle tickets")
	ErrInvalidTicketNumber  = errors.New("invalid 4-digit ticket number (0000-9999)")
	ErrTicketAlreadyClaimed = errors.New("ticket already claimed")
	ErrDrawingNotSettled    = errors.New("lottery drawing not yet settled")
	ErrTicketNotFound       = errors.New("lottery ticket not found")
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

type LotteryTicket struct {
	ID           string     `json:"id"`
	CharacterID  string     `json:"character_id"`
	RoundID      int        `json:"round_id"`
	TicketNumber string     `json:"ticket_number"`
	PurchasedAt  time.Time  `json:"purchased_at"`
	Claimed      bool       `json:"claimed"`
	PrizeTier    string     `json:"prize_tier"`
	PrizeGold    int        `json:"prize_gold"`
	ClaimedAt    *time.Time `json:"claimed_at,omitempty"`
}

type LotteryDrawing struct {
	RoundID       int       `json:"round_id"`
	WinningNumber string    `json:"winning_number"`
	DrawnAt       time.Time `json:"drawn_at"`
	IsSettled     bool      `json:"is_settled"`
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

// EvaluateLotteryTicket matches player ticket number with winning number and calculates prize.
func EvaluateLotteryTicket(ticketNumber, winningNumber string) (tier string, prizeGold int) {
	if len(ticketNumber) != 4 || len(winningNumber) != 4 {
		return PrizeTierMiss, 0
	}

	if ticketNumber == winningNumber {
		return PrizeTier1st, 100000
	}
	if ticketNumber[1:] == winningNumber[1:] {
		return PrizeTier2nd, 10000
	}
	if ticketNumber[2:] == winningNumber[2:] {
		return PrizeTier3rd, 1000
	}
	if ticketNumber[3:] == winningNumber[3:] {
		return PrizeTier4th, 300
	}
	return PrizeTierMiss, 0
}

// GenerateRandom4Digit returns a random 4-digit numeric string ("0000" to "9999").
func GenerateRandom4Digit() (string, error) {
	nBig, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d", nBig.Int64()), nil
}

// Repository defines data access for character raffle tickets, lottery tickets, and drawings.
type Repository interface {
	GetRaffleTickets(ctx context.Context, characterID string) (int, error)
	BuyRaffleTickets(ctx context.Context, characterID string, count int, goldCost int) (int, corecharacter.Character, error)
	UseRaffleTickets(ctx context.Context, characterID string, count int, rewardGold int) (int, corecharacter.Character, error)
	PurchaseLotteryTicket(ctx context.Context, ticket LotteryTicket, goldCost int) (LotteryTicket, corecharacter.Character, error)
	GetLotteryTicket(ctx context.Context, ticketID string) (LotteryTicket, error)
	ListLotteryTickets(ctx context.Context, characterID string, roundID int) ([]LotteryTicket, error)
	SaveDrawing(ctx context.Context, drawing LotteryDrawing) error
	GetDrawing(ctx context.Context, roundID int) (LotteryDrawing, error)
	ClaimLotteryTicket(ctx context.Context, ticketID string, tier string, prizeGold int) (LotteryTicket, corecharacter.Character, error)
}

// Service provides high-level lottery and raffle operations.
type Service struct {
	repo Repository
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) GetRaffleTickets(ctx context.Context, characterID string) (int, error) {
	if characterID == "" {
		return 0, corecharacter.ErrNotFound
	}
	return s.repo.GetRaffleTickets(ctx, characterID)
}

func (s *Service) ListLotteryTickets(ctx context.Context, characterID string, roundID int) ([]LotteryTicket, error) {
	if characterID == "" {
		return nil, corecharacter.ErrNotFound
	}
	return s.repo.ListLotteryTickets(ctx, characterID, roundID)
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

func (s *Service) PurchaseLotteryTicket(ctx context.Context, characterID string, roundID int, number string) (LotteryTicket, corecharacter.Character, error) {
	if len(number) != 4 {
		return LotteryTicket{}, corecharacter.Character{}, ErrInvalidTicketNumber
	}
	for _, ch := range number {
		if ch < '0' || ch > '9' {
			return LotteryTicket{}, corecharacter.Character{}, ErrInvalidTicketNumber
		}
	}

	ticket := LotteryTicket{
		CharacterID:  characterID,
		RoundID:      roundID,
		TicketNumber: number,
		PurchasedAt:  time.Now().UTC(),
	}
	return s.repo.PurchaseLotteryTicket(ctx, ticket, LotteryTicketCostGold)
}

func (s *Service) SettleDrawing(ctx context.Context, roundID int, winningNumber string) (LotteryDrawing, error) {
	if len(winningNumber) != 4 {
		return LotteryDrawing{}, ErrInvalidTicketNumber
	}
	drawing := LotteryDrawing{
		RoundID:       roundID,
		WinningNumber: winningNumber,
		DrawnAt:       time.Now().UTC(),
		IsSettled:     true,
	}
	if err := s.repo.SaveDrawing(ctx, drawing); err != nil {
		return LotteryDrawing{}, err
	}
	return drawing, nil
}

func (s *Service) ClaimLotteryTicket(ctx context.Context, characterID, ticketID string) (LotteryTicket, corecharacter.Character, error) {
	ticket, err := s.repo.GetLotteryTicket(ctx, ticketID)
	if err != nil {
		return LotteryTicket{}, corecharacter.Character{}, err
	}
	if ticket.CharacterID != characterID {
		return LotteryTicket{}, corecharacter.Character{}, ErrTicketNotFound
	}
	if ticket.Claimed {
		return ticket, corecharacter.Character{}, ErrTicketAlreadyClaimed
	}

	drawing, err := s.repo.GetDrawing(ctx, ticket.RoundID)
	if err != nil {
		return ticket, corecharacter.Character{}, err
	}
	if !drawing.IsSettled {
		return ticket, corecharacter.Character{}, ErrDrawingNotSettled
	}

	tier, prizeGold := EvaluateLotteryTicket(ticket.TicketNumber, drawing.WinningNumber)
	return s.repo.ClaimLotteryTicket(ctx, ticketID, tier, prizeGold)
}
