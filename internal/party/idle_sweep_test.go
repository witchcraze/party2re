package party_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil"
)

type sleepRecorder struct {
	failActor string
	deadlines map[string][]time.Time
}

var errSleepWrite = errors.New("sleep write failed")

func (r *sleepRecorder) ExtendLockUntil(_ context.Context, category, actor string, until time.Time) error {
	if category != timer.CategorySleep {
		return fmt.Errorf("unexpected timer category %s", category)
	}
	if actor == r.failActor {
		return errSleepWrite
	}
	r.deadlines[actor] = append(r.deadlines[actor], until)
	return nil
}

func TestSweepIdleRetriesWithFixedDeadline(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)
	repo := party.NewMemoryRepository(party.WithLobbyClock(func() time.Time { return now }))
	start := now
	sleep := &sleepRecorder{failActor: "ally", deadlines: make(map[string][]time.Time)}
	service := newIdleService(t, repo, repo, sleep, func() time.Time { return now })
	p := sampleTestParty("idle", "leader", "Lobby")
	seedIdleLobby(t, repo, p, "ally")
	other := sampleTestParty("other-idle", "other-leader", "Other")
	seedIdleLobby(t, repo, other)
	active := sampleTestParty("active", "active-leader", "Active")
	seedIdleLobby(t, repo, active)

	now = start.Add(30 * time.Minute)
	if err := service.SweepIdle(ctx); err != nil || len(sleep.deadlines) != 0 {
		t.Fatalf("at idle boundary: locks=%v err=%v", sleep.deadlines, err)
	}
	if err := repo.UpdateParty(ctx, active); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if err := service.SweepIdle(ctx); !errors.Is(err, errSleepWrite) {
		t.Fatalf("sweep error = %v", err)
	}
	if _, err := repo.GetParty(ctx, other.ID); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("sweep skipped a later lobby after a timer failure: %v", err)
	}
	until := now.Add(time.Hour)
	for _, actor := range []string{"leader", "ally"} {
		if _, _, err := repo.GetActivePartyByCharacter(ctx, actor); err != nil {
			t.Fatalf("failed sweep lost %s's membership: %v", actor, err)
		}
	}
	if _, err := service.LeaveParty(ctx, p.ID, "leader"); !errors.Is(err, party.ErrPartyNotRecruiting) {
		t.Fatalf("leave during pending timeout = %v", err)
	}

	sleep.failActor = ""
	now = now.Add(5 * time.Second)
	for range 2 {
		if err := service.SweepIdle(ctx); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string][]time.Time{"leader": {until, until}, "ally": {until}, "other-leader": {until}}
	if !reflect.DeepEqual(sleep.deadlines, want) {
		t.Fatalf("deadlines = %v, want %v", sleep.deadlines, want)
	}
	if _, err := repo.GetParty(ctx, p.ID); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("timed out lobby lookup = %v", err)
	}
	if _, err := repo.GetParty(ctx, active.ID); err != nil {
		t.Fatalf("active lobby lookup = %v", err)
	}
}

func TestSweepIdleRacesWithRosterRenewal(t *testing.T) {
	ctx := t.Context()
	clockNow := time.Now().UTC().Truncate(time.Millisecond)
	repo := party.NewMemoryRepository(party.WithLobbyClock(func() time.Time { return clockNow }))
	sleep := &sleepRecorder{deadlines: make(map[string][]time.Time)}
	service := newIdleService(t, repo, repo, sleep, func() time.Time { return clockNow })
	cfg := testutil.GetStressConfig()
	for i := range cfg.Workers * cfg.OpsPerWorker {
		p := sampleTestParty(fmt.Sprintf("race-%d", i), fmt.Sprintf("leader-%d", i), "Lobby")
		seedIdleLobby(t, repo, p)
		clockNow = clockNow.Add(30*time.Minute + time.Second)
		member := party.Member{PartyID: p.ID, CharacterID: fmt.Sprintf("ally-%d", i), JoinedAt: clockNow}
		renewErr, sweepErr := testutil.RunRace2(func() error {
			return repo.WithPartyAdventureLock(ctx, p.ID, func(ctx context.Context) error {
				return repo.AddMember(ctx, member)
			})
		}, func() error {
			return service.SweepIdle(ctx)
		})
		if sweepErr != nil {
			t.Fatal(sweepErr)
		}
		if renewErr == nil {
			if _, _, err := repo.GetActivePartyByCharacter(ctx, member.CharacterID); err != nil {
				t.Fatalf("sweep removed renewed lobby: %v", err)
			}
			if len(sleep.deadlines[p.LeaderCharacterID]) != 0 {
				t.Fatal("sweep penalized a renewed lobby")
			}
			continue
		}
		if !errors.Is(renewErr, party.ErrPartyNotRecruiting) && !errors.Is(renewErr, party.ErrNotFound) {
			t.Fatal(renewErr)
		}
		if _, err := repo.GetParty(ctx, p.ID); !errors.Is(err, party.ErrNotFound) {
			t.Fatalf("timed out lobby still present: %v", err)
		}
		if len(sleep.deadlines[p.LeaderCharacterID]) != 1 || len(sleep.deadlines[member.CharacterID]) != 0 {
			t.Fatalf("sweep penalized a different roster than the one it removed")
		}
		clockNow = clockNow.Add(time.Second)
	}
}

