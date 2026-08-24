package casino

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
)

type Action string

const (
	ActionCall     Action = "call"     // つづける: match current bet and continue
	ActionShowdown Action = "showdown" // しょうぶ: match current bet and force showdown
	ActionFold     Action = "fold"     // おりる: forfeit hand
)

func (a Action) Valid() bool {
	switch a {
	case ActionCall, ActionShowdown, ActionFold:
		return true
	default:
		return false
	}
}

type GameStatus string

const (
	StatusInProgress   GameStatus = "in_progress"
	StatusPlayerWon    GameStatus = "player_won"
	StatusDealerWon    GameStatus = "dealer_won"
	StatusTie          GameStatus = "tie"
	StatusPlayerFolded GameStatus = "player_folded"
	StatusDealerFolded GameStatus = "dealer_folded"
)

const (
	DefaultMaxRounds = 5
	MinBaseRate      = 1
	MaxBaseRate      = 5000
)

var (
	ErrInvalidBaseRate  = errors.New("base rate must be between 1 and 5000")
	ErrGameAlreadyOver  = errors.New("game is already finished")
	ErrInvalidAction    = errors.New("invalid poker action")
	ErrInsufficientCoin = errors.New("insufficient coins for bet")
)

type IndianPokerGame struct {
	BaseRate             int64      `json:"base_rate"`
	MaxRounds            int        `json:"max_rounds"`
	Round                int        `json:"round"`
	CurrentBet           int64      `json:"current_bet"`
	PlayerCard           Card       `json:"player_card"` // Hidden from player in UI
	DealerCard           Card       `json:"dealer_card"` // Visible to player
	PlayerCommittedCoins int64      `json:"player_committed_coins"`
	DealerCommittedCoins int64      `json:"dealer_committed_coins"`
	Pot                  int64      `json:"pot"`
	Status               GameStatus `json:"status"`
	Winner               string     `json:"winner,omitempty"`
	PayoutCoins          int64      `json:"payout_coins"`
	Logs                 []string   `json:"logs"`
}

// NewIndianPokerGame starts a new game dealing 1 card to Player and 1 card to Dealer.
func NewIndianPokerGame(baseRate int64) (*IndianPokerGame, error) {
	if baseRate < MinBaseRate || baseRate > MaxBaseRate {
		return nil, ErrInvalidBaseRate
	}

	deck := NewStandardDeck()
	if err := deck.Shuffle(); err != nil {
		return nil, err
	}

	playerCard, err := deck.Draw()
	if err != nil {
		return nil, err
	}
	dealerCard, err := deck.Draw()
	if err != nil {
		return nil, err
	}

	// Initial Ante: Both player and dealer commit BaseRate
	initialBet := baseRate
	game := &IndianPokerGame{
		BaseRate:             baseRate,
		MaxRounds:            DefaultMaxRounds,
		Round:                1,
		CurrentBet:           initialBet,
		PlayerCard:           playerCard,
		DealerCard:           dealerCard,
		PlayerCommittedCoins: initialBet,
		DealerCommittedCoins: initialBet,
		Pot:                  initialBet * 2,
		Status:               StatusInProgress,
		Logs: []string{
			fmt.Sprintf("Game started with rate %d coins.", baseRate),
			fmt.Sprintf("Round 1: Dealer card is %s (Player card is hidden).", dealerCard),
		},
	}
	return game, nil
}

