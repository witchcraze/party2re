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
	if got.ID == "" || got.Name != "Alice" || got.JobID != DefaultJobID ||
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
