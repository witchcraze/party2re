package character

import (
	"errors"
	"testing"
)

func TestNewCreatesLevelOneCharacter(t *testing.T) {
	got, err := New("  Alice  ")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got.ID == "" || got.PlayerID != "" || got.Name != "Alice" || got.JobID != DefaultJobID ||
		got.Gender != DefaultGender || got.Level != InitialLevel || got.Experience != 0 ||
		got.Money != InitialMoney {
		t.Fatalf("New() = %#v", got)
	}
	if got.Stats.MaxHP < 30 || got.Stats.MaxHP > 32 || got.Stats.MaxMP < 6 || got.Stats.MaxMP > 8 ||
		got.Stats.HP != got.Stats.MaxHP || got.Stats.MP != got.Stats.MaxMP ||
		got.Stats.Attack < 6 || got.Stats.Attack > 8 ||
		got.Stats.Defense < 6 || got.Stats.Defense > 8 ||
		got.Stats.Agility < 6 || got.Stats.Agility > 8 {
		t.Fatalf("New() stats = %#v", got.Stats)
	}
}

type sequenceRandom struct {
	values []int
	index  int
}

func (r *sequenceRandom) Intn(_ int) (int, error) {
	value := r.values[r.index]
	r.index++
	return value, nil
}

func TestNewWithOptionsUsesInitialValuesAndSelectedIdentity(t *testing.T) {
	got, err := NewWithOptions("Alice", "starter-2", "female", &sequenceRandom{
		values: []int{2, 1, 0, 2, 1},
	})
	if err != nil {
		t.Fatalf("NewWithOptions() error = %v", err)
	}
	if got.JobID != "starter-2" || got.Gender != "female" || got.Money != InitialMoney {
		t.Fatalf("NewWithOptions() identity = %#v", got)
	}
	wantStats := Stats{MaxHP: 32, MaxMP: 7, HP: 32, MP: 7, Attack: 6, Defense: 8, Agility: 7}
	if got.Stats != wantStats {
		t.Fatalf("NewWithOptions() stats = %#v, want %#v", got.Stats, wantStats)
	}
}

func TestNewRejectsInvalidNames(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "whitespace", input: "   "},
		{name: "too long", input: "123456789012345678901234567890123"},
		{name: "control character", input: "Alice\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.input); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("New(%q) error = %v, want %v", test.input, err, ErrInvalidName)
			}
		})
	}
}

func TestCharacterMoneyEncapsulation(t *testing.T) {
	c := Character{Money: 100}

	// HasMoney
	if !c.HasMoney(50) || !c.HasMoney(100) || c.HasMoney(101) || c.HasMoney(-1) {
		t.Errorf("unexpected HasMoney results")
	}

	// AddMoney
	if err := c.AddMoney(50); err != nil || c.Money != 150 {
		t.Errorf("AddMoney(50) failed: money=%d, err=%v", c.Money, err)
	}
	if err := c.AddMoney(-10); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("AddMoney(-10) expected ErrInvalidAmount, got %v", err)
	}
	// Max cap
	c.Money = MaxMoney - 50
	if err := c.AddMoney(100); err != nil || c.Money != MaxMoney {
		t.Errorf("AddMoney overflow cap failed: money=%d, err=%v", c.Money, err)
	}

	// DeductMoney
	c.Money = 100
	if err := c.DeductMoney(40); err != nil || c.Money != 60 {
		t.Errorf("DeductMoney(40) failed: money=%d, err=%v", c.Money, err)
	}
	if err := c.DeductMoney(100); !errors.Is(err, ErrInsufficientFunds) || c.Money != 60 {
		t.Errorf("DeductMoney(100) expected ErrInsufficientFunds, got %v", err)
	}
	if err := c.DeductMoney(-5); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("DeductMoney(-5) expected ErrInvalidAmount, got %v", err)
	}
}

func TestCharacterSmallMedalsEncapsulation(t *testing.T) {
	c := Character{SmallMedals: 10}

	// HasSmallMedals
	if !c.HasSmallMedals(5) || !c.HasSmallMedals(10) || c.HasSmallMedals(11) || c.HasSmallMedals(-1) {
		t.Errorf("unexpected HasSmallMedals results")
	}

	// AddSmallMedals
	if err := c.AddSmallMedals(5); err != nil || c.SmallMedals != 15 {
		t.Errorf("AddSmallMedals(5) failed: medals=%d, err=%v", c.SmallMedals, err)
	}
	if err := c.AddSmallMedals(-1); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("AddSmallMedals(-1) expected ErrInvalidAmount, got %v", err)
	}
	// Max cap
	c.SmallMedals = MaxSmallMedals - 5
	if err := c.AddSmallMedals(10); err != nil || c.SmallMedals != MaxSmallMedals {
		t.Errorf("AddSmallMedals overflow cap failed: medals=%d, err=%v", c.SmallMedals, err)
	}

	// DeductSmallMedals
	c.SmallMedals = 10
	if err := c.DeductSmallMedals(4); err != nil || c.SmallMedals != 6 {
		t.Errorf("DeductSmallMedals(4) failed: medals=%d, err=%v", c.SmallMedals, err)
	}
	if err := c.DeductSmallMedals(20); !errors.Is(err, ErrInsufficientMedals) || c.SmallMedals != 6 {
		t.Errorf("DeductSmallMedals(20) expected ErrInsufficientMedals, got %v", err)
	}
	if err := c.DeductSmallMedals(-1); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("DeductSmallMedals(-1) expected ErrInvalidAmount, got %v", err)
	}
}
