package challenge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// postStartFailure models an admission whose enclosing SQL transaction fails
// after the start callback has already published the run. rollback simulates
// the effect on SQL writes of a confirmed abort.
type postStartFailure struct {
	err      error
	rollback func()
}

func (a postStartFailure) AdmitParty(ctx context.Context, _, partyID string, ids []string, start func(context.Context, string, []string) error) error {
	if err := start(ctx, partyID, ids); err != nil {
		return err
	}
	if a.rollback != nil {
		a.rollback()
	}
	return a.err
}

func (a postStartFailure) AdmitSolo(ctx context.Context, _ string, start func(context.Context) error) error {
	if err := start(ctx); err != nil {
		return err
	}
	if a.rollback != nil {
		a.rollback()
	}
	return a.err
}

// publishThenFailStore publishes the run and then reports a write failure.
type publishThenFailStore struct{ challenge.ActiveSessionStore }

func (s publishThenFailStore) SaveActiveSession(ctx context.Context, session challenge.ChallengeSession) error {
	if err := s.ActiveSessionStore.SaveActiveSession(ctx, session); err != nil {
		return err
	}
	return errors.New("valkey acknowledgement lost")
}

type outcomeFixture struct {
	repo   *mockChallengeRepo
	store  *mockActiveStore
	memory *challenge.MemorySessionRepository
}

func newOutcomeService(t *testing.T, adm func(*outcomeFixture) challenge.Admission) (*challenge.Service, *outcomeFixture) {
	t.Helper()
	fx := &outcomeFixture{repo: newMockChallengeRepo(), memory: challenge.NewMemorySessionRepository()}
	fx.store = newMockActiveStore(fx.memory)
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{
		"lead": createPartyTestChar("lead", 15, 200, 50, 30),
		"c2":   createPartyTestChar("c2", 15, 180, 45, 25),
	}}
	svc, err := challenge.NewService(fx.repo, chars, &corebattle.Engine{},
		challenge.WithActiveSessionStore(fx.store), challenge.WithAdmission(adm(fx)))
	if err != nil {
		t.Fatal(err)
	}
	return svc, fx
}

func (fx *outcomeFixture) abort() { fx.repo.sessions = map[string]challenge.ChallengeSession{} }

func assertNoActiveRun(t *testing.T, fx *outcomeFixture, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if run, err := fx.memory.GetActiveSession(context.Background(), id); err != nil || run != nil {
			t.Fatalf("%s still has an active run: %+v %v", id, run, err)
		}
	}
}

func startBoth(svc *challenge.Service) (*challenge.ChallengeSession, error) {
	return svc.StartPartySession(context.Background(), "lead", []string{"lead", "c2"}, "1", "Party", "#FFFFFF")
}

var errCommit = errors.New("commit acknowledgement lost")

func TestStartCommitUnknownResolvesCommittedRun(t *testing.T) {
	svc, fx := newOutcomeService(t, func(*outcomeFixture) challenge.Admission { return postStartFailure{err: errCommit} })
	run, err := startBoth(svc)
	if err != nil || run == nil {
		t.Fatalf("committed start must return its run: %+v %v", run, err)
	}
	if _, ok := fx.repo.sessions[run.ID]; !ok {
		t.Fatal("SQL start row missing")
	}
	for _, id := range []string{"lead", "c2"} {
		if active, err := fx.memory.GetActiveSession(context.Background(), id); err != nil || active == nil || active.ID != run.ID {
			t.Fatalf("%s lost the run: %+v %v", id, active, err)
		}
	}
	if _, err := startBoth(svc); !errors.Is(err, challenge.ErrActiveSessionExists) {
		t.Fatalf("no duplicate run: %v", err)
	}
}

func TestStartConfirmedAbortRemovesOnlyThatRun(t *testing.T) {
	svc, fx := newOutcomeService(t, func(fx *outcomeFixture) challenge.Admission {
		return postStartFailure{err: errCommit, rollback: fx.abort}
	})
	run, err := startBoth(svc)
	if run != nil || !errors.Is(err, errCommit) || errors.Is(err, challenge.ErrStartOutcomeUnknown) {
		t.Fatalf("confirmed abort must return the original failure: %+v %v", run, err)
	}
	assertNoActiveRun(t, fx, "lead", "c2")
	// The roster is no longer unexplainably busy.
	svc2, _ := newOutcomeService(t, func(*outcomeFixture) challenge.Admission { return postStartFailure{} })
	if _, err := startBoth(svc2); err != nil {
		t.Fatalf("retry after abort: %v", err)
	}
}

