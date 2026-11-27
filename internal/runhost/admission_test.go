package runhost

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type testRun struct{ State }

func (r *testRun) Host() *State { return &r.State }

type publicRunStore struct {
	Store[testRun]
	run *testRun
	err error
}

func (s publicRunStore) FindActorRun(_ context.Context, actor string) (*testRun, error) {
	if actor != "target" {
		return nil, errors.New("wrong actor")
	}
	return s.run, s.err
}

func (s publicRunStore) FindRun(context.Context, string) (*testRun, error) {
	return nil, errors.New("unexpected FindRun")
}

type stubChars struct {
	Characters
	player string
}

func (c stubChars) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	return corecharacter.Character{ID: id, PlayerID: c.player}, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestHasActiveRun(t *testing.T) {
	wantErr := errors.New("read failed")
	for _, tc := range []struct {
		run *testRun
		err error
	}{
		{}, {run: &testRun{State: State{ID: "private-run"}}}, {err: wantErr},
	} {
		s := &Service[testRun, *testRun]{cfg: Config[testRun]{Store: publicRunStore{run: tc.run, err: tc.err}}}
		active, err := s.HasActiveRun(context.Background(), "target")
		if active != (tc.run != nil) || !errors.Is(err, tc.err) {
			t.Fatalf("active=%v err=%v", active, err)
		}
	}
}

func TestObserveRunUsesSingleActorReadAndChecksOwnership(t *testing.T) {
	now := time.Date(2026, 2, 1, 1, 0, 0, 0, time.UTC)
	svc := func(run *testRun, err error) *Service[testRun, *testRun] {
		return &Service[testRun, *testRun]{cfg: Config[testRun]{
			Store:      publicRunStore{run: run, err: err},
			Characters: stubChars{player: "owner"},
			Clock:      fixedClock{now: now},
		}}
	}

	if obs, err := svc(nil, nil).ObserveRun(context.Background(), "owner", "target"); err != nil || obs != nil {
		t.Fatalf("no run: obs=%+v err=%v", obs, err)
	}
	if _, err := svc(&testRun{State: State{ID: "r1", Roster: []string{"other"}, Recipients: map[string]Recipient{"other": {PlayerID: "owner"}}}}, nil).ObserveRun(context.Background(), "owner", "target"); !errors.Is(err, ErrRecovery) {
		t.Fatalf("missing roster member: %v", err)
	}
	if _, err := svc(&testRun{State: State{ID: "r1", Roster: []string{"target"}, Recipients: map[string]Recipient{"target": {PlayerID: "stranger"}}}}, nil).ObserveRun(context.Background(), "owner", "target"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("mismatched recipient owner: %v", err)
	}
	timedOut := &testRun{State: State{
		ID: "r1", Version: 4, Round: 2, LeaderID: "target", Roster: []string{"target"},
		Admission:    Admission{Speed: 18},
		LastActivity: now.Add(-1801 * time.Second),
		LastActions:  map[string]time.Time{"target": now.Add(-10 * time.Second)},
		Recipients:   map[string]Recipient{"target": {PlayerID: "owner", TimerPending: true, Pending: Earnings{Gold: 7}}},
		TimedOut:     true,
	}}
	obs, err := svc(timedOut, nil).ObserveRun(context.Background(), "owner", "target")
	if err != nil || obs == nil || obs.Version != 4 || obs.Role != "leader" || !obs.IdleDue || !obs.Recipient.TimerPending || obs.Recipient.Pending.Gold != 7 || obs.Speed != 18 {
		t.Fatalf("timed-out observation: obs=%+v err=%v", obs, err)
	}
}
