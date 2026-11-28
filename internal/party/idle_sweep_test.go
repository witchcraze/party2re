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
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
		removable := fmt.Sprintf("removable-%d", i)
		seedIdleLobby(t, repo, p, removable)
		clockNow = clockNow.Add(30*time.Minute + time.Second)
		renewErr, sweepErr := testutil.RunRace2(func() error {
			return repo.WithPartyAdventureLock(ctx, p.ID, func(ctx context.Context) error {
				return repo.RemoveMember(ctx, p.ID, removable)
			})
		}, func() error {
			return service.SweepIdle(ctx)
		})
		if sweepErr != nil {
			t.Fatal(sweepErr)
		}
		if renewErr == nil {
			if _, _, err := repo.GetActivePartyByCharacter(ctx, p.LeaderCharacterID); err != nil {
				t.Fatalf("sweep removed renewed lobby: %v", err)
			}
			if len(sleep.deadlines[p.LeaderCharacterID]) != 0 || len(sleep.deadlines[removable]) != 0 {
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
		if len(sleep.deadlines[p.LeaderCharacterID]) != 1 || len(sleep.deadlines[removable]) != 1 {
			t.Fatalf("sweep penalized a different roster than the one it removed: %v", sleep.deadlines)
		}
		clockNow = clockNow.Add(time.Second)
	}
}

