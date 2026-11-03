package home

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
)

func TestWakeEnforcesDurablePenaltyWithoutTimer(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	deadline := now.Add(time.Hour)
	pending := true
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{"actor": {ID: "actor", Name: "Actor", PendingWake: true, Stats: corecharacter.Stats{HP: 1, MaxHP: 50, MaxMP: 30}, Tired: 50}}}
	svc, err := NewService(newMockHomeRepo(), chars, WithTimer(timer.NewService(nil)), WithCharacterUpdater(chars), WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	svc.SetSleepPenaltyReader(func(context.Context, string) (time.Time, bool, error) { return deadline, pending, nil })
	if _, err := svc.Wake(ctx, "actor"); !errors.Is(err, ErrWakeInProgress) {
		t.Fatal(err)
	}
	pending = false
	if _, err := svc.Wake(ctx, "actor"); !errors.Is(err, ErrStillSleeping) {
		t.Fatal(err)
	}
	status, err := svc.GetSleepStatus(ctx, "actor")
	if err != nil || !status.Sleeping || status.CanWake || status.RemainingSeconds != 3600 {
		t.Fatal(status, err)
	}
	now = deadline
	pending = true
	if _, err := svc.Wake(ctx, "actor"); !errors.Is(err, ErrWakeInProgress) {
		t.Fatal("pending projection allowed Wake", err)
	}
	pending = false
	status, err = svc.GetSleepStatus(ctx, "actor")
	if err != nil || !status.CanWake {
		t.Fatal(status, err)
	}
	result, err := svc.Wake(ctx, "actor")
	if err != nil || !result.Success || result.Character.PendingWake || result.Character.Stats.HP != 50 {
		t.Fatal(result, err)
	}
	readError := errors.New("penalty authority unavailable")
	svc.SetSleepPenaltyReader(func(context.Context, string) (time.Time, bool, error) { return time.Time{}, false, readError })
	if _, err := svc.Wake(ctx, "actor"); !errors.Is(err, readError) {
		t.Fatal(err)
	}
}
