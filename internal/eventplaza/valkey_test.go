package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

type mockSortedSetStore struct {
	mu      sync.Mutex
	members map[string]float64
	fail    string
	aborts  int
}

func newMockSortedSetStore() *mockSortedSetStore {
	return &mockSortedSetStore{members: make(map[string]float64)}
}

func (s *mockSortedSetStore) Client() valkey.Client {
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		args := cmd.Commands()
		if s.fail == args[0] {
			return valkeytest.MakeErrorResult(errors.New("store unavailable"))
		}
		if args[0] == "ZRANGE" {
			keys := make([]string, 0, len(s.members))
			for key := range s.members {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			values := make([]string, 0, 2*len(keys))
			for _, key := range keys {
				values = append(values, key, strconv.FormatFloat(s.members[key], 'f', -1, 64))
			}
			return valkeytest.MakeStringSliceResult(values)
		}
		return valkeytest.MakeOKResult()
	}))
	client.DedicatedFn = func(fn func(valkey.DedicatedClient) error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		dedicated := valkeytest.NewMockClient(valkeytest.WithDoHandler(client.DoFn), valkeytest.WithDoMultiHandler(client.DoMultiFn))
		return dedicated.Dedicated(fn)
	}
	client.DoMultiFn = func(ctx context.Context, commands ...valkey.Completed) []valkey.ValkeyResult {
		results := make([]valkey.ValkeyResult, len(commands))
		for i := range results {
			results[i] = valkeytest.MakeOKResult()
		}
		if s.aborts > 0 {
			s.aborts--
			results[len(results)-1] = valkeytest.MakeNilResult()
			return results
		}
		for _, cmd := range commands {
			args := cmd.Commands()
			if s.fail == args[0] {
				results[len(results)-1] = valkeytest.MakeErrorResult(errors.New("store unavailable"))
				return results
			}
		}
		for _, cmd := range commands {
			args := cmd.Commands()
			switch args[0] {
			case "DEL":
				clear(s.members)
			case "ZADD":
				for i := 2; i < len(args); i += 2 {
					score, err := strconv.ParseFloat(args[i], 64)
					if err != nil {
						panic(err)
					}
					s.members[args[i+1]] = score
				}
			}
		}
		results[len(results)-1] = valkeytest.MakeIntSliceResult([]int64{1})
		return results
	}
	return client
}

