package casino

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
)

type SlotSymbol string

const (
	SymbolCherry SlotSymbol = "Cherry" // ∞
	SymbolNote   SlotSymbol = "Note"   // ♪
	SymbolDagger SlotSymbol = "Dagger" // †
	SymbolStar   SlotSymbol = "Star"   // ★
	SymbolSeven  SlotSymbol = "Seven"  // ７
)

var AllSlotSymbols = []SlotSymbol{
	SymbolCherry,
	SymbolNote,
	SymbolDagger,
	SymbolStar,
	SymbolSeven,
}

var ValidBetRates = map[int64]bool{
	1:   true,
	10:  true,
	50:  true,
	100: true,
	200: true,
}

var (
	ErrInvalidBetRate = errors.New("invalid slot bet rate (allowed: 1, 10, 50, 100, 200)")
)

type SpinResult struct {
	BetCoins    int64         `json:"bet_coins"`
	Reels       [3]SlotSymbol `json:"reels"`
	IsWin       bool          `json:"is_win"`
	Multiplier  int           `json:"multiplier"`
	PayoutCoins int64         `json:"payout_coins"`
	NetCoins    int64         `json:"net_coins"`
	Message     string        `json:"message"`
}

// EvaluateSpin determines payout and multiplier from the 3 reel symbols.
func EvaluateSpin(bet int64, reels [3]SlotSymbol) SpinResult {
	res := SpinResult{
		BetCoins: bet,
		Reels:    reels,
	}

	// 1. Check for 3 matching symbols
	if reels[0] == reels[1] && reels[1] == reels[2] {
		res.IsWin = true
		switch reels[0] {
		case SymbolSeven:
			res.Multiplier = 100
			res.Message = "JACKPOT! Three Sevens (777)!"
		case SymbolStar:
			res.Multiplier = 70
			res.Message = "Super Win! Three Stars (★★★)!"
		case SymbolDagger:
			res.Multiplier = 50
			res.Message = "Big Win! Three Daggers (†††)!"
		case SymbolNote:
			res.Multiplier = 20
			res.Message = "Win! Three Music Notes (♪♪♪)!"
		case SymbolCherry:
			res.Multiplier = 10
			res.Message = "Win! Three Cherries (∞∞∞)!"
		}
		res.PayoutCoins = bet * int64(res.Multiplier)
		res.NetCoins = res.PayoutCoins - bet
		return res
	}

	// 2. Check for 2 matching Cherries (first two reels)
	if reels[0] == SymbolCherry && reels[1] == SymbolCherry {
		res.IsWin = true
		res.Multiplier = 3
		res.Message = "Nice! Two Cherries on the first reels!"
		res.PayoutCoins = bet * int64(res.Multiplier)
		res.NetCoins = res.PayoutCoins - bet
		return res
	}

	// 3. Loss / Miss
	res.IsWin = false
	res.Multiplier = 0
	res.PayoutCoins = 0
	res.NetCoins = -bet
	res.Message = "Miss. Better luck next spin!"
	return res
}

// SpinSlotMachine generates random reels and evaluates the spin outcome.
func SpinSlotMachine(bet int64) (SpinResult, error) {
	if !ValidBetRates[bet] {
		return SpinResult{}, ErrInvalidBetRate
	}

	var reels [3]SlotSymbol
	numSymbols := big.NewInt(int64(len(AllSlotSymbols)))
	for i := 0; i < 3; i++ {
		idxBig, err := rand.Int(rand.Reader, numSymbols)
		if err != nil {
			return SpinResult{}, fmt.Errorf("failed generating random reel: %w", err)
		}
		reels[i] = AllSlotSymbols[idxBig.Int64()]
	}

	return EvaluateSpin(bet, reels), nil
}
