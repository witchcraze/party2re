package casino

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
)

type DoppelMark string

const (
	MarkStar             DoppelMark = "★"
	MarkCircle           DoppelMark = "●"
	MarkDiamond          DoppelMark = "◆"
	MarkNote             DoppelMark = "♪"
	MarkSquare           DoppelMark = "■"
	MarkTriangle         DoppelMark = "▲"
	MarkDagger           DoppelMark = "†"
	MarkInvertedTriangle DoppelMark = "▼"
)

var AllDoppelMarks = []DoppelMark{
	MarkStar,
	MarkCircle,
	MarkDiamond,
	MarkNote,
	MarkSquare,
	MarkTriangle,
	MarkDagger,
	MarkInvertedTriangle,
}

var ValidPoolSizes = map[int]bool{
	4: true,
	6: true,
	8: true,
}

var (
	ErrInvalidPoolSize   = errors.New("invalid doppel pool size (allowed: 4, 6, 8)")
	ErrInvalidDoppelMark = errors.New("invalid doppel mark for selected pool")
	ErrInvalidDoppelBet  = errors.New("doppel bet must be between 1 and 5000 coins")
)

type DoppelResult struct {
	BetCoins    int64      `json:"bet_coins"`
	PoolSize    int        `json:"pool_size"`
	PlayerMark  DoppelMark `json:"player_mark"`
	DoppelMark  DoppelMark `json:"doppel_mark"`
	IsWin       bool       `json:"is_win"`
	Multiplier  int        `json:"multiplier"`
	PayoutCoins int64      `json:"payout_coins"`
	NetCoins    int64      `json:"net_coins"`
	Message     string     `json:"message"`
}

// GetAvailableMarks returns the slice of marks available for the given pool size.
func GetAvailableMarks(poolSize int) ([]DoppelMark, error) {
	if !ValidPoolSizes[poolSize] {
		return nil, ErrInvalidPoolSize
	}
	return AllDoppelMarks[:poolSize], nil
}

// EvaluateDoppel calculates the outcome comparing player mark and doppelganger mark.
func EvaluateDoppel(betCoins int64, poolSize int, playerMark, doppelMark DoppelMark) DoppelResult {
	res := DoppelResult{
		BetCoins:   betCoins,
		PoolSize:   poolSize,
		PlayerMark: playerMark,
		DoppelMark: doppelMark,
	}

	if playerMark == doppelMark {
		res.IsWin = true
		res.Multiplier = poolSize
		res.PayoutCoins = betCoins * int64(poolSize)
		res.NetCoins = res.PayoutCoins - betCoins
		res.Message = fmt.Sprintf("DOPPEL MATCH! Both chose 【%s】! You win %d coins (%dx multiplier)!", playerMark, res.PayoutCoins, poolSize)
	} else {
		res.IsWin = false
		res.Multiplier = 0
		res.PayoutCoins = 0
		res.NetCoins = -betCoins
		res.Message = fmt.Sprintf("Miss! Player chose 【%s】 but Doppelganger chose 【%s】.", playerMark, doppelMark)
	}
	return res
}

// PlayDoppelGame chooses a random mark for the Doppelganger from the pool and evaluates the game.
func PlayDoppelGame(betCoins int64, poolSize int, playerMark DoppelMark) (DoppelResult, error) {
	if betCoins < MinBaseRate || betCoins > MaxBaseRate {
		return DoppelResult{}, ErrInvalidDoppelBet
	}
	available, err := GetAvailableMarks(poolSize)
	if err != nil {
		return DoppelResult{}, err
	}

	// Validate player mark is within active pool
	validMark := false
	for _, m := range available {
		if m == playerMark {
			validMark = true
			break
		}
	}
	if !validMark {
		return DoppelResult{}, ErrInvalidDoppelMark
	}

	// Randomly select Doppelganger mark from pool
	idxBig, err := rand.Int(rand.Reader, big.NewInt(int64(poolSize)))
	if err != nil {
		return DoppelResult{}, fmt.Errorf("failed selecting random doppel mark: %w", err)
	}
	doppelMark := available[idxBig.Int64()]

	return EvaluateDoppel(betCoins, poolSize, playerMark, doppelMark), nil
}
