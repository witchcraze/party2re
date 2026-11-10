package rescue

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corescheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
)

type stubRescueRepo struct {
	records          []RescueRecord
	saveErr, markErr error
}

func (r *stubRescueRepo) Save(_ context.Context, record RescueRecord) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.records = append([]RescueRecord{record}, r.records...)
	return nil
}

func (r *stubRescueRepo) FindLatestByCharacterID(_ context.Context, characterID string) (RescueRecord, error) {
	for _, rec := range r.records {
		if rec.CharacterID == characterID {
			return rec, nil
		}
	}
	return RescueRecord{}, ErrNoRescueRecord
}

func (r *stubRescueRepo) MarkApplied(_ context.Context, id string, at time.Time) error {
	if r.markErr != nil {
		return r.markErr
	}
	for i := range r.records {
		if r.records[i].ID == id && r.records[i].AppliedAt.IsZero() {
			r.records[i].AppliedAt = at
		}
	}
	return nil
}

type stubCharRepo struct {
	characters map[string]corecharacter.Character
	locks      int
}

func (r *stubCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *stubCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	r.locks++
	return r.FindByID(ctx, id)
}

// stubActionCleaner holds pending scheduled work per character; nil stuckCharacters means every character has work.
type stubActionCleaner struct {
	clearedCharacters []string
	stuckCharacters   map[string]bool
	err               error
}

func (c *stubActionCleaner) hasWork(characterID string) bool {
	return c.stuckCharacters == nil || c.stuckCharacters[characterID]
}

func (c *stubActionCleaner) FindPendingByActorID(_ context.Context, characterID string) ([]corescheduling.ScheduledAction, error) {
	if !c.hasWork(characterID) {
		return nil, nil
	}
	return []corescheduling.ScheduledAction{{ID: "work", ActorID: characterID, State: corescheduling.StatePending}}, nil
}

func (c *stubActionCleaner) ClearActiveActions(_ context.Context, characterID string) (bool, error) {
	c.clearedCharacters = append(c.clearedCharacters, characterID)
	if c.err != nil {
		return false, c.err
	}
	cleared := c.hasWork(characterID)
	if c.stuckCharacters == nil {
		c.stuckCharacters = map[string]bool{}
	}
	c.stuckCharacters[characterID] = false
	return cleared, nil
}

type stubTx struct{}

