package eventplaza

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func realPresenceTracker(t *testing.T) (*ValkeyPresenceTracker, valkey.Client) {
	t.Helper()
	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		t.Fatal(err)
	}
	tracker := NewValkeyPresenceTracker(client)
	tracker.key += ":test:" + id.New()
	t.Cleanup(func() {
		if err := client.Do(context.Background(), client.B().Del().Key(tracker.key).Build()).Error(); err != nil {
			t.Error(err)
		}
		client.Close()
	})
	return tracker, client
}

func TestRealValkeyPresenceRetention(t *testing.T) {
	tracker, client := realPresenceTracker(t)
	ctx := context.Background()
	clock := &mockClock{now: time.Now().UTC().Truncate(time.Second)}
	start := clock.now
	characters := newMemoryCharacterRepo()
	char, err := corecharacter.New("Hero")
	if err != nil {
		t.Fatal(err)
	}
	characters.characters[char.ID] = char
	svc, err := NewService(newMemoryRepo(), characters, newMemoryInventoryRepo(), WithClock(clock), WithPresenceTracker(tracker))
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.AppendCrowd(ctx, []Presence{
		{"Late", start.Add(-6 * time.Minute)},
		{"Early", start.Add(time.Hour)}, {"Late", start.Add(7199 * time.Second)},
		{"Late", start.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	// Simulate the previous implementation's sliding TTL, then refresh a real visitor.
	if err := client.Do(ctx, client.B().Expire().Key(tracker.key).Seconds(3600).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordPresence(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	ttl, err := client.Do(ctx, client.B().Ttl().Key(tracker.key).Build()).AsInt64()
	if err != nil || ttl != -1 {
		t.Fatalf("heartbeat retained an early key expiry: TTL=%d err=%v", ttl, err)
	}
	for _, step := range []struct {
		elapsed time.Duration
		active  int
	}{
		{time.Hour + PresenceWindow, 2}, {time.Hour + PresenceWindow + time.Second, 1},
		{7199*time.Second + PresenceWindow, 1}, {7200*time.Second + PresenceWindow, 0},
	} {
		clock.now = start.Add(step.elapsed)
		status, err := svc.GetPlazaStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if status.ActiveParticipants != step.active {
			t.Fatalf("at %v: %+v", step.elapsed, status)
		}
	}
	exists, err := client.Do(ctx, client.B().Exists().Key(tracker.key).Build()).AsInt64()
	if err != nil || exists != 0 {
		t.Fatalf("empty key not removed: exists=%d err=%v", exists, err)
	}
}

func TestRealValkeyConcurrentCrowdAndObservation(t *testing.T) {
	tracker, _ := realPresenceTracker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Now().UTC()
	cfg := testutil.GetStressConfig()
	result := testutil.RunConcurrentStressTest(t, cfg, func(worker, op int) error {
		name := fmt.Sprintf("guest-%d-%d", worker, op)
		err := tracker.AppendCrowd(ctx, []Presence{{name, now.Add(time.Hour)}})
		if err != nil {
			t.Logf("worker=%d op=%d: %v", worker, op, err)
		}
		return err
	})
	if result.Failures != 0 {
		t.Fatalf("concurrent append failures: %+v", result)
	}
	for _, err := range testutil.RunRace(
		func() error { _, err := tracker.Observe(ctx, now); return err },
		func() error { return tracker.RecordPresence(ctx, "hero", now) },
	) {
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := tracker.Observe(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	want := cfg.Workers*cfg.OpsPerWorker + 2 // one real visitor and the merchant
	if snapshot.Active != want {
		t.Fatalf("lost/duplicated concurrent records: got %d, want %d", snapshot.Active, want)
	}
}
