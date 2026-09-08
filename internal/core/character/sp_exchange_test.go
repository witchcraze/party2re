package character_test

import (
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestCharacter_ApplySPExchange_Success(t *testing.T) {
	tests := []struct {
		name         string
		stat         corecharacter.SPExchangeStat
		sp           int
		initialStat  int
		expectedDiff int
		getStat      func(c *corecharacter.Character) int
	}{
		{
			name:         "MHP exchange rate 1:2",
			stat:         corecharacter.SPExchangeMaxHP,
			sp:           5,
			expectedDiff: 10,
			getStat:      func(c *corecharacter.Character) int { return c.Stats.MaxHP },
		},
		{
			name:         "MMP exchange rate 1:2",
			stat:         corecharacter.SPExchangeMaxMP,
			sp:           3,
			expectedDiff: 6,
			getStat:      func(c *corecharacter.Character) int { return c.Stats.MaxMP },
		},
		{
			name:         "Attack exchange rate 1:1",
			stat:         corecharacter.SPExchangeAttack,
			sp:           4,
			expectedDiff: 4,
			getStat:      func(c *corecharacter.Character) int { return c.Stats.Attack },
		},
		{
			name:         "Defense exchange rate 1:1",
			stat:         corecharacter.SPExchangeDefense,
			sp:           2,
			expectedDiff: 2,
			getStat:      func(c *corecharacter.Character) int { return c.Stats.Defense },
		},
		{
			name:         "Agility exchange rate 1:1",
			stat:         corecharacter.SPExchangeAgility,
			sp:           7,
			expectedDiff: 7,
			getStat:      func(c *corecharacter.Character) int { return c.Stats.Agility },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char, err := corecharacter.New("Hero")
			if err != nil {
				t.Fatalf("failed to create character: %v", err)
			}
			char.SP = 20
			initialStat := tt.getStat(&char)

			increase, err := char.ApplySPExchange(tt.stat, tt.sp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if increase != tt.expectedDiff {
				t.Errorf("got increase %d, want %d", increase, tt.expectedDiff)
			}
			if char.SP != 20-tt.sp {
				t.Errorf("got remaining SP %d, want %d", char.SP, 20-tt.sp)
			}
			finalStat := tt.getStat(&char)
			if finalStat != initialStat+tt.expectedDiff {
				t.Errorf("got final stat %d, want %d", finalStat, initialStat+tt.expectedDiff)
			}
		})
	}
}

func TestCharacter_ApplySPExchange_Errors(t *testing.T) {
	char, err := corecharacter.New("Hero")
	if err != nil {
		t.Fatalf("failed to create character: %v", err)
	}
	char.SP = 10

	// 1. Zero SP
	if _, err := char.ApplySPExchange(corecharacter.SPExchangeMaxHP, 0); err != corecharacter.ErrInvalidSPAmount {
		t.Errorf("expected ErrInvalidSPAmount for 0 SP, got %v", err)
	}

	// 2. Negative SP
	if _, err := char.ApplySPExchange(corecharacter.SPExchangeMaxHP, -3); err != corecharacter.ErrInvalidSPAmount {
		t.Errorf("expected ErrInvalidSPAmount for negative SP, got %v", err)
	}

	// 3. SP exceeds owned SP
	if _, err := char.ApplySPExchange(corecharacter.SPExchangeMaxHP, 11); err != corecharacter.ErrInsufficientSP {
		t.Errorf("expected ErrInsufficientSP, got %v", err)
	}

	// 4. OverLevel character cannot exchange SP
	charOver := char
	charOver.OverLevel = true
	if _, err := charOver.ApplySPExchange(corecharacter.SPExchangeMaxHP, 1); err != corecharacter.ErrOverLevelRestricted {
		t.Errorf("expected ErrOverLevelRestricted, got %v", err)
	}

	// 5. JobMemory active cannot exchange SP
	charMemory := char
	charMemory.JobMemory = &corecharacter.JobMemory{JobID: "job-01", SP: 10}
	if _, err := charMemory.ApplySPExchange(corecharacter.SPExchangeMaxHP, 1); err != corecharacter.ErrJobMemoryActive {
		t.Errorf("expected ErrJobMemoryActive, got %v", err)
	}

	// 6. Invalid stat
	if _, err := char.ApplySPExchange(corecharacter.SPExchangeStat("invalid"), 1); err != corecharacter.ErrInvalidTargetStat {
		t.Errorf("expected ErrInvalidTargetStat, got %v", err)
	}
}

func TestParseSPExchangeStat(t *testing.T) {
	tests := []struct {
		input    string
		expected corecharacter.SPExchangeStat
		wantErr  bool
	}{
		{"mhp", corecharacter.SPExchangeMaxHP, false},
		{"hp", corecharacter.SPExchangeMaxHP, false},
		{"max_hp", corecharacter.SPExchangeMaxHP, false},
		{"たいりょく", corecharacter.SPExchangeMaxHP, false},
		{"mmp", corecharacter.SPExchangeMaxMP, false},
		{"mp", corecharacter.SPExchangeMaxMP, false},
		{"max_mp", corecharacter.SPExchangeMaxMP, false},
		{"まりょく", corecharacter.SPExchangeMaxMP, false},
		{"at", corecharacter.SPExchangeAttack, false},
		{"attack", corecharacter.SPExchangeAttack, false},
		{"こうげき", corecharacter.SPExchangeAttack, false},
		{"df", corecharacter.SPExchangeDefense, false},
		{"defense", corecharacter.SPExchangeDefense, false},
		{"ぼうぎょ", corecharacter.SPExchangeDefense, false},
		{"ag", corecharacter.SPExchangeAgility, false},
		{"agility", corecharacter.SPExchangeAgility, false},
		{"すばやさ", corecharacter.SPExchangeAgility, false},
		{"unknown", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := corecharacter.ParseSPExchangeStat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSPExchangeStat(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.expected {
				t.Errorf("ParseSPExchangeStat(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
