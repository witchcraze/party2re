package character

import (
	"testing"
)

func TestStatsClampVitality(t *testing.T) {
	tests := []struct {
		name      string
		initial   Stats
		wantStats Stats
	}{
		{
			name:      "within bounds",
			initial:   Stats{MaxHP: 100, HP: 80, MaxMP: 50, MP: 20},
			wantStats: Stats{MaxHP: 100, HP: 80, MaxMP: 50, MP: 20},
		},
		{
			name:      "negative HP and MP clamped to zero",
			initial:   Stats{MaxHP: 100, HP: -15, MaxMP: 50, MP: -5},
			wantStats: Stats{MaxHP: 100, HP: 0, MaxMP: 50, MP: 0},
		},
		{
			name:      "HP and MP exceeding MaxHP and MaxMP clamped",
			initial:   Stats{MaxHP: 100, HP: 150, MaxMP: 50, MP: 80},
			wantStats: Stats{MaxHP: 100, HP: 100, MaxMP: 50, MP: 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.initial
			s.ClampVitality()
			if s.HP != tt.wantStats.HP || s.MP != tt.wantStats.MP {
				t.Errorf("ClampVitality() got HP=%d, MP=%d; want HP=%d, MP=%d", s.HP, s.MP, tt.wantStats.HP, tt.wantStats.MP)
			}
		})
	}

	// Nil receiver safety
	var nilStats *Stats
	nilStats.ClampVitality()
}

func TestCharacterRecoverVitality(t *testing.T) {
	c := &Character{
		Stats: Stats{MaxHP: 200, HP: 25, MaxMP: 80, MP: 5},
	}
	c.RecoverVitality()

	if c.Stats.HP != 200 || c.Stats.MP != 80 {
		t.Errorf("RecoverVitality() got HP=%d, MP=%d; want HP=200, MP=80", c.Stats.HP, c.Stats.MP)
	}

	// Nil receiver safety
	var nilChar *Character
	nilChar.RecoverVitality()
}

func TestCharacterApplyCombatSurvival(t *testing.T) {
	tests := []struct {
		name        string
		initial     Stats
		survivingHP int
		survivingMP int
		fallen      bool
		wantHP      int
		wantMP      int
	}{
		{
			name:        "normal survival",
			initial:     Stats{MaxHP: 100, HP: 100, MaxMP: 50, MP: 50},
			survivingHP: 45,
			survivingMP: 20,
			fallen:      false,
			wantHP:      45,
			wantMP:      20,
		},
		{
			name:        "fallen by flag gets authentic 1 HP floor",
			initial:     Stats{MaxHP: 100, HP: 100, MaxMP: 50, MP: 50},
			survivingHP: 0,
			survivingMP: 10,
			fallen:      true,
			wantHP:      1,
			wantMP:      10,
		},
		{
			name:        "fallen by zero HP gets authentic 1 HP floor even if fallen flag false",
			initial:     Stats{MaxHP: 100, HP: 100, MaxMP: 50, MP: 50},
			survivingHP: 0,
			survivingMP: 5,
			fallen:      false,
			wantHP:      1,
			wantMP:      5,
		},
		{
			name:        "fallen by negative HP gets authentic 1 HP floor",
			initial:     Stats{MaxHP: 100, HP: 100, MaxMP: 50, MP: 50},
			survivingHP: -25,
			survivingMP: 0,
			fallen:      false,
			wantHP:      1,
			wantMP:      0,
		},
		{
			name:        "surviving HP exceeds MaxHP clamped",
			initial:     Stats{MaxHP: 100, HP: 50, MaxMP: 50, MP: 50},
			survivingHP: 150,
			survivingMP: 30,
			fallen:      false,
			wantHP:      100,
			wantMP:      30,
		},
		{
			name:        "surviving MP exceeds MaxMP clamped",
			initial:     Stats{MaxHP: 100, HP: 50, MaxMP: 50, MP: 10},
			survivingHP: 60,
			survivingMP: 90,
			fallen:      false,
			wantHP:      60,
			wantMP:      50,
		},
		{
			name:        "negative surviving MP preserves and clamps current MP",
			initial:     Stats{MaxHP: 100, HP: 50, MaxMP: 50, MP: 35},
			survivingHP: 75,
			survivingMP: -1,
			fallen:      false,
			wantHP:      75,
			wantMP:      35,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{Stats: tt.initial}
			c.ApplyCombatSurvival(tt.survivingHP, tt.survivingMP, tt.fallen)
			if c.Stats.HP != tt.wantHP || c.Stats.MP != tt.wantMP {
				t.Errorf("ApplyCombatSurvival() got HP=%d, MP=%d; want HP=%d, MP=%d",
					c.Stats.HP, c.Stats.MP, tt.wantHP, tt.wantMP)
			}
		})
	}

	// Nil receiver safety
	var nilChar *Character
	nilChar.ApplyCombatSurvival(50, 10, false)
}

func TestCharacterFatigueClamping(t *testing.T) {
	c := &Character{Tired: 0}

	if c.IsExhausted() {
		t.Errorf("expected IsExhausted to be false at 0%% tired")
	}

	// Increments and upper bound clamping
	c.AddTired(40)
	if c.Tired != 40 || c.IsExhausted() {
		t.Errorf("expected 40%% tired, got %d (exhausted=%v)", c.Tired, c.IsExhausted())
	}

	c.AddTired(60)
	if c.Tired != 100 || !c.IsExhausted() {
		t.Errorf("expected 100%% tired and exhausted, got %d (exhausted=%v)", c.Tired, c.IsExhausted())
	}

	// Cannot exceed 100
	c.AddTired(30)
	if c.Tired != 100 {
		t.Errorf("expected Tired to be clamped at 100, got %d", c.Tired)
	}

	// Negative/zero delta ignored
	c.AddTired(0)
	c.AddTired(-10)
	if c.Tired != 100 {
		t.Errorf("expected Tired to remain 100, got %d", c.Tired)
	}

	// Celestial wish drops below 0% as authentic legacy buffer
	c.ReduceTired(150)
	if c.Tired != -50 || c.IsExhausted() {
		t.Errorf("expected -50%% tired after ReduceTired(150), got %d (exhausted=%v)", c.Tired, c.IsExhausted())
	}

	// Adding fatigue moves upward from negative buffer
	c.AddTired(20)
	if c.Tired != -30 || c.IsExhausted() {
		t.Errorf("expected -30%% tired after AddTired(20), got %d (exhausted=%v)", c.Tired, c.IsExhausted())
	}

	// Negative/zero delta ignored for ReduceTired
	c.ReduceTired(0)
	c.ReduceTired(-20)
	if c.Tired != -30 {
		t.Errorf("expected Tired to remain -30, got %d", c.Tired)
	}

	// Sleep resets to 0
	c.ResetTired()
	if c.Tired != 0 || c.IsExhausted() {
		t.Errorf("expected 0%% tired after ResetTired, got %d (exhausted=%v)", c.Tired, c.IsExhausted())
	}

	// Nil receiver safety
	var nilChar *Character
	nilChar.AddTired(10)
	nilChar.ReduceTired(10)
	nilChar.ResetTired()
	if nilChar.IsExhausted() {
		t.Errorf("expected nilChar.IsExhausted to be false")
	}
}
