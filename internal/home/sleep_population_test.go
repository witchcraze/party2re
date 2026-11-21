package home

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
)

func newPopulationSleepService(t *testing.T, counter OnlineCounter, opts ...ServiceOption) (*Service, *mockCharRepo, timer.Service) {
	t.Helper()
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{"c1": {ID: "c1", Name: "Hero"}}}
	tm := timer.NewService(nil)
	base := []ServiceOption{WithTimer(tm), WithCharacterUpdater(chars)}
	if counter != nil {
		base = append(base, WithOnlineCounter(counter))
	}
	svc, err := NewService(newMockHomeRepo(), chars, append(base, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return svc, chars, tm
}

func TestSleepDurationFromObservedPopulation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		base  time.Duration
		count int
		want  time.Duration
	}{
		{"default 0", 0, 0, 300 * time.Second},
		{"default 19", 0, 19, 300 * time.Second},
		{"default 20", 0, 20, 600 * time.Second},
		{"default 29", 0, 29, 600 * time.Second},
		{"default 30", 0, 30, 900 * time.Second},
		{"override 0", 120 * time.Second, 0, 120 * time.Second},
		{"override 20", 120 * time.Second, 20, 240 * time.Second},
		{"override 30", 120 * time.Second, 30, 360 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var opts []ServiceOption
			if tc.base > 0 {
				opts = append(opts, WithBaseSleepDuration(tc.base))
			}
			counter := &mockOnlineCounter{count: tc.count}
			svc, _, tm := newPopulationSleepService(t, counter, opts...)
			res, err := svc.Sleep(ctx, "c1", "")
			if err != nil {
				t.Fatal(err)
			}
			if res.DurationSeconds != int(tc.want/time.Second) || counter.calls != 1 {
				t.Fatalf("duration = %ds after %d counts, want %v once", res.DurationSeconds, counter.calls, tc.want)
			}
			remaining, err := tm.GetRemainingLock(ctx, timer.CategorySleep, "c1")
			if err != nil || remaining > tc.want || remaining < tc.want-2*time.Second {
				t.Fatalf("sleep timer = %v, %v; want %v", remaining, err, tc.want)
			}

			// A later population change neither re-counts on status reads nor changes the running timer.
			counter.count = 30
			status, err := svc.GetSleepStatus(ctx, "c1")
			if err != nil || !status.Sleeping || counter.calls != 1 || status.RemainingSeconds > int(tc.want/time.Second) {
				t.Fatalf("status = %+v, %v after %d counts", status, err, counter.calls)
			}
		})
	}
}

type orderCheckingCounter struct {
	t     *testing.T
	chars *mockCharRepo
	tm    timer.Service
	count int
}

func (c *orderCheckingCounter) GetOnlineCount(ctx context.Context) (int, error) {
	if c.chars.chars["c1"].PendingWake {
		c.t.Error("population counted after the Sleep save")
	}
	if locked, err := c.tm.IsLocked(ctx, timer.CategorySleep, "c1"); err != nil || locked {
		c.t.Errorf("population counted after sleep timer: %v, %v", locked, err)
	}
	return c.count, nil
}

func TestSleepCountsPopulationBeforeAnySleepEffect(t *testing.T) {
	counter := &orderCheckingCounter{t: t, count: 19}
	svc, chars, tm := newPopulationSleepService(t, counter)
	counter.chars, counter.tm = chars, tm
	res, err := svc.Sleep(context.Background(), "c1", "")
	if err != nil || res.DurationSeconds != 300 || !chars.chars["c1"].PendingWake {
		t.Fatalf("sleep = %+v, %v", res, err)
	}
}

func TestSleepPopulationFailureStopsBeforeEffects(t *testing.T) {
	injected := errors.New("count failed")
	cases := []struct {
		name    string
		counter OnlineCounter
		want    error
	}{
		{"missing counter", nil, nil},
		{"count error", &mockOnlineCounter{err: injected}, injected},
		{"canceled", &mockOnlineCounter{err: context.Canceled}, context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, chars, tm := newPopulationSleepService(t, tc.counter)
			_, err := svc.Sleep(ctx, "c1", "")
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("Sleep error = %v, want %v", err, tc.want)
			}
			if chars.chars["c1"].PendingWake {
				t.Fatal("pending wake written after count failure")
			}
			for _, category := range []string{timer.CategorySleep, timer.CategoryAsleep, timer.CategoryWaking} {
				if locked, err := tm.IsLocked(ctx, category, "c1"); err != nil || locked {
					t.Fatalf("%s lock = %v, %v after count failure", category, locked, err)
				}
			}

			// The released claim lets a healthy explicit retry start Sleep.
			svc.onlineCounter = &mockOnlineCounter{}
			if res, err := svc.Sleep(ctx, "c1", ""); err != nil || res.DurationSeconds != 300 {
				t.Fatalf("retry = %+v, %v", res, err)
			}
		})
	}
}