func TestStartConfirmedAbortPreservesReplacementRun(t *testing.T) {
	svc, fx := newOutcomeService(t, func(fx *outcomeFixture) challenge.Admission {
		return postStartFailure{err: errCommit, rollback: func() {
			fx.abort()
			replacement := challenge.ChallengeSession{ID: "replacement", CharacterID: "lead", Status: challenge.StatusActive,
				Members: []challenge.ChallengeMember{{CharacterID: "lead"}, {CharacterID: "c2"}}}
			if err := fx.memory.DeleteActiveSession(context.Background(), "lead"); err != nil {
				t.Fatal(err)
			}
			if err := fx.memory.SaveActiveSession(context.Background(), replacement); err != nil {
				t.Fatal(err)
			}
		}}
	})
	if _, err := startBoth(svc); !errors.Is(err, errCommit) {
		t.Fatalf("expected original failure: %v", err)
	}
	for _, id := range []string{"lead", "c2"} {
		if active, err := fx.memory.GetActiveSession(context.Background(), id); err != nil || active == nil || active.ID != "replacement" {
			t.Fatalf("replacement run for %s must be preserved: %+v %v", id, active, err)
		}
	}
}

func TestStartUnresolvedCommitKeepsRunAndReportsUnknown(t *testing.T) {
	svc, fx := newOutcomeService(t, func(fx *outcomeFixture) challenge.Admission {
		return postStartFailure{err: errCommit, rollback: func() { fx.repo.findSessionByIDErr = errors.New("mariadb unavailable") }}
	})
	run, err := startBoth(svc)
	if run != nil || !errors.Is(err, challenge.ErrStartOutcomeUnknown) || !errors.Is(err, errCommit) {
		t.Fatalf("expected unknown outcome: %+v %v", run, err)
	}
	for _, id := range []string{"lead", "c2"} {
		if active, err := fx.memory.GetActiveSession(context.Background(), id); err != nil || active == nil {
			t.Fatalf("unknown outcome must not delete state for %s: %+v %v", id, active, err)
		}
	}
}

func TestStartConfirmedAbortCleanupFailureReportsUnknownAndPreservesEvidence(t *testing.T) {
	cleanup := errors.New("valkey delete failed")
	svc, fx := newOutcomeService(t, func(fx *outcomeFixture) challenge.Admission {
		return postStartFailure{err: errCommit, rollback: func() {
			fx.abort()
			fx.store.discardActiveSessionErr = cleanup
		}}
	})
	_, err := startBoth(svc)
	if !errors.Is(err, challenge.ErrStartOutcomeUnknown) || !errors.Is(err, errCommit) || !errors.Is(err, cleanup) {
		t.Fatalf("admission failure, cleanup failure, and unknown outcome must all be visible: %v", err)
	}
	if active, _ := fx.memory.GetActiveSession(context.Background(), "lead"); active == nil {
		t.Fatal("failed cleanup must leave the run discoverable, not hidden")
	}
}

func TestStartSQLInsertFailureNeverPublishesRun(t *testing.T) {
	svc, fx := newOutcomeService(t, func(*outcomeFixture) challenge.Admission { return postStartFailure{} })
	fx.repo.saveSessionErr = errors.New("duplicate key in challenge_sessions")
	if _, err := startBoth(svc); err == nil {
		t.Fatal("expected insert failure")
	}
	assertNoActiveRun(t, fx, "lead", "c2")
}