func (stubTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

// faultyTimer injects sleep-timer read/write failures over a real in-memory timer.
type faultyTimer struct {
	timer.Service
	getErr, setErr error
}

func (t *faultyTimer) GetRemainingLock(ctx context.Context, category, id string) (time.Duration, error) {
	if t.getErr != nil {
		return 0, t.getErr
	}
	return t.Service.GetRemainingLock(ctx, category, id)
}

func (t *faultyTimer) SetLock(ctx context.Context, category, id string, d time.Duration) error {
	if t.setErr != nil {
		return t.setErr
	}
	return t.Service.SetLock(ctx, category, id, d)
}

func newTestService(repo RescueRepository, chars CharacterRepository, cleaner ActionCleaner, timers TimerService) *Service {
	return NewService(repo, chars, cleaner, timers, stubTx{})
}

func heroRepo() *stubCharRepo {
	return &stubCharRepo{characters: map[string]corecharacter.Character{"char-1": {ID: "char-1", Name: "StuckHero"}}}
}

func assertSleep(t *testing.T, timers timer.Service, want time.Duration) {
	t.Helper()
	rem, err := timers.GetRemainingLock(context.Background(), timer.CategorySleep, "char-1")
	if err != nil || rem < want-10*time.Second || rem > want {
		t.Fatalf("sleep remaining = %v (err %v), want ~%v", rem, err, want)
	}
}

func TestEmergencyRescueSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	rescueRepo := &stubRescueRepo{}
	cleaner := &stubActionCleaner{}
	timerSvc := timer.NewService(nil)
	svc := newTestService(rescueRepo, heroRepo(), cleaner, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-1", " Screen frozen ", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}
	if rec.CharacterID != "char-1" || rec.Reason != "Screen frozen" || rec.PenaltySeconds != DefaultPenaltySeconds {
		t.Fatalf("unexpected record %+v", rec)
	}
	if len(cleaner.clearedCharacters) != 1 || cleaner.clearedCharacters[0] != "char-1" {
		t.Fatalf("expected action cleaner called for char-1, got %v", cleaner.clearedCharacters)
	}
	assertSleep(t, timerSvc, 600*time.Second)
	if len(rescueRepo.records) != 1 || rescueRepo.records[0].AppliedAt.IsZero() {
		t.Fatalf("expected one confirmed record, got %+v", rescueRepo.records)
	}

	underPenalty, remaining, err := svc.IsUnderPenalty(ctx, "char-1", now.Add(time.Minute))
	if err != nil || !underPenalty || remaining != 9*time.Minute {
		t.Fatalf("after 1 min: under=%v remaining=%v err=%v", underPenalty, remaining, err)
	}
	if underPenalty, _, err := svc.IsUnderPenalty(ctx, "char-1", now.Add(11*time.Minute)); err != nil || underPenalty {
		t.Fatalf("after 11 min: under=%v err=%v", underPenalty, err)
	}
}

func TestEmergencyRescue_ConsecutiveRescuesDoNotDoublePenalty(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	rescueRepo := &stubRescueRepo{records: []RescueRecord{{
		ID: "rec-0", CharacterID: "char-1", Reason: "First rescue", PenaltySeconds: DefaultPenaltySeconds,
		CreatedAt: now.Add(-2 * time.Hour), SleepUntil: now.Add(-110 * time.Minute), AppliedAt: now.Add(-2 * time.Hour),
	}}}
	timerSvc := timer.NewService(nil)
	svc := newTestService(rescueRepo, heroRepo(), &stubActionCleaner{}, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "Second stuck", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}
	if rec.ID == "rec-0" || rec.PenaltySeconds != DefaultPenaltySeconds {
		t.Fatalf("expected a new flat penalty, got %+v", rec)
	}
	assertSleep(t, timerSvc, 600*time.Second)

	// The work is gone now: a repeated request is idle and adds nothing.
	again, err := svc.EmergencyRescue(ctx, "char-1", "Third click", now)
	if err != nil || again.ID != "" || again.PenaltySeconds != 0 || len(rescueRepo.records) != 2 {
		t.Fatalf("repeat rescue = %+v, %v (records %d)", again, err, len(rescueRepo.records))
	}
	assertSleep(t, timerSvc, 600*time.Second)
}

func TestEmergencyRescue_AccumulatesSleepTimerWhenAlreadyLocked(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	timerSvc := timer.NewService(nil)
	if err := timerSvc.SetLock(ctx, timer.CategorySleep, "char-1", 200*time.Second); err != nil {
		t.Fatal(err)
	}
	svc := newTestService(&stubRescueRepo{}, heroRepo(), &stubActionCleaner{}, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "Rescue during active sleep", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}
	assertSleep(t, timerSvc, 800*time.Second)
	if owed := rec.SleepUntil.Sub(now); owed < 790*time.Second || owed > 800*time.Second {
		t.Fatalf("recorded deadline = now+%v, want ~800s", owed)
	}
}

func TestEmergencyRescue_IdleCharacterReturnsEarlyWithZeroPenalty(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	rescueRepo := &stubRescueRepo{}
	cleaner := &stubActionCleaner{stuckCharacters: map[string]bool{}}
	timerSvc := &faultyTimer{Service: timer.NewService(nil), getErr: errors.New("idle rescue must not read sleep")}
	svc := newTestService(rescueRepo, heroRepo(), cleaner, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "Accidental click in town", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}
	if rec.ID != "" || rec.PenaltySeconds != 0 || rec.CharacterID != "char-1" {
		t.Fatalf("expected idle result, got %+v", rec)
	}
	if len(rescueRepo.records) != 0 || len(cleaner.clearedCharacters) != 0 {
		t.Fatalf("idle rescue changed state: records=%v cleared=%v", rescueRepo.records, cleaner.clearedCharacters)
	}
	if locked, err := timerSvc.IsLocked(ctx, timer.CategorySleep, "char-1"); err != nil || locked {
		t.Fatalf("expected no sleep for idle character, got locked=%v err=%v", locked, err)
	}
	if underPenalty, remaining, err := svc.IsUnderPenalty(ctx, "char-1", now); err != nil || underPenalty || remaining != 0 {
		t.Fatalf("expected no penalty, got %v %v %v", underPenalty, remaining, err)
	}
}

// Every failure point keeps existing sleep and leaves either no change or an
// unconfirmed record; the retry resumes it with exactly one penalty.
func TestEmergencyRescue_PartialFailuresResumeWithOnePenalty(t *testing.T) {
	ctx := context.Background()
	injected := errors.New("injected store failure")
	for _, tc := range []struct {
		name          string
		inject        func(*stubRescueRepo, *stubActionCleaner, *faultyTimer)
		wantRecord    bool
		wantCleared   bool
		wantFullSleep bool // the penalty already reached the timer before the failure
	}{
		{"remaining sleep read", func(_ *stubRescueRepo, _ *stubActionCleaner, tm *faultyTimer) { tm.getErr = injected }, false, false, false},
		{"record save", func(r *stubRescueRepo, _ *stubActionCleaner, _ *faultyTimer) { r.saveErr = injected }, false, false, false},
		{"scheduled work clear", func(_ *stubRescueRepo, c *stubActionCleaner, _ *faultyTimer) { c.err = injected }, true, false, false},
		{"sleep timer write", func(_ *stubRescueRepo, _ *stubActionCleaner, tm *faultyTimer) { tm.setErr = injected }, true, true, false},
		{"confirmation", func(r *stubRescueRepo, _ *stubActionCleaner, _ *faultyTimer) { r.markErr = injected }, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			repo := &stubRescueRepo{}
			cleaner := &stubActionCleaner{}
			base := timer.NewService(nil)
			if err := base.SetLock(ctx, timer.CategorySleep, "char-1", time.Hour); err != nil {
				t.Fatal(err)
			}
			tm := &faultyTimer{Service: base}
			svc := newTestService(repo, heroRepo(), cleaner, tm)
			tc.inject(repo, cleaner, tm)

			rec, err := svc.EmergencyRescue(ctx, "char-1", "stuck", now)
			if !errors.Is(err, injected) || rec.ID != "" || rec.PenaltySeconds != 0 {
				t.Fatalf("failure surfaced as %+v, %v", rec, err)
			}
			if tc.wantFullSleep {
				assertSleep(t, base, time.Hour+600*time.Second)
			} else {
				assertSleep(t, base, time.Hour)
			}
			if got := len(repo.records) == 1 && repo.records[0].AppliedAt.IsZero(); got != tc.wantRecord || (!tc.wantRecord && len(repo.records) != 0) {
				t.Fatalf("records after failure: %+v", repo.records)
			}
			if cleared := !cleaner.hasWork("char-1"); cleared != tc.wantCleared {
				t.Fatalf("work cleared = %v, want %v", cleared, tc.wantCleared)
			}
			if under, _, err := svc.IsUnderPenalty(ctx, "char-1", now); err != nil || under != tc.wantRecord {
				t.Fatalf("owed penalty visible = %v (err %v), want %v", under, err, tc.wantRecord)
			}

			*repo = stubRescueRepo{records: repo.records}
			cleaner.err, tm.getErr, tm.setErr = nil, nil, nil
			rec, err = svc.EmergencyRescue(ctx, "char-1", "retry", now.Add(time.Second))
			if err != nil || rec.PenaltySeconds != DefaultPenaltySeconds {
				t.Fatalf("retry = %+v, %v", rec, err)
			}
			if len(repo.records) != 1 || repo.records[0].AppliedAt.IsZero() || repo.records[0].ID != rec.ID {
				t.Fatalf("retry must confirm exactly one record: %+v", repo.records)
			}
			if tc.wantRecord && rec.Reason != "stuck" {
				t.Fatalf("retry did not resume the unconfirmed record: %+v", rec)
			}
			assertSleep(t, base, time.Hour+600*time.Second)
		})
	}
}

