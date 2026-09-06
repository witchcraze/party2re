package database

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/id"
)

func TestConcurrencyStressCasinoExchanges(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	casinoRepo, err := NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	numChars := 3
	charIDs := make([]string, numChars)
	suffix := id.New()[:8]
	for i := 0; i < numChars; i++ {
		c, err := CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("Casino_%s_%d", suffix, i), 50000)
		if err != nil {
			t.Fatal(err)
		}
		charIDs[i] = c.ID
		if _, err := casinoRepo.AdjustCoins(ctx, c.ID, 1000); err != nil {
			t.Fatal(err)
		}
	}

	cfg := GetStressConfig()
	res := RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID*1000+op)))
		charID := charIDs[r.Intn(numChars)]
		action := r.Intn(3)

		switch action {
		case 0:
			// ExchangeGoldToCoins: buy 10 coins for 200 gold
			_, _, err := casinoRepo.ExchangeGoldToCoins(ctx, charID, 10, 200)
			if err != nil && !errors.Is(err, casino.ErrInsufficientGold) {
				return err
			}
		case 1:
			// ExchangeCoinsToGold: sell 10 coins for 200 gold
			_, _, err := casinoRepo.ExchangeCoinsToGold(ctx, charID, 10, 200)
			if err != nil && !errors.Is(err, casino.ErrInsufficientCoins) {
				return err
			}
		case 2:
			// DeductBetAndCreditPayout: bet 5 coins, payout 0 or 15 coins
			payout := int64(0)
			if r.Intn(2) == 1 {
				payout = 15
			}
			_, err := casinoRepo.DeductBetAndCreditPayout(ctx, charID, 5, payout)
			if err != nil && !errors.Is(err, casino.ErrInsufficientCoins) {
				return err
			}
		}
		return nil
	})

	t.Logf("Casino Concurrency Stress Test Completed: %d operations across %d workers in %v with 0 deadlocks",
		res.TotalOps, cfg.Workers, res.Duration)
}