func TestStartPublicationWriteFailureDiscardsPartialRun(t *testing.T) {
	repo := newMockChallengeRepo()
	memory := challenge.NewMemorySessionRepository()
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{"lead": createPartyTestChar("lead", 15, 200, 50, 30)}}
	svc, err := challenge.NewService(repo, chars, &corebattle.Engine{},
		challenge.WithActiveSessionStore(publishThenFailStore{memory}), challenge.WithAdmission(postStartFailure{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartSession(context.Background(), "lead", "1"); err == nil {
		t.Fatal("expected publication failure")
	}
	if run, err := memory.GetActiveSession(context.Background(), "lead"); err != nil || run != nil {
		t.Fatalf("partial run left behind: %+v %v", run, err)
	}
}

type publishAndDiscardFailStore struct {
	challenge.ActiveSessionStore
	saveErr    error
	discardErr error
}

func (s publishAndDiscardFailStore) SaveActiveSession(ctx context.Context, session challenge.ChallengeSession) error {
	if err := s.ActiveSessionStore.SaveActiveSession(ctx, session); err != nil {
		return err
	}
	return s.saveErr
}

func (s publishAndDiscardFailStore) DiscardActiveSession(ctx context.Context, session challenge.ChallengeSession) error {
	return s.discardErr
}

func TestStartPublicationLostResponseCleanupFailureReportsUnknownAndPreservesEvidence(t *testing.T) {
	saveErr := errors.New("valkey save response lost")
	discardErr := errors.New("valkey discard failed")
	repo := newMockChallengeRepo()
	memory := challenge.NewMemorySessionRepository()
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{
		"lead": createPartyTestChar("lead", 15, 200, 50, 30),
		"c2":   createPartyTestChar("c2", 15, 180, 45, 25),
	}}
	store := publishAndDiscardFailStore{
		ActiveSessionStore: memory,
		saveErr:            saveErr,
		discardErr:         discardErr,
	}
	svc, err := challenge.NewService(repo, chars, &corebattle.Engine{},
		challenge.WithActiveSessionStore(store), challenge.WithAdmission(postStartFailure{}))
	if err != nil {
		t.Fatal(err)
	}

	// 1. Solo start
	run, err := svc.StartSession(context.Background(), "lead", "1")
	if run != nil {
		t.Fatalf("expected nil run on failure: %+v", run)
	}
	if !errors.Is(err, challenge.ErrStartOutcomeUnknown) || !errors.Is(err, saveErr) || !errors.Is(err, discardErr) {
		t.Fatalf("expected ErrStartOutcomeUnknown, saveErr, and discardErr: %v", err)
	}
	if active, getErr := memory.GetActiveSession(context.Background(), "lead"); getErr != nil || active == nil {
		t.Fatalf("session in active store must be preserved as evidence: %+v %v", active, getErr)
	}

	// Clean up memory store for next case
	_ = memory.DeleteActiveSession(context.Background(), "lead")

	// 2. Party start
	run, err = svc.StartPartySession(context.Background(), "lead", []string{"lead", "c2"}, "1", "Party", "#FFFFFF")
	if run != nil {
		t.Fatalf("expected nil run on party failure: %+v", run)
	}
	if !errors.Is(err, challenge.ErrStartOutcomeUnknown) || !errors.Is(err, saveErr) || !errors.Is(err, discardErr) {
		t.Fatalf("party start must return ErrStartOutcomeUnknown, saveErr, and discardErr: %v", err)
	}
	for _, id := range []string{"lead", "c2"} {
		if active, getErr := memory.GetActiveSession(context.Background(), id); getErr != nil || active == nil {
			t.Fatalf("member %s run in active store must be preserved as evidence: %+v %v", id, active, getErr)
		}
	}
}

type captureContextStore struct {
	challenge.ActiveSessionStore
	saveErr        error
	discardErr     error
	discardInvoked bool
	discardCtxErr  error
}

func (s *captureContextStore) SaveActiveSession(ctx context.Context, session challenge.ChallengeSession) error {
	_ = s.ActiveSessionStore.SaveActiveSession(ctx, session)
	return s.saveErr
}

func (s *captureContextStore) DiscardActiveSession(ctx context.Context, session challenge.ChallengeSession) error {
	s.discardInvoked = true
	s.discardCtxErr = ctx.Err()
	if s.discardErr != nil {
		return s.discardErr
	}
	return s.ActiveSessionStore.DiscardActiveSession(ctx, session)
}

func TestStartPublicationCanceledContextUsesDetachedContextForCleanup(t *testing.T) {
	repo := newMockChallengeRepo()
	memory := challenge.NewMemorySessionRepository()
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{
		"lead": createPartyTestChar("lead", 15, 200, 50, 30),
	}}
	saveErr := context.Canceled
	store := &captureContextStore{
		ActiveSessionStore: memory,
		saveErr:            saveErr,
	}
	svc, err := challenge.NewService(repo, chars, &corebattle.Engine{},
		challenge.WithActiveSessionStore(store), challenge.WithAdmission(postStartFailure{}))
	if err != nil {
		t.Fatal(err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // canceled before calling StartSession

	_, err = svc.StartSession(canceledCtx, "lead", "1")
	if !errors.Is(err, saveErr) {
		t.Fatalf("expected saveErr: %v", err)
	}
	if errors.Is(err, challenge.ErrStartOutcomeUnknown) {
		t.Fatalf("clean discard must not return ErrStartOutcomeUnknown: %v", err)
	}

	// Verify discard was invoked with a detached, uncanceled context!
	if !store.discardInvoked {
		t.Fatal("expected DiscardActiveSession to be invoked")
	}
	if store.discardCtxErr != nil {
		t.Fatalf("cleanup context must not be canceled during execution: %v", store.discardCtxErr)
	}
	// Verify memory store was cleanly emptied
	if active, _ := memory.GetActiveSession(context.Background(), "lead"); active != nil {
		t.Fatalf("expected active session to be discarded, found: %+v", active)
	}
}

func TestStartPublicationCanceledContextCleanupFailureReportsUnknown(t *testing.T) {
	repo := newMockChallengeRepo()
	memory := challenge.NewMemorySessionRepository()
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{
		"lead": createPartyTestChar("lead", 15, 200, 50, 30),
	}}
	saveErr := context.Canceled
	discardErr := errors.New("valkey down during discard")
	store := &captureContextStore{
		ActiveSessionStore: memory,
		saveErr:            saveErr,
		discardErr:         discardErr,
	}
	svc, err := challenge.NewService(repo, chars, &corebattle.Engine{},
		challenge.WithActiveSessionStore(store), challenge.WithAdmission(postStartFailure{}))
	if err != nil {
		t.Fatal(err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = svc.StartSession(canceledCtx, "lead", "1")
	if !errors.Is(err, challenge.ErrStartOutcomeUnknown) || !errors.Is(err, saveErr) || !errors.Is(err, discardErr) {
		t.Fatalf("expected ErrStartOutcomeUnknown, saveErr, and discardErr: %v", err)
	}
	if active, _ := memory.GetActiveSession(context.Background(), "lead"); active == nil {
		t.Fatal("session must remain in memory when cleanup fails")
	}
}
