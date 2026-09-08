package character

import (
	"errors"
	"testing"
	"time"
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

func TestCharacterOrbEncapsulation(t *testing.T) {
	c := Character{Orb: ""}

	if c.OrbCount() != 0 {
		t.Fatalf("expected 0 orbs, got %d", c.OrbCount())
	}

	if c.HasAllOrbs() {
		t.Fatal("expected HasAllOrbs to be false for empty orb string")
	}
	if c.IsRamiaAwakened() {
		t.Fatal("expected IsRamiaAwakened to be false")
	}

	// Add valid orb
	added, err := c.AddOrb(OrbSilver)
	if err != nil || !added || !c.HasOrb(OrbSilver) {
		t.Fatalf("failed to add OrbSilver: added=%v, err=%v", added, err)
	}
	if c.OrbCount() != 1 {
		t.Fatalf("expected 1 orb, got %d", c.OrbCount())
	}

	// Duplicate add returns added=false, err=nil
	added, err = c.AddOrb(OrbSilver)
	if err != nil || added {
		t.Fatalf("expected duplicate AddOrb to return false, nil; got added=%v, err=%v", added, err)
	}

	// Invalid orb rune
	_, err = c.AddOrb('x')
	if !errors.Is(err, ErrInvalidOrbRune) {
		t.Fatalf("expected ErrInvalidOrbRune, got %v", err)
	}

	// Add remaining 5 orbs
	for _, orb := range []rune{OrbRed, OrbBlue, OrbGreen, OrbYellow, OrbPurple} {
		added, err := c.AddOrb(orb)
		if err != nil || !added {
			t.Fatalf("failed to add orb %c: %v", orb, err)
		}
	}

	if c.OrbCount() != 6 {
		t.Fatalf("expected 6 orbs, got %d", c.OrbCount())
	}
	if !c.HasAllOrbs() {
		t.Fatal("expected HasAllOrbs to be true")
	}

	// Awaken Ramia
	c.SetRamiaAwakened()
	if !c.IsRamiaAwakened() {
		t.Fatal("expected IsRamiaAwakened to be true")
	}
	if c.Orb != "G" {
		t.Fatalf("expected Orb to be 'G', got %q", c.Orb)
	}

	// Clear orbs
	c.ClearOrbs()
	if c.Orb != "" || c.OrbCount() != 0 || c.IsRamiaAwakened() {
		t.Fatalf("expected cleared orbs, got %q", c.Orb)
	}
}

func TestApplyJobChangeHalvesStatsAndResetsProgression(t *testing.T) {
	value := Character{
		JobID: "job-old", Level: 42, Experience: 1234, SP: 7, JobLevel: 2, OverLevel: true,
		Stats: Stats{MaxHP: 101, MaxMP: 19, HP: 1, MP: 1, Attack: 25, Defense: 9, Agility: 20},
	}
	if err := value.ApplyJobChange("job-new", 3); err != nil {
		t.Fatal(err)
	}
	if value.JobID != "job-new" || value.OldJobID != "job-old" || value.OldSP != 7 || value.SP != 3 {
		t.Fatalf("job state = %#v", value)
	}
	if value.Stats.MaxHP != 50 || value.Stats.MaxMP != 10 || value.Stats.Attack != 12 ||
		value.Stats.Defense != 10 || value.Stats.Agility != 10 ||
		value.Stats.HP != value.Stats.MaxHP || value.Stats.MP != value.Stats.MaxMP {
		t.Fatalf("stats = %#v", value.Stats)
	}
	if value.Level != 1 || value.Experience != 0 || value.JobLevel != 3 || value.OverLevel {
		t.Fatalf("progression = %#v", value)
	}
}

func TestFutureMemorySnapshotAndRestoration(t *testing.T) {
	c := Character{
		ID:         "char-1",
		JobID:      "job-05",
		OldJobID:   "job-01",
		Level:      45,
		Experience: 8900,
		SP:         80,
		OldSP:      50,
		Gender:     "male",
		OverLevel:  true,
		OverFuture: 1, // capacity for 1 + 1 = 2 snapshots (index 0, 1)
		Stats: Stats{
			MaxHP:   500,
			MaxMP:   200,
			HP:      150,
			MP:      80,
			Attack:  120,
			Defense: 90,
			Agility: 75,
		},
	}

	// CanSave checks
	if !c.CanSaveFutureMemory(0) {
		t.Fatal("expected CanSaveFutureMemory(0) with OverFuture=1 to be true")
	}
	if !c.CanSaveFutureMemory(1) {
		t.Fatal("expected CanSaveFutureMemory(1) with OverFuture=1 to be true")
	}
	if c.CanSaveFutureMemory(2) {
		t.Fatal("expected CanSaveFutureMemory(2) with OverFuture=1 to be false")
	}

	// If JobMemory is set (during recall), cannot save
	c.JobMemory = &JobMemory{JobID: "job-08"}
	if c.CanSaveFutureMemory(0) {
		t.Fatal("expected CanSaveFutureMemory to be false when JobMemory is active")
	}
	c.JobMemory = nil

	now := time.Now()
	snapshot := c.CreateFutureMemory("mem-1", now)
	if snapshot.ID != "mem-1" || snapshot.CharacterID != "char-1" || snapshot.JobID != "job-05" ||
		snapshot.OldJobID != "job-01" || snapshot.Level != 45 || snapshot.MaxHP != 500 || snapshot.Attack != 120 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}

	// Change character state to simulate different job / level
	c.JobID = "job-02"
	c.OldJobID = "job-05"
	c.Level = 10
	c.Experience = 200
	c.Stats = Stats{MaxHP: 100, MaxMP: 50, HP: 50, MP: 20, Attack: 30, Defense: 30, Agility: 30}
	c.SP = 10
	c.OldSP = 80
	c.OverLevel = false

	// Apply FutureMemory
	if err := c.ApplyFutureMemory(snapshot, 95, 60); err != nil {
		t.Fatalf("ApplyFutureMemory failed: %v", err)
	}

	if c.JobID != "job-05" || c.OldJobID != "job-01" || c.Level != 45 || c.Experience != 8900 {
		t.Fatalf("unexpected job/level state after recall: %#v", c)
	}
	if c.Stats.MaxHP != 500 || c.Stats.HP != 500 || c.Stats.MaxMP != 200 || c.Stats.MP != 200 ||
		c.Stats.Attack != 120 || c.Stats.Defense != 90 || c.Stats.Agility != 75 {
		t.Fatalf("unexpected stats after recall: %#v", c.Stats)
	}
	if c.SP != 95 || c.OldSP != 60 || !c.OverLevel {
		t.Fatalf("unexpected SP or OverLevel after recall: SP=%d, OldSP=%d, OverLevel=%v", c.SP, c.OldSP, c.OverLevel)
	}
}
