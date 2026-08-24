package casino_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestEvaluateDoppel(t *testing.T) {
	tests := []struct {
		name           string
		bet            int64
		poolSize       int
		playerMark     casino.DoppelMark
		doppelMark     casino.DoppelMark
		wantWin        bool
		wantMultiplier int
		wantPayout     int64
		wantNet        int64
	}{
		{
			name:           "Pool 4 Match Star",
			bet:            10,
			poolSize:       4,
			playerMark:     casino.MarkStar,
			doppelMark:     casino.MarkStar,
			wantWin:        true,
			wantMultiplier: 4,
			wantPayout:     40,
			wantNet:        30,
		},
		{
			name:           "Pool 6 Match Diamond",
			bet:            50,
			poolSize:       6,
			playerMark:     casino.MarkDiamond,
			doppelMark:     casino.MarkDiamond,
			wantWin:        true,
			wantMultiplier: 6,
			wantPayout:     300,
			wantNet:        250,
		},
		{
			name:           "Pool 8 Match Inverted Triangle",
			bet:            100,
			poolSize:       8,
			playerMark:     casino.MarkInvertedTriangle,
			doppelMark:     casino.MarkInvertedTriangle,
			wantWin:        true,
			wantMultiplier: 8,
			wantPayout:     800,
			wantNet:        700,
		},
		{
			name:           "Mismatch / Loss",
			bet:            20,
			poolSize:       4,
			playerMark:     casino.MarkStar,
			doppelMark:     casino.MarkCircle,
			wantWin:        false,
			wantMultiplier: 0,
			wantPayout:     0,
			wantNet:        -20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := casino.EvaluateDoppel(tt.bet, tt.poolSize, tt.playerMark, tt.doppelMark)
			if res.IsWin != tt.wantWin {
				t.Errorf("IsWin = %v, want %v", res.IsWin, tt.wantWin)
			}
			if res.Multiplier != tt.wantMultiplier {
				t.Errorf("Multiplier = %d, want %d", res.Multiplier, tt.wantMultiplier)
			}
			if res.PayoutCoins != tt.wantPayout {
				t.Errorf("PayoutCoins = %d, want %d", res.PayoutCoins, tt.wantPayout)
			}
			if res.NetCoins != tt.wantNet {
				t.Errorf("NetCoins = %d, want %d", res.NetCoins, tt.wantNet)
			}
		})
	}
}

func TestPlayDoppelGame_Validation(t *testing.T) {
	t.Run("Invalid pool size", func(t *testing.T) {
		_, err := casino.PlayDoppelGame(10, 5, casino.MarkStar)
		if err != casino.ErrInvalidPoolSize {
			t.Errorf("expected ErrInvalidPoolSize, got %v", err)
		}
	})

	t.Run("Mark not in active pool", func(t *testing.T) {
		// MarkSquare (index 4) is not in pool of size 4 (indices 0..3)
		_, err := casino.PlayDoppelGame(10, 4, casino.MarkSquare)
		if err != casino.ErrInvalidDoppelMark {
			t.Errorf("expected ErrInvalidDoppelMark, got %v", err)
		}
	})

	t.Run("Invalid bet amounts", func(t *testing.T) {
		_, err := casino.PlayDoppelGame(0, 4, casino.MarkStar)
		if err != casino.ErrInvalidDoppelBet {
			t.Errorf("expected ErrInvalidDoppelBet, got %v", err)
		}
		_, err = casino.PlayDoppelGame(6000, 4, casino.MarkStar)
		if err != casino.ErrInvalidDoppelBet {
			t.Errorf("expected ErrInvalidDoppelBet, got %v", err)
		}
	})

	t.Run("Valid game execution", func(t *testing.T) {
		res, err := casino.PlayDoppelGame(25, 4, casino.MarkStar)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.BetCoins != 25 {
			t.Errorf("bet = %d, want 25", res.BetCoins)
		}
		if res.PoolSize != 4 {
			t.Errorf("pool size = %d, want 4", res.PoolSize)
		}
	})
}
