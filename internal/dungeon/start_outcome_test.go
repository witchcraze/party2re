package dungeon_test

import (
	"context"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// postStartFailure models an admission whose enclosing transaction fails after
// the start callback has already published the run (e.g. SQL commit failure).
type postStartFailure struct {
	err   error
	after func()
}

func (a postStartFailure) AdmitParty(ctx context.Context, _, partyID string, ids []string, start func(context.Context, string, []string) error) error {
	if err := start(ctx, partyID, ids); err != nil {
		return err
	}
	if a.after != nil {
		a.after()
	}
	return a.err
}

func (a postStartFailure) AdmitSolo(ctx context.Context, _ string, start func(context.Context) error) error {
	if err := start(ctx); err != nil {
		return err
	}
	if a.after != nil {
		a.after()
	}
	return a.err
}

type readFailStore struct {
	dungeon.ActiveExpeditionStore
	err error
}

func (s *readFailStore) GetActiveExpedition(ctx context.Context, id string) (*dungeon.ActiveExpedition, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ActiveExpeditionStore.GetActiveExpedition(ctx, id)
}

func newOutcomeService(t *testing.T, adm dungeon.Admission, store dungeon.ActiveExpeditionStore) *dungeon.Service {
	t.Helper()
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{
		"char-1": createTestChar("char-1", 10, 150, 40, 30),
		"char-2": createTestChar("char-2", 10, 120, 35, 25),
	}}
	svc, err := dungeon.NewService(newMockDungeonRepo(), chars, &corebattle.Engine{},
		dungeon.WithAdmission(adm), dungeon.WithActiveExpeditionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// Dungeon admission performs no SQL writes: a failure after publication cannot
// undo the Valkey-owned run, so the verified run is the start result.
func TestStartAfterPublicationFailureKeepsVerifiedRun(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("commit acknowledgement lost")
	for _, tc := range []struct {
		name  string
		start func(*dungeon.Service) (*dungeon.ActiveExpedition, error)
	}{
		{"solo", func(s *dungeon.Service) (*dungeon.ActiveExpedition, error) {
			return s.StartExpedition(ctx, "char-1", "dungeon-01")
		}},
		{"party", func(s *dungeon.Service) (*dungeon.ActiveExpedition, error) {
			return s.StartPartyExpedition(ctx, "char-1", []string{"char-1", "char-2"}, "dungeon-01", "party-01")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := dungeon.NewMemoryExpeditionRepository()
			svc := newOutcomeService(t, postStartFailure{err: failure}, store)
			run, err := tc.start(svc)
			if err != nil || run == nil {
				t.Fatalf("published run must be returned: %+v %v", run, err)
			}
			active, err := store.GetActiveExpedition(ctx, "char-1")
			if err != nil || active == nil || active.ID != run.ID {
				t.Fatalf("run not preserved: %+v %v", active, err)
			}
			if _, err := tc.start(svc); !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
				t.Fatalf("second start must see the existing run, got %v", err)
			}
		})
	}
}

func TestStartAfterPublicationFailureReportsUnknownWhenRunUnverifiable(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("commit acknowledgement lost")
	for _, tc := range []struct {
		name  string
		setup func(*readFailStore) postStartFailure
	}{
		{"read failure", func(s *readFailStore) postStartFailure {
			return postStartFailure{err: failure, after: func() { s.err = errors.New("valkey unavailable") }}
		}},
		{"run absent", func(s *readFailStore) postStartFailure {
			return postStartFailure{err: failure, after: func() {
				run, err := s.ActiveExpeditionStore.GetActiveExpedition(ctx, "char-1")
				if err != nil || run == nil {
					t.Fatalf("published run missing: %+v %v", run, err)
				}
				if err := s.ActiveExpeditionStore.DeleteActiveExpedition(ctx, "char-1", run.ID); err != nil {
					t.Fatal(err)
				}
			}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &readFailStore{ActiveExpeditionStore: dungeon.NewMemoryExpeditionRepository()}
			svc := newOutcomeService(t, tc.setup(store), store)
			run, err := svc.StartExpedition(ctx, "char-1", "dungeon-01")
			if run != nil || !errors.Is(err, dungeon.ErrStartOutcomeUnknown) || !errors.Is(err, failure) {
				t.Fatalf("expected unknown outcome wrapping the cause, got %+v %v", run, err)
			}
		})
	}
}

func TestStartCallbackFailureIsNotAnUnknownOutcome(t *testing.T) {
	adm := postStartFailure{err: errors.New("unused")}
	svc := newOutcomeService(t, adm, dungeon.NewMemoryExpeditionRepository())
	if _, err := svc.StartExpedition(context.Background(), "char-1", "missing-dungeon"); !errors.Is(err, dungeon.ErrDungeonNotFound) || errors.Is(err, dungeon.ErrStartOutcomeUnknown) {
		t.Fatalf("pre-publication rejection must stay a plain rejection: %v", err)
	}
}