// PlayRound processes the player's action and the dealer's response.
func (g *IndianPokerGame) PlayRound(playerAction Action, playerAvailableCoins int64) error {
	if g.Status != StatusInProgress {
		return ErrGameAlreadyOver
	}
	if !playerAction.Valid() {
		return ErrInvalidAction
	}

	// 1. Handle Player Fold
	if playerAction == ActionFold {
		g.Status = StatusPlayerFolded
		g.Winner = "dealer"
		g.PayoutCoins = 0
		g.Logs = append(g.Logs, "Player folded. Dealer wins the pot.")
		return nil
	}

	// 2. Check if player has enough coins for current round bet
	neededCoins := g.CurrentBet
	if playerAvailableCoins < neededCoins {
		return ErrInsufficientCoin
	}

	// 3. Player commits coins for this round
	g.PlayerCommittedCoins += neededCoins
	g.Pot += neededCoins
	g.Logs = append(g.Logs, fmt.Sprintf("Player chose %s and bet %d coins.", playerAction, neededCoins))

	// 4. Dealer decides action based on visible Player card
	dealerAction := g.decideDealerAction()
	g.Logs = append(g.Logs, fmt.Sprintf("Dealer chose %s.", dealerAction))

	if dealerAction == ActionFold {
		g.Status = StatusDealerFolded
		g.Winner = "player"
		g.PayoutCoins = g.Pot
		g.Logs = append(g.Logs, fmt.Sprintf("Dealer folded! Player wins total pot of %d coins.", g.Pot))
		return nil
	}

	// Dealer commits matching coins
	g.DealerCommittedCoins += neededCoins
	g.Pot += neededCoins

	// 5. Evaluate Showdown conditions
	// Showdown occurs if:
	// - Player chose ActionShowdown, OR
	// - Dealer chose ActionShowdown, OR
	// - Max rounds reached
	if playerAction == ActionShowdown || dealerAction == ActionShowdown || g.Round >= g.MaxRounds {
		g.resolveShowdown()
		return nil
	}

	// 6. Otherwise advance to next round
	g.Round++
	g.CurrentBet = g.BaseRate * int64(g.Round)
	g.Logs = append(g.Logs, fmt.Sprintf("Advancing to Round %d. Next bet: %d coins.", g.Round, g.CurrentBet))
	return nil
}

// decideDealerAction implements the Dealer AI based on visible player card rank.
func (g *IndianPokerGame) decideDealerAction() Action {
	// Dealer sees player's card
	playerRank := g.PlayerCard.Rank

	// Random factor for slight unpredictability / bluffing (0..99)
	randValBig, _ := rand.Int(rand.Reader, big.NewInt(100))
	randVal := int(randValBig.Int64())

	switch {
	case playerRank == RankKing:
		// Player has King (highest rank). Dealer can at best tie if Dealer has King, otherwise loses.
		if randVal < 80 {
			return ActionFold
		}
		return ActionCall
	case playerRank >= RankJack:
		// Player has Queen or Jack (high rank)
		if randVal < 45 {
			return ActionFold
		} else if randVal < 85 {
			return ActionCall
		}
		return ActionShowdown
	case playerRank <= RankThree:
		// Player has very low rank (Ace, 2, 3). High probability dealer has better card.
		if randVal < 60 {
			return ActionShowdown
		}
		return ActionCall
	default:
		// Mid ranks (4..10)
		if g.Round >= 3 && randVal < 40 {
			return ActionShowdown
		}
		return ActionCall
	}
}

// resolveShowdown determines the winner by comparing card ranks.
func (g *IndianPokerGame) resolveShowdown() {
	g.Logs = append(g.Logs, fmt.Sprintf("--- SHOWDOWN --- Player card: %s vs Dealer card: %s", g.PlayerCard, g.DealerCard))

	if g.PlayerCard.Rank > g.DealerCard.Rank {
		g.Status = StatusPlayerWon
		g.Winner = "player"
		g.PayoutCoins = g.Pot
		g.Logs = append(g.Logs, fmt.Sprintf("Player has higher rank (%s > %s). Player wins %d coins!", g.PlayerCard.Rank, g.DealerCard.Rank, g.Pot))
	} else if g.DealerCard.Rank > g.PlayerCard.Rank {
		g.Status = StatusDealerWon
		g.Winner = "dealer"
		g.PayoutCoins = 0
		g.Logs = append(g.Logs, fmt.Sprintf("Dealer has higher rank (%s > %s). Dealer wins the pot.", g.DealerCard.Rank, g.PlayerCard.Rank))
	} else {
		// Tie rank: pot split / returned
		g.Status = StatusTie
		g.Winner = "tie"
		g.PayoutCoins = g.PlayerCommittedCoins
		g.Logs = append(g.Logs, fmt.Sprintf("Equal card ranks (%s == %s). It's a tie! %d coins returned to player.", g.PlayerCard.Rank, g.DealerCard.Rank, g.PayoutCoins))
	}
}