// A resumed record whose deadline is shorter than the current sleep is confirmed without shortening it.
func TestEmergencyRescue_ResumeNeverShortensSleep(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := &stubRescueRepo{records: []RescueRecord{{
		ID: "rec-0", CharacterID: "char-1", Reason: "old", PenaltySeconds: DefaultPenaltySeconds,
		CreatedAt: now.Add(-5 * time.Minute), SleepUntil: now.Add(5 * time.Minute),
	}}}
	timerSvc := timer.NewService(nil)
	if err := timerSvc.SetLock(ctx, timer.CategorySleep, "char-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	svc := newTestService(repo, heroRepo(), &stubActionCleaner{stuckCharacters: map[string]bool{}}, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "retry", now)
	if err != nil || rec.ID != "rec-0" || repo.records[0].AppliedAt.IsZero() {
		t.Fatalf("resume = %+v, %v (%+v)", rec, err, repo.records)
	}
	assertSleep(t, timerSvc, time.Hour)
}

func TestService_Unstuck(t *testing.T) {
	ctx := context.Background()

	rescueRepo := &stubRescueRepo{}
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-unstuck": {ID: "char-unstuck", Name: "AdminTarget"},
		},
	}
	cleaner := &stubActionCleaner{}
	timerSvc := timer.NewService(nil)
	svc := newTestService(rescueRepo, charRepo, cleaner, timerSvc)

	t.Run("success", func(t *testing.T) {
		err := svc.Unstuck(ctx, "char-unstuck")
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}

		if len(cleaner.clearedCharacters) != 1 || cleaner.clearedCharacters[0] != "char-unstuck" {
			t.Fatalf("expected cleaner invoked with char-unstuck, got %v", cleaner.clearedCharacters)
		}

		if len(rescueRepo.records) != 0 {
			t.Errorf("expected 0 saved rescue records for Unstuck, got %d", len(rescueRepo.records))
		}
	})

	t.Run("empty_id", func(t *testing.T) {
		err := svc.Unstuck(ctx, "   ")
		if !errors.Is(err, ErrInvalidCharacterID) {
			t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		err := svc.Unstuck(ctx, "char-non-existent")
		if !errors.Is(err, corecharacter.ErrNotFound) {
			t.Fatalf("expected corecharacter.ErrNotFound, got %v", err)
		}
	})
}

