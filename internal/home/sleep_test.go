package home

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
)

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, ErrCharacterNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	m.chars[char.ID] = char
	return nil
}

type mockFullnessResetter struct {
	calledFor string
}

func (m *mockFullnessResetter) ResetFullness(ctx context.Context, characterID string) error {
	m.calledFor = characterID
	return nil
}

type mockBlessingCleaner struct {
	calledFor string
}

func (m *mockBlessingCleaner) ClearBlessing(ctx context.Context, characterID string) error {
	m.calledFor = characterID
	return nil
}

type mockOnlineCounter struct {
	count int
}

func (m *mockOnlineCounter) GetOnlineCount(ctx context.Context) (int, error) {
	return m.count, nil
}

func TestCalculateSleepDuration(t *testing.T) {
	base := 60 * time.Second

	tests := []struct {
		onlineCount int
		expected    time.Duration
	}{
		{0, 60 * time.Second},
		{10, 60 * time.Second},
		{19, 60 * time.Second},
		{20, 120 * time.Second},
		{29, 120 * time.Second},
		{30, 180 * time.Second},
		{50, 180 * time.Second},
	}

	for _, tt := range tests {
		got := CalculateSleepDuration(base, tt.onlineCount)
		if got != tt.expected {
			t.Errorf("CalculateSleepDuration(base, %d) = %v; want %v", tt.onlineCount, got, tt.expected)
		}
	}
}

func TestSleepAndWake_Lifecycle(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {
				ID:    "c1",
				Name:  "Hero",
				Tired: 80,
				Stats: corecharacter.Stats{
					MaxHP: 100, HP: 10,
					MaxMP: 50, MP: 5,
				},
			},
			"c2": {
				ID:   "c2",
				Name: "Friend",
			},
		},
	}

	timerSvc := timer.NewService(nil)
	fullness := &mockFullnessResetter{}
	blessing := &mockBlessingCleaner{}
	online := &mockOnlineCounter{count: 25} // 2x multiplier

	mockHomeRepo := newMockHomeRepo()

	svc, err := NewService(
		mockHomeRepo,
		charRepo,
		WithTimer(timerSvc),
		WithCharacterUpdater(charRepo),
		WithFullnessResetter(fullness),
		WithBlessingCleaner(blessing),
		WithOnlineCounter(online),
		WithBaseSleepDuration(50*time.Millisecond), // short duration for test
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 1. Sleep in friend's house (c2)
	res, err := svc.Sleep(ctx, "c1", "c2")
	if err != nil {
		t.Fatalf("failed to sleep: %v", err)
	}
	if !res.Sleeping || res.HomeCharacterID != "c2" {
		t.Fatalf("unexpected sleep result: %+v", res)
	}

	// 2. Already sleeping error
	_, err = svc.Sleep(ctx, "c1", "c1")
	if !errors.Is(err, ErrAlreadySleeping) {
		t.Fatalf("expected ErrAlreadySleeping, got %v", err)
	}

	// 3. Status while sleeping
	status, err := svc.GetSleepStatus(ctx, "c1")
	if err != nil || !status.Sleeping || status.CanWake {
		t.Fatalf("expected sleeping status, got %+v, err=%v", status, err)
	}

	// 4. Try waking early -> ErrStillSleeping
	_, err = svc.Wake(ctx, "c1")
	if !errors.Is(err, ErrStillSleeping) {
		t.Fatalf("expected ErrStillSleeping, got %v", err)
	}

	// 5. Wait for sleep timer to expire (50ms * 2 = 100ms)
	time.Sleep(120 * time.Millisecond)

	// 6. Status after timer expired -> CanWake = true
	status, err = svc.GetSleepStatus(ctx, "c1")
	if err != nil || status.Sleeping || !status.CanWake {
		t.Fatalf("expected can wake status, got %+v, err=%v", status, err)
	}

	// 7. Wake up successfully
	wakeRes, err := svc.Wake(ctx, "c1")
	if err != nil {
		t.Fatalf("failed to wake: %v", err)
	}
	if !wakeRes.Success {
		t.Fatalf("expected wake success, got %+v", wakeRes)
	}
	if wakeRes.Character.Stats.HP != 100 || wakeRes.Character.Stats.MP != 50 {
		t.Fatalf("expected full HP/MP recovery, got HP=%d, MP=%d", wakeRes.Character.Stats.HP, wakeRes.Character.Stats.MP)
	}
	if wakeRes.Character.Tired != 0 {
		t.Fatalf("expected tired to be 0, got %d", wakeRes.Character.Tired)
	}
	if fullness.calledFor != "c1" {
		t.Fatalf("expected fullness reset for c1, got %q", fullness.calledFor)
	}
	if blessing.calledFor != "c1" {
		t.Fatalf("expected blessing clear for c1, got %q", blessing.calledFor)
	}

	// 8. Subsequent Wake is idempotent
	wakeAgain, err := svc.Wake(ctx, "c1")
	if err != nil || !wakeAgain.Success {
		t.Fatalf("expected idempotent wake, got %+v, err=%v", wakeAgain, err)
	}
}

func TestSleep_JobMemoryReversal(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {
				ID:       "c1",
				Name:     "Hero",
				JobID:    "warrior",
				SP:       10,
				OldJobID: "novice",
				OldSP:    5,
				JobMemory: &corecharacter.JobMemory{
					JobID:    "mage",
					SP:       20,
					OldJobID: "apprentice",
					OldSP:    15,
				},
			},
		},
	}

	timerSvc := timer.NewService(nil)
	mockHomeRepo := newMockHomeRepo()

	svc, err := NewService(
		mockHomeRepo,
		charRepo,
		WithTimer(timerSvc),
		WithCharacterUpdater(charRepo),
		WithBaseSleepDuration(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Going to sleep restores original job memory
	_, err = svc.Sleep(ctx, "c1", "c1")
	if err != nil {
		t.Fatalf("failed to sleep: %v", err)
	}

	updated := charRepo.chars["c1"]
	if updated.JobMemory != nil {
		t.Fatalf("expected JobMemory to be cleared, got %+v", updated.JobMemory)
	}
	if updated.JobID != "mage" || updated.SP != 20 {
		t.Fatalf("expected Job to be restored to mage (SP 20), got JobID=%s, SP=%d", updated.JobID, updated.SP)
	}
}