func TestSweepIdleProcessesAtMostFiftyLobbies(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	clock := func() time.Time { return now }
	repo := party.NewMemoryRepository(party.WithLobbyClock(clock))
	sleep := &sleepRecorder{deadlines: make(map[string][]time.Time)}
	service := newIdleService(t, repo, repo, sleep, clock)
	for i := range 51 {
		seedIdleLobby(t, repo, sampleTestParty(fmt.Sprintf("lobby-%d", i), fmt.Sprintf("actor-%d", i), "Lobby"))
	}
	now = now.Add(30*time.Minute + time.Second)
	for _, want := range []int{50, 51} {
		if err := service.SweepIdle(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := len(sleep.deadlines); got != want {
			t.Fatalf("sleep deadlines after tick = %d, want %d", got, want)
		}
	}
}

func TestSweepIdleWithLiveValkey(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)
	p := sampleTestParty("idle", id.New(), "Lobby")
	ally := id.New()
	client, repo, prefix := liveLobbyRepository(t, []string{p.ID}, []string{p.LeaderCharacterID, ally})
	timers := timer.NewService(client)
	svc := newIdleService(t, repo, repo, timers, func() time.Time { return now })
	t.Cleanup(func() {
		for _, actor := range []string{p.LeaderCharacterID, ally} {
			if err := timers.ReleaseLock(context.Background(), timer.CategorySleep, actor); err != nil {
				t.Error(err)
			}
		}
	})
	seedIdleLobby(t, repo, p, ally)
	longer := now.Add(2 * time.Hour)
	if err := timers.SetLockUntil(ctx, timer.CategorySleep, ally, longer); err != nil {
		t.Fatal(err)
	}

	// Simulate rolling forward from the old creation-scored index and 30-minute keys.
	lobbyKey := prefix + "lobby:" + p.ID
	// The seeded lobby is converted to the old on-disk shape by removing the new field.
	raw, err := client.Do(ctx, client.B().Get().Key(lobbyKey).Build()).ToString()
	if err != nil {
		t.Fatal(err)
	}
	seedLegacyLobby(t, client, lobbyKey, prefix+"lobbies", p.ID, raw, now.Add(time.Minute))
	if err := svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetParty(ctx, p.ID); err != nil {
		t.Fatalf("sweep expired an active legacy lobby: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	for actor, until := range map[string]time.Time{p.LeaderCharacterID: now.Add(time.Hour), ally: longer} {
		if _, _, err := repo.GetActivePartyByCharacter(ctx, actor); !errors.Is(err, party.ErrNotFound) {
			t.Fatalf("timed out %s's membership = %v", actor, err)
		}
		key := "party2:timer:" + timer.CategorySleep + ":" + actor
		got, err := client.Do(ctx, client.B().Pexpiretime().Key(key).Build()).AsInt64()
		if err != nil || got != until.UnixMilli() {
			t.Fatalf("%s's sleep deadline = %d, %v, want %d", actor, got, err, until.UnixMilli())
		}
	}
}

func seedLegacyLobby(t *testing.T, client valkey.Client, key, index, roomID, raw string, expiry time.Time) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "idle_at_ms")
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := client.Do(ctx, client.B().Set().Key(key).Value(string(data)).Pxat(expiry).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := client.Do(ctx, client.B().Zadd().Key(index).ScoreMember().ScoreMember(float64(expiry.Add(-30*time.Minute).Unix()), roomID).Build()).Error(); err != nil {
		t.Fatal(err)
	}
}

func newIdleService(t *testing.T, repo party.Repository, idle party.LobbyTimeoutRepository, sleep party.SleepTimer, clock func() time.Time) *party.Service {
	t.Helper()
	svc, err := party.NewService(repo, &liveTestCharRepo{}, nil, liveTestStageProvider{}, liveTestMonsterProvider{}, nil,
		party.WithClock(clock), party.WithIdleSweep(idle, sleep))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func seedIdleLobby(t *testing.T, repo party.Repository, p party.Party, allies ...string) {
	t.Helper()
	if err := repo.SaveParty(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	for i, actor := range append([]string{p.LeaderCharacterID}, allies...) {
		m := party.Member{PartyID: p.ID, CharacterID: actor, IsLeader: i == 0, JoinedAt: p.CreatedAt.Add(time.Duration(i) * time.Second)}
		if err := repo.AddMember(t.Context(), m); err != nil {
			t.Fatal(err)
		}
	}
}