func newTrackedService(t *testing.T, store *mockSortedSetStore, clock *mockClock) *Service {
	t.Helper()
	svc, err := NewService(newMemoryRepo(), newMemoryCharacterRepo(), newMemoryInventoryRepo(),
		WithClock(clock), WithPresenceTracker(NewValkeyPresenceTracker(store.Client())))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestMerchantSnapshotBeforeCleanup(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	for _, count := range []int{9, 10, 19, 20, 29, 30} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			store := newMockSortedSetStore()
			tracker := NewValkeyPresenceTracker(store.Client())
			for i := 0; i < count; i++ {
				if err := tracker.RecordPresence(ctx, fmt.Sprint(i), now.Add(-PresenceWindow-time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			clock := &mockClock{now: now}
			svc := newTrackedService(t, store, clock)
			status, err := svc.GetPlazaStatus(ctx)
			if err != nil {
				t.Fatal(err)
			}
			tier, _, _ := CalculateMerchantTier(count)
			active := 0
			if tier > 0 {
				active = 1
			}
			if status.MerchantSelectionCount != count || status.MerchantTier != tier || status.ActiveParticipants != active {
				t.Fatalf("first status = %+v, want count/tier/active %d/%d/%d", status, count, tier, active)
			}
			next, err := svc.GetPlazaStatus(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if next.MerchantTier != 0 || next.MerchantSelectionCount != active {
				t.Fatalf("after cleanup = %+v", next)
			}
		})
	}
}

func TestPresenceGraceDuplicatesAndStaggeredDeparture(t *testing.T) {
	ctx := context.Background()
	clock := &mockClock{now: time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)}
	start := clock.now
	store := newMockSortedSetStore()
	tracker := NewValkeyPresenceTracker(store.Client())
	// The later duplicate must not extend the first retained guest's lifetime.
	if err := tracker.AppendCrowd(ctx, []Presence{
		{"Guest", start.Add(time.Hour)}, {"Guest", start.Add(7199 * time.Second)},
		{"Decoration", start.Add(7199 * time.Second)},
	}); err != nil {
		t.Fatal(err)
	}
	svc := newTrackedService(t, store, clock)
	for _, step := range []struct {
		elapsed time.Duration
		active  int
	}{
		{time.Hour + PresenceWindow, 2},
		{time.Hour + PresenceWindow + time.Second, 1},
		{7199*time.Second + PresenceWindow, 1},
		{7200*time.Second + PresenceWindow, 0},
	} {
		clock.now = start.Add(step.elapsed)
		status, err := svc.GetPlazaStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if status.ActiveParticipants != step.active {
			t.Fatalf("at %v: active=%d, want %d", step.elapsed, status.ActiveParticipants, step.active)
		}
	}
	if len(store.members) != 0 {
		t.Fatal("empty plaza key still contains records")
	}
}

func TestMerchantRowContributesToNextSelection(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store := newMockSortedSetStore()
	tracker := NewValkeyPresenceTracker(store.Client())
	// Ten stored rows collapse to nine guests plus the newly added merchant.
	crowd := make([]Presence, 0, 10)
	for i := 0; i < 9; i++ {
		crowd = append(crowd, Presence{fmt.Sprint("guest", i), now.Add(time.Hour)})
	}
	crowd = append(crowd, crowd[0])
	if err := tracker.AppendCrowd(ctx, crowd); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		snapshot, err := tracker.Observe(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot != (PresenceSnapshot{Stored: 10, Active: 10}) {
			t.Fatalf("observation %d: %+v", i, snapshot)
		}
	}
}

func TestValkeyPresenceTrackerErrorsAndRetries(t *testing.T) {
	for _, command := range []string{"WATCH", "ZRANGE", "ZADD", "EXEC"} {
		t.Run(command, func(t *testing.T) {
			store := newMockSortedSetStore()
			store.fail = command
			tracker := NewValkeyPresenceTracker(store.Client())
			if err := tracker.RecordPresence(context.Background(), "hero", time.Now()); err == nil {
				t.Fatal("store error suppressed")
			}
		})
	}
	store := newMockSortedSetStore()
	store.aborts = 16
	tracker := NewValkeyPresenceTracker(store.Client())
	if err := tracker.RecordPresence(context.Background(), "hero", time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(store.members) != 1 {
		t.Fatal("retry duplicated/lost presence")
	}
	tracker.retryLimit = 4
	store.aborts = 4
	if _, err := tracker.Observe(context.Background(), time.Now()); err == nil {
		t.Fatal("contention limit ignored")
	}
}

func TestValkeyPresenceTrackerMigratesExistingMembers(t *testing.T) {
	now := time.Now().UTC()
	store := newMockSortedSetStore()
	store.members["hero"] = float64(now.Unix())
	store.members["banquet:old:npc:0"] = float64(now.Add(time.Hour).Unix())
	tracker := NewValkeyPresenceTracker(store.Client())
	if err := tracker.RecordPresence(context.Background(), "hero", now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := tracker.Observe(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot != (PresenceSnapshot{Stored: 2, Active: 2}) {
		t.Fatalf("migration = %+v", snapshot)
	}
}

func TestValkeyPresenceTrackerClosesSocketWhenUnwatchFails(t *testing.T) {
	closed := false
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		switch cmd.Commands()[0] {
		case "ZRANGE":
			return valkeytest.MakeStringSliceResult(nil)
		case "EXEC":
			return valkeytest.MakeIntSliceResult([]int64{1})
		case "UNWATCH":
			return valkeytest.MakeErrorResult(context.Canceled)
		default:
			return valkeytest.MakeOKResult()
		}
	}))
	client.CloseFn = func() { closed = true }
	tracker := NewValkeyPresenceTracker(client)
	if err := tracker.RecordPresence(context.Background(), "Hero", time.Now()); err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("socket with uncleared WATCH was returned to the pool")
	}
}