func TestSweepIdleRacesWithJoin(t *testing.T) {
	ctx := t.Context()
	clockNow := time.Now().UTC().Truncate(time.Millisecond)
	repo := party.NewMemoryRepository(party.WithLobbyClock(func() time.Time { return clockNow }))
	sleep := &sleepRecorder{deadlines: make(map[string][]time.Time)}
	chars := &liveTestCharRepo{chars: map[string]corecharacter.Character{}}
	service, err := party.NewService(repo, chars, nil, liveTestStageProvider{}, liveTestMonsterProvider{}, nil,
		party.WithClock(func() time.Time { return clockNow }), party.WithIdleSweep(repo, sleep))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testutil.GetStressConfig()
	for i := range cfg.Workers * cfg.OpsPerWorker {
		leaderID := fmt.Sprintf("join-leader-%d", i)
		joinerID := fmt.Sprintf("join-ally-%d", i)
		chars.chars[leaderID] = corecharacter.Character{ID: leaderID, Name: leaderID, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
		chars.chars[joinerID] = corecharacter.Character{ID: joinerID, Name: joinerID, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}

		created, err := service.CreateParty(ctx, leaderID, party.CreatePartyRequest{Name: "Lobby", StageID: "stage-1", Speed: 3})
		if err != nil {
			t.Fatal(err)
		}
		clockNow = clockNow.Add(30*time.Minute + time.Second)

		joinErr, sweepErr := testutil.RunRace2(func() error {
			_, err := service.JoinParty(ctx, created.Party.ID, joinerID, "")
			return err
		}, func() error {
			return service.SweepIdle(ctx)
		})
		if sweepErr != nil {
			t.Fatal(sweepErr)
		}
		if joinErr == nil {
			// Join won the lock first; because joining does not renew the idle
			// deadline, the next sweep tick expires the lobby and penalizes both.
			clockNow = clockNow.Add(time.Second)
			if err := service.SweepIdle(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.GetParty(ctx, created.Party.ID); !errors.Is(err, party.ErrNotFound) {
				t.Fatalf("expired lobby still present after join: %v", err)
			}
			if len(sleep.deadlines[leaderID]) != 1 || len(sleep.deadlines[joinerID]) != 1 {
				t.Fatalf("sweep penalized wrong roster after accepted join: leader=%v joiner=%v", sleep.deadlines[leaderID], sleep.deadlines[joinerID])
			}
		} else {
			if !errors.Is(joinErr, party.ErrPartyNotRecruiting) && !errors.Is(joinErr, party.ErrNotFound) {
				t.Fatal(joinErr)
			}
			if _, err := repo.GetParty(ctx, created.Party.ID); !errors.Is(err, party.ErrNotFound) {
				t.Fatalf("timed out lobby still present: %v", err)
			}
			if len(sleep.deadlines[leaderID]) != 1 || len(sleep.deadlines[joinerID]) != 0 {
				t.Fatalf("sweep penalized rejected joiner: leader=%v joiner=%v", sleep.deadlines[leaderID], sleep.deadlines[joinerID])
			}
		}
		clockNow = clockNow.Add(time.Second)
	}
}

func TestJoinPreservesIdleDeadlineAndInRoomCommandsRenew(t *testing.T) {
	ctx := t.Context()
	start := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)
	now := start
	clock := func() time.Time { return now }
	repo := party.NewMemoryRepository(party.WithLobbyClock(clock))
	sleep := &sleepRecorder{deadlines: make(map[string][]time.Time)}
	chars := &liveTestCharRepo{chars: map[string]corecharacter.Character{
		"leader-a": {ID: "leader-a", Name: "LeaderA", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
		"ally-a":   {ID: "ally-a", Name: "AllyA", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
		"leader-b": {ID: "leader-b", Name: "LeaderB", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
		"ally-b":   {ID: "ally-b", Name: "AllyB", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
	}}
	svc, err := party.NewService(repo, chars, nil, liveTestStageProvider{}, liveTestMonsterProvider{}, nil,
		party.WithClock(clock), party.WithIdleSweep(repo, sleep))
	if err != nil {
		t.Fatal(err)
	}

	lobbyA, err := svc.CreateParty(ctx, "leader-a", party.CreatePartyRequest{Name: "RoomA", StageID: "stage-1", Speed: 3, Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	lobbyB, err := svc.CreateParty(ctx, "leader-b", party.CreatePartyRequest{Name: "RoomB", StageID: "stage-1", Speed: 3})
	if err != nil {
		t.Fatal(err)
	}

	// Join at T0 + 29m: updates roster and reverse lookups, leaves idle deadline at T0 + 30m.
	now = start.Add(29 * time.Minute)
	detailA, err := svc.JoinParty(ctx, lobbyA.Party.ID, "ally-a", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(detailA.Members) != 2 {
		t.Fatalf("joined members = %d, want 2", len(detailA.Members))
	}
	if _, err := svc.JoinParty(ctx, lobbyB.Party.ID, "ally-b", ""); err != nil {
		t.Fatal(err)
	}
	for actor, wantID := range map[string]string{
		"leader-a": lobbyA.Party.ID,
		"ally-a":   lobbyA.Party.ID,
		"leader-b": lobbyB.Party.ID,
		"ally-b":   lobbyB.Party.ID,
	} {
		got, _, err := repo.GetActivePartyByCharacter(ctx, actor)
		if err != nil || got.ID != wantID {
			t.Fatalf("reverse lookup for %s = %q, %v, want %q", actor, got.ID, err, wantID)
		}
	}

	// Observations, rejected commands and an in-room kick at T0 + 29m30s.
	now = start.Add(29*time.Minute + 30*time.Second)
	if _, err := svc.ListParties(ctx, party.StatusRecruiting, 10, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetParty(ctx, lobbyA.Party.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinParty(ctx, lobbyA.Party.ID, "leader-b", "wrong"); !errors.Is(err, party.ErrInvalidPassword) {
		t.Fatalf("rejected join error = %v", err)
	}
	if _, err := svc.KickMember(ctx, lobbyA.Party.ID, "ally-a", "leader-a"); !errors.Is(err, party.ErrNotPartyLeader) {
		t.Fatalf("rejected kick error = %v", err)
	}
	if _, err := svc.KickMember(ctx, lobbyB.Party.ID, "leader-b", "ally-b"); err != nil {
		t.Fatal(err)
	}

	// T0 + 30m: neither lobby is strictly past 1800s yet.
	now = start.Add(30 * time.Minute)
	if err := svc.SweepIdle(ctx); err != nil || len(sleep.deadlines) != 0 {
		t.Fatalf("at 30m boundary: deadlines=%v err=%v", sleep.deadlines, err)
	}

	// T0 + 30m + 1s: lobbyA expires (join/reads/rejections did not renew); lobbyB survives (kick renewed).
	now = start.Add(30*time.Minute + time.Second)
	if err := svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	wantA := now.Add(time.Hour)
	if !reflect.DeepEqual(sleep.deadlines, map[string][]time.Time{"leader-a": {wantA}, "ally-a": {wantA}}) {
		t.Fatalf("sleep deadlines at T0+30m1s = %v", sleep.deadlines)
	}
	for _, actor := range []string{"leader-a", "ally-a"} {
		if _, _, err := repo.GetActivePartyByCharacter(ctx, actor); !errors.Is(err, party.ErrNotFound) {
			t.Fatalf("%s reverse lookup after expiry = %v", actor, err)
		}
	}
	if _, err := repo.GetParty(ctx, lobbyB.Party.ID); err != nil {
		t.Fatalf("kicked lobby expired prematurely: %v", err)
	}

	// T0 + 59m30s: still at the renewed 30m boundary from the kick; expires at +59m31s.
	now = start.Add(59*time.Minute + 30*time.Second)
	if err := svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetParty(ctx, lobbyB.Party.ID); err != nil {
		t.Fatalf("kicked lobby expired at 30m boundary: %v", err)
	}
	now = now.Add(time.Second)
	if err := svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetParty(ctx, lobbyB.Party.ID); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("kicked lobby still present after renewed idle deadline: %v", err)
	}
	if got := sleep.deadlines["leader-b"]; !reflect.DeepEqual(got, []time.Time{now.Add(time.Hour)}) {
		t.Fatalf("leader-b sleep deadline = %v", got)
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

func TestSweepIdleLegacyRosterExcludesMembersOfAnotherLobby(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)
	old := sampleTestParty(id.New(), id.New(), "Old lobby")
	current := sampleTestParty(id.New(), id.New(), "Current lobby")
	moved, missing := old.LeaderCharacterID, id.New()
	client, repo, prefix := liveLobbyRepository(t, []string{old.ID, current.ID}, []string{moved, missing, current.LeaderCharacterID})
	sleep := &sleepRecorder{failActor: missing, deadlines: make(map[string][]time.Time)}
	service := newIdleService(t, repo, repo, sleep, func() time.Time { return now })
	seedIdleLobby(t, repo, old, missing)
	seedIdleLobby(t, repo, current)

	// The old lobby body outlived the leader's original character mapping.
	if err := client.Do(ctx, client.B().Del().Key(prefix+"character:"+moved).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(ctx, party.Member{PartyID: current.ID, CharacterID: moved, JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	lobbyKey := prefix + "lobby:" + old.ID
	raw, err := client.Do(ctx, client.B().Get().Key(lobbyKey).Build()).ToString()
	if err != nil {
		t.Fatal(err)
	}
	seedLegacyLobby(t, client, lobbyKey, prefix+"lobbies", old.ID, raw, now.Add(time.Minute))
	if err := service.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Do(ctx, client.B().Del().Key(prefix+"character:"+missing).Build()).Error(); err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Minute)
	until := now.Add(time.Hour)
	if err := service.SweepIdle(ctx); !errors.Is(err, errSleepWrite) {
		t.Fatalf("first timeout = %v, want a sleep write failure", err)
	}
	if got := sleep.deadlines[moved]; len(got) != 0 {
		t.Errorf("first timeout penalized another lobby's member: %v", got)
	}
	if p, _, err := repo.GetActivePartyByCharacter(ctx, moved); err != nil || p.ID != current.ID {
		t.Fatalf("current lobby ownership = %q, %v", p.ID, err)
	}
	// Removing the newer membership cannot make the frozen old timeout claim it.
	if err := repo.RemoveMember(ctx, current.ID, moved); err != nil {
		t.Fatal(err)
	}
	sleep.failActor = ""
	now = now.Add(5 * time.Second)
	if err := service.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	want := map[string][]time.Time{missing: {until}}
	if !reflect.DeepEqual(sleep.deadlines, want) {
		t.Fatalf("sleep deadlines = %v, want %v", sleep.deadlines, want)
	}
	if _, err := repo.GetParty(ctx, old.ID); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("old lobby after retry = %v", err)
	}
}

func liveLobbyRepositoryWithClock(t *testing.T, partyIDs, actors []string, clock func() time.Time) (valkey.Client, *party.ValkeyRepository, string) {
	t.Helper()
	client, closeClient := getLiveValkeyClient(t)
	t.Cleanup(closeClient)
	prefix := "party2:test:party-lobby:" + id.New() + ":"
	t.Cleanup(func() {
		ctx := context.Background()
		keys := []string{prefix + "lobbies"}
		for _, pid := range partyIDs {
			keys = append(keys, prefix+"lobby:"+pid)
		}
		for _, actor := range actors {
			keys = append(keys, prefix+"character:"+actor)
		}
		for _, key := range keys {
			if err := client.Do(ctx, client.B().Del().Key(key).Build()).Error(); err != nil {
				t.Error(err)
			}
		}
	})
	repo := party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(prefix+"lobby:"),
		party.WithCharacterKeyPrefix(prefix+"character:"),
		party.WithLobbiesIndexKey(prefix+"lobbies"),
		party.WithLobbyClock(clock),
	)
	return client, repo, prefix
}

func TestJoinPreservesIdleDeadlineWithLiveValkey(t *testing.T) {
	ctx := t.Context()
	start := time.Now().UTC().Truncate(time.Millisecond)
	now := start
	clock := func() time.Time { return now }

	leaderA, allyA := id.New(), id.New()
	leaderB, allyB := id.New(), id.New()
	legacyLeader, legacyJoiner := id.New(), id.New()
	legacyParty := sampleTestParty(id.New(), legacyLeader, "Legacy")
	// Pre-create client to clean up dynamic party IDs created via CreateParty.
	client, closeClient := getLiveValkeyClient(t)
	t.Cleanup(closeClient)
	prefix := "party2:test:party-lobby:" + id.New() + ":"
	var createdPartyIDs []string
	t.Cleanup(func() {
		bg := context.Background()
		keys := []string{prefix + "lobbies", prefix + "lobby:" + legacyParty.ID}
		for _, pid := range createdPartyIDs {
			keys = append(keys, prefix+"lobby:"+pid)
		}
		for _, actor := range []string{leaderA, allyA, leaderB, allyB, legacyLeader, legacyJoiner} {
			keys = append(keys, prefix+"character:"+actor)
		}
		for _, key := range keys {
			if err := client.Do(bg, client.B().Del().Key(key).Build()).Error(); err != nil {
				t.Error(err)
			}
		}
	})
	repo := party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(prefix+"lobby:"),
		party.WithCharacterKeyPrefix(prefix+"character:"),
		party.WithLobbiesIndexKey(prefix+"lobbies"),
		party.WithLobbyClock(clock),
	)
	timers := timer.NewService(client)
	t.Cleanup(func() {
		for _, actor := range []string{leaderA, allyA, leaderB, allyB} {
			if err := timers.ReleaseLock(context.Background(), timer.CategorySleep, actor); err != nil {
				t.Error(err)
			}
		}
	})
	chars := &liveTestCharRepo{chars: map[string]corecharacter.Character{
		leaderA: {ID: leaderA, Name: "LeaderA", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
		allyA:   {ID: allyA, Name: "AllyA", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
		leaderB: {ID: leaderB, Name: "LeaderB", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
		allyB:   {ID: allyB, Name: "AllyB", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
	}}
	svc, err := party.NewService(repo, chars, nil, liveTestStageProvider{}, liveTestMonsterProvider{}, nil,
		party.WithClock(clock), party.WithIdleSweep(repo, timers))
	if err != nil {
		t.Fatal(err)
	}

	lobbyA, err := svc.CreateParty(ctx, leaderA, party.CreatePartyRequest{Name: "RoomA", StageID: "stage-1", Speed: 3, Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	createdPartyIDs = append(createdPartyIDs, lobbyA.Party.ID)
	lobbyB, err := svc.CreateParty(ctx, leaderB, party.CreatePartyRequest{Name: "RoomB", StageID: "stage-1", Speed: 3})
	if err != nil {
		t.Fatal(err)
	}
	createdPartyIDs = append(createdPartyIDs, lobbyB.Party.ID)

	zscore := func(partyID string) int64 {
		t.Helper()
		score, err := client.Do(ctx, client.B().Zscore().Key(prefix+"lobbies").Member(partyID).Build()).AsInt64()
		if err != nil {
			t.Fatalf("ZSCORE %s: %v", partyID, err)
		}
		return score
	}
	wantInitialDue := start.Add(30 * time.Minute).UnixMilli()
	if got := zscore(lobbyA.Party.ID); got != wantInitialDue {
		t.Fatalf("initial lobbyA due = %d, want %d", got, wantInitialDue)
	}

	// Join at T0 + 29m: updates roster and both reverse lookups, preserves idle deadline.
	now = start.Add(29 * time.Minute)
	if _, err := svc.JoinParty(ctx, lobbyA.Party.ID, allyA, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinParty(ctx, lobbyB.Party.ID, allyB, ""); err != nil {
		t.Fatal(err)
	}
	if got := zscore(lobbyA.Party.ID); got != wantInitialDue {
		t.Fatalf("lobbyA due changed after join: got %d, want %d", got, wantInitialDue)
	}
	membersA, err := repo.GetMembers(ctx, lobbyA.Party.ID)
	if err != nil || len(membersA) != 2 || membersA[0].CharacterID != leaderA || membersA[1].CharacterID != allyA {
		t.Fatalf("lobbyA members after join = %+v, err=%v", membersA, err)
	}
	for _, actor := range []string{leaderA, allyA} {
		got, _, err := repo.GetActivePartyByCharacter(ctx, actor)
		if err != nil || got.ID != lobbyA.Party.ID {
			t.Fatalf("reverse lookup for %s = %q, %v", actor, got.ID, err)
		}
	}

	// Legacy lobby without idle_at_ms preserves its existing expiry on AddMember.
	seedIdleLobby(t, repo, legacyParty)
	legacyKey := prefix + "lobby:" + legacyParty.ID
	raw, err := client.Do(ctx, client.B().Get().Key(legacyKey).Build()).ToString()
	if err != nil {
		t.Fatal(err)
	}
	legacyDeadline := start.Add(30 * time.Minute)
	seedLegacyLobby(t, client, legacyKey, prefix+"lobbies", legacyParty.ID, raw, legacyDeadline)
	if err := repo.AddMember(ctx, party.Member{PartyID: legacyParty.ID, CharacterID: legacyJoiner, JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got := zscore(legacyParty.ID); got != legacyDeadline.UnixMilli() {
		t.Fatalf("legacy lobby due after AddMember = %d, want %d", got, legacyDeadline.UnixMilli())
	}
	if err := repo.DeleteParty(ctx, legacyParty.ID); err != nil {
		t.Fatal(err)
	}

	// Reads, rejected requests, and in-room kick at T0 + 29m30s.
	now = start.Add(29*time.Minute + 30*time.Second)
	if _, err := svc.ListParties(ctx, party.StatusRecruiting, 10, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetParty(ctx, lobbyA.Party.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinParty(ctx, lobbyA.Party.ID, leaderB, "wrong"); !errors.Is(err, party.ErrInvalidPassword) {
		t.Fatalf("rejected join = %v", err)
	}
	if _, err := svc.KickMember(ctx, lobbyA.Party.ID, allyA, leaderA); !errors.Is(err, party.ErrNotPartyLeader) {
		t.Fatalf("rejected kick = %v", err)
	}
	if got := zscore(lobbyA.Party.ID); got != wantInitialDue {
		t.Fatalf("lobbyA due changed after reads/rejections: got %d, want %d", got, wantInitialDue)
	}
	if _, err := svc.KickMember(ctx, lobbyB.Party.ID, leaderB, allyB); err != nil {
		t.Fatal(err)
	}
	wantKickDue := now.Add(30 * time.Minute).UnixMilli()
	if got := zscore(lobbyB.Party.ID); got != wantKickDue {
		t.Fatalf("lobbyB due after kick = %d, want %d", got, wantKickDue)
	}

	// T0 + 30m + 1s: lobbyA expires and sleeps both members; lobbyB survives.
	now = start.Add(30*time.Minute + time.Second)
	if err := svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetParty(ctx, lobbyA.Party.ID); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("lobbyA still present after expiry: %v", err)
	}
	for _, actor := range []string{leaderA, allyA} {
		if _, _, err := repo.GetActivePartyByCharacter(ctx, actor); !errors.Is(err, party.ErrNotFound) {
			t.Fatalf("%s reverse lookup after expiry = %v", actor, err)
		}
		sleepKey := "party2:timer:" + timer.CategorySleep + ":" + actor
		got, err := client.Do(ctx, client.B().Pexpiretime().Key(sleepKey).Build()).AsInt64()
		if err != nil || got != now.Add(time.Hour).UnixMilli() {
			t.Fatalf("%s sleep deadline = %d, %v, want %d", actor, got, err, now.Add(time.Hour).UnixMilli())
		}
	}
	if _, err := repo.GetParty(ctx, lobbyB.Party.ID); err != nil {
		t.Fatalf("lobbyB expired prematurely: %v", err)
	}

	// T0 + 59m31s: lobbyB expires after its renewed deadline.
	now = start.Add(59*time.Minute + 31*time.Second)
	if err := svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetParty(ctx, lobbyB.Party.ID); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("lobbyB still present after renewed expiry: %v", err)
	}
	sleepKeyB := "party2:timer:" + timer.CategorySleep + ":" + leaderB
	gotB, err := client.Do(ctx, client.B().Pexpiretime().Key(sleepKeyB).Build()).AsInt64()
	if err != nil || gotB != now.Add(time.Hour).UnixMilli() {
		t.Fatalf("%s sleep deadline = %d, %v, want %d", leaderB, gotB, err, now.Add(time.Hour).UnixMilli())
	}
}

func TestSweepIdleRacesWithJoinLiveValkey(t *testing.T) {
	ctx := t.Context()
	start := time.Now().UTC().Truncate(time.Millisecond)
	now := start
	clock := func() time.Time { return now }
	leaderID, joinerID := id.New(), id.New()
	p := sampleTestParty(id.New(), leaderID, "RaceLobby")
	client, repo, _ := liveLobbyRepositoryWithClock(t, []string{p.ID}, []string{leaderID, joinerID}, clock)
	timers := timer.NewService(client)
	t.Cleanup(func() {
		for _, actor := range []string{leaderID, joinerID} {
			if err := timers.ReleaseLock(context.Background(), timer.CategorySleep, actor); err != nil {
				t.Error(err)
			}
		}
	})
	chars := &liveTestCharRepo{chars: map[string]corecharacter.Character{
		leaderID: {ID: leaderID, Name: "Leader", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
		joinerID: {ID: joinerID, Name: "Joiner", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
	}}
	svc, err := party.NewService(repo, chars, nil, liveTestStageProvider{}, liveTestMonsterProvider{}, nil,
		party.WithClock(clock), party.WithIdleSweep(repo, timers))
	if err != nil {
		t.Fatal(err)
	}
	p.CreatedAt = start
	p.UpdatedAt = start
	if err := repo.SaveParty(ctx, p, party.Member{PartyID: p.ID, CharacterID: leaderID, IsLeader: true, JoinedAt: start}); err != nil {
		t.Fatal(err)
	}
	now = start.Add(30*time.Minute + time.Second)
	joinErr, sweepErr := testutil.RunRace2(func() error {
		_, err := svc.JoinParty(ctx, p.ID, joinerID, "")
		return err
	}, func() error {
		return svc.SweepIdle(ctx)
	})
	if sweepErr != nil {
		t.Fatal(sweepErr)
	}
	now = now.Add(time.Second)
	if err := svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetParty(ctx, p.ID); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("expired lobby still present: %v", err)
	}
	wantJoinerSleep := joinErr == nil
	if joinErr != nil && !errors.Is(joinErr, party.ErrPartyNotRecruiting) && !errors.Is(joinErr, party.ErrNotFound) {
		t.Fatalf("unexpected join error: %v", joinErr)
	}
	for actor, wantSleep := range map[string]bool{leaderID: true, joinerID: wantJoinerSleep} {
		locked, err := timers.IsLocked(ctx, timer.CategorySleep, actor)
		if err != nil {
			t.Fatal(err)
		}
		if locked != wantSleep {
			t.Fatalf("sleep lock for %s = %v, want %v (joinErr=%v)", actor, locked, wantSleep, joinErr)
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