type stubActivityReader struct {
	kinds         []string
	err           error
	owner, target string
}

func (r *stubActivityReader) ActivityKinds(_ context.Context, ownerID, characterID string) ([]string, error) {
	r.owner, r.target = ownerID, characterID
	return r.kinds, r.err
}

func TestRescueRejectsRemainingFeatureActivity(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	readFailure := errors.New("activity store down")
	for _, tc := range []struct {
		name    string
		reader  *stubActivityReader
		wantErr error
	}{
		{"activity without scheduled work", &stubActivityReader{kinds: []string{"dungeon", "pvp"}}, ErrActivityRemaining},
		{"observation failure", &stubActivityReader{err: readFailure}, readFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rescueRepo := &stubRescueRepo{}
			charRepo := &stubCharRepo{characters: map[string]corecharacter.Character{"char-1": {ID: "char-1", PlayerID: "player-1"}}}
			cleaner := &stubActionCleaner{stuckCharacters: map[string]bool{}}
			timerSvc := timer.NewService(nil)
			if err := timerSvc.SetLock(ctx, timer.CategorySleep, "char-1", time.Hour); err != nil {
				t.Fatal(err)
			}
			svc := newTestService(rescueRepo, charRepo, cleaner, timerSvc)
			svc.SetActivityReader(tc.reader)

			if _, err := svc.EmergencyRescue(ctx, "char-1", "stuck", now); !errors.Is(err, tc.wantErr) {
				t.Fatalf("rescue error = %v, want %v", err, tc.wantErr)
			}
			if err := svc.Unstuck(ctx, "char-1"); !errors.Is(err, tc.wantErr) {
				t.Fatalf("unstuck error = %v, want %v", err, tc.wantErr)
			}
			if tc.reader.owner != "player-1" || tc.reader.target != "char-1" {
				t.Fatalf("activity read for %q/%q", tc.reader.owner, tc.reader.target)
			}
			if len(cleaner.clearedCharacters) != 0 || len(rescueRepo.records) != 0 {
				t.Fatalf("unrecovered rescue changed state: cleared=%v records=%v", cleaner.clearedCharacters, rescueRepo.records)
			}
			if rem, err := timerSvc.GetRemainingLock(ctx, timer.CategorySleep, "char-1"); err != nil || rem <= 59*time.Minute || rem > time.Hour {
				t.Fatalf("sleep changed: %s %v", rem, err)
			}
		})
	}

	t.Run("no activity keeps idle rescue free", func(t *testing.T) {
		charRepo := &stubCharRepo{characters: map[string]corecharacter.Character{"char-1": {ID: "char-1"}}}
		svc := newTestService(&stubRescueRepo{}, charRepo, &stubActionCleaner{stuckCharacters: map[string]bool{}}, timer.NewService(nil))
		svc.SetActivityReader(&stubActivityReader{kinds: []string{}})
		rec, err := svc.EmergencyRescue(ctx, "char-1", "stuck", now)
		if err != nil || rec.PenaltySeconds != 0 {
			t.Fatalf("idle rescue = %+v, %v", rec, err)
		}
	})
}

