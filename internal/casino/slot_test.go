package casino_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestEvaluateSpin(t *testing.T) {
	tests := []struct {
		name           string
		bet            int64
		reels          [3]casino.SlotSymbol
		wantWin        bool
		wantMultiplier int
		wantPayout     int64
		wantNet        int64
	}{
		{
			name:           "Jackpot Seven 777",
			bet:            10,
			reels:          [3]casino.SlotSymbol{casino.SymbolSeven, casino.SymbolSeven, casino.SymbolSeven},
			wantWin:        true,
			wantMultiplier: 100,
			wantPayout:     1000,
			wantNet:        990,
		},
		{
			name:           "Super Win Stars",
			bet:            50,
			reels:          [3]casino.SlotSymbol{casino.SymbolStar, casino.SymbolStar, casino.SymbolStar},
			wantWin:        true,
			wantMultiplier: 70,
			wantPayout:     3500,
			wantNet:        3450,
		},
		{
			name:           "Big Win Daggers",
			bet:            100,
			reels:          [3]casino.SlotSymbol{casino.SymbolDagger, casino.SymbolDagger, casino.SymbolDagger},
			wantWin:        true,
			wantMultiplier: 50,
			wantPayout:     5000,
			wantNet:        4900,
		},
		{
			name:           "Win Notes",
			bet:            1,
			reels:          [3]casino.SlotSymbol{casino.SymbolNote, casino.SymbolNote, casino.SymbolNote},
			wantWin:        true,
			wantMultiplier: 20,
			wantPayout:     20,
			wantNet:        19,
		},
		{
			name:           "Win Cherries 3x",
			bet:            200,
			reels:          [3]casino.SlotSymbol{casino.SymbolCherry, casino.SymbolCherry, casino.SymbolCherry},
			wantWin:        true,
			wantMultiplier: 10,
			wantPayout:     2000,
			wantNet:        1800,
		},
		{
			name:           "Cherries 2x (Reel 1 & 2)",
			bet:            10,
			reels:          [3]casino.SlotSymbol{casino.SymbolCherry, casino.SymbolCherry, casino.SymbolStar},
			wantWin:        true,
			wantMultiplier: 3,
			wantPayout:     30,
			wantNet:        20,
		},
		{
			name:           "Miss",
			bet:            10,
			reels:          [3]casino.SlotSymbol{casino.SymbolCherry, casino.SymbolStar, casino.SymbolCherry},
			wantWin:        false,
			wantMultiplier: 0,
			wantPayout:     0,
			wantNet:        -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := casino.EvaluateSpin(tt.bet, tt.reels)
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

func TestSpinSlotMachine_Validation(t *testing.T) {
	_, err := casino.SpinSlotMachine(99) // Invalid bet rate
	if err != casino.ErrInvalidBetRate {
		t.Errorf("expected ErrInvalidBetRate, got %v", err)
	}

	for bet := range casino.ValidBetRates {
		res, err := casino.SpinSlotMachine(bet)
		if err != nil {
			t.Fatalf("valid bet %d returned error: %v", bet, err)
		}
		if res.BetCoins != bet {
			t.Errorf("BetCoins = %d, want %d", res.BetCoins, bet)
		}
	}
}