// After work was cleared but sleep failed, a newly started activity must not hide the owed sleep.
func TestEmergencyRescue_ResumeAppliesOwedSleepDuringNewActivity(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := &stubRescueRepo{}
	cleaner := &stubActionCleaner{}
	base := timer.NewService(nil)
	tm := &faultyTimer{Service: base, setErr: errors.New("sleep write failed")}
	svc := newTestService(repo, heroRepo(), cleaner, tm)

	if _, err := svc.EmergencyRescue(ctx, "char-1", "stuck", now); err == nil {
		t.Fatal("expected the sleep write failure")
	}
	tm.setErr = nil
	activity := &stubActivityReader{kinds: []string{"dungeon"}}
	svc.SetActivityReader(activity)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "retry", now.Add(time.Second))
	if err != nil || rec.Reason != "stuck" || repo.records[0].AppliedAt.IsZero() {
		t.Fatalf("resume during activity = %+v, %v (%+v)", rec, err, repo.records)
	}
	assertSleep(t, base, 599*time.Second)
	if len(cleaner.clearedCharacters) != 1 {
		t.Fatalf("resume during activity must not clear work again: %v", cleaner.clearedCharacters)
	}

	// Uncleared work with an activity is still rejected without change.
	repo.records[0].AppliedAt = time.Time{}
	cleaner.stuckCharacters["char-1"] = true
	if _, err := svc.EmergencyRescue(ctx, "char-1", "again", now); !errors.Is(err, ErrActivityRemaining) {
		t.Fatalf("uncleared work during activity = %v, want ErrActivityRemaining", err)
	}
	if len(cleaner.clearedCharacters) != 1 || !repo.records[0].AppliedAt.IsZero() {
		t.Fatalf("rejected resume changed state: cleared=%v records=%+v", cleaner.clearedCharacters, repo.records)
	}
}
