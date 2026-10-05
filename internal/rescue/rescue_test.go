package rescue

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
)

type stubRescueRepo struct {
	records []RescueRecord
}

func (r *stubRescueRepo) Save(_ context.Context, record RescueRecord) error {
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

type stubCharRepo struct {
	characters map[string]corecharacter.Character
}

func (r *stubCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *stubCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	r.characters[c.ID] = c
	return nil
}

type stubActionCleaner struct {
	clearedCharacters []string
	stuckCharacters   map[string]bool
	err               error
}

func (c *stubActionCleaner) ClearActiveActions(_ context.Context, characterID string) (bool, error) {
	c.clearedCharacters = append(c.clearedCharacters, characterID)
	if c.err != nil {
		return false, c.err
	}
	if c.stuckCharacters != nil {
		return c.stuckCharacters[characterID], nil
	}
	return true, nil
}

func TestEmergencyRescueSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	rescueRepo := &stubRescueRepo{}
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "StuckHero"},
		},
	}
	cleaner := &stubActionCleaner{}
	timerSvc := timer.NewService(nil)

	svc := NewService(rescueRepo, charRepo, cleaner, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "Screen frozen", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}

	if rec.CharacterID != "char-1" {
		t.Errorf("expected char-1, got %s", rec.CharacterID)
	}
	if rec.PenaltySeconds != DefaultPenaltySeconds {
		t.Errorf("expected penalty %d, got %d", DefaultPenaltySeconds, rec.PenaltySeconds)
	}
	if len(cleaner.clearedCharacters) != 1 || cleaner.clearedCharacters[0] != "char-1" {
		t.Errorf("expected action cleaner called for char-1")
	}

	// Verify timer.CategorySleep lock was set for 600s
	locked, err := timerSvc.IsLocked(ctx, timer.CategorySleep, "char-1")
	if err != nil || !locked {
		t.Errorf("expected timer.CategorySleep locked, got locked=%v, err=%v", locked, err)
	}
	lockRem, err := timerSvc.GetRemainingLock(ctx, timer.CategorySleep, "char-1")
	if err != nil || lockRem <= 590*time.Second || lockRem > 600*time.Second {
		t.Errorf("expected ~600s remaining lock, got %v (err: %v)", lockRem, err)
	}

	// Character is under penalty immediately after rescue
	underPenalty, remaining, err := svc.IsUnderPenalty(ctx, "char-1", now.Add(1*time.Minute))
	if err != nil || !underPenalty || remaining <= 0 {
		t.Errorf("expected under penalty after 1 min, got %v (remaining: %v)", underPenalty, remaining)
	}

	err = svc.CheckActionAllowed(ctx, "char-1", now.Add(1*time.Minute))
	if !errors.Is(err, ErrCharacterUnderPenalty) {
		t.Errorf("expected ErrCharacterUnderPenalty, got %v", err)
	}

	// Penalty expires after 10 minutes (600 seconds)
	underPenalty, _, err = svc.IsUnderPenalty(ctx, "char-1", now.Add(11*time.Minute))
	if err != nil || underPenalty {
		t.Errorf("expected penalty expired after 11 min, got under penalty: %v", underPenalty)
	}

	err = svc.CheckActionAllowed(ctx, "char-1", now.Add(11*time.Minute))
	if err != nil {
		t.Errorf("expected action allowed after penalty expiry, got %v", err)
	}
}

func TestEmergencyRescue_ConsecutiveRescuesDoNotDoublePenalty(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	rescueRepo := &stubRescueRepo{
		records: []RescueRecord{
			{
				ID:             "rec-0",
				CharacterID:    "char-1",
				Reason:         "First rescue",
				PenaltySeconds: DefaultPenaltySeconds,
				CreatedAt:      now.Add(-2 * time.Hour),
			},
		},
	}
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "StuckHero"},
		},
	}
	cleaner := &stubActionCleaner{
		stuckCharacters: map[string]bool{"char-1": true},
	}
	timerSvc := timer.NewService(nil)

	svc := NewService(rescueRepo, charRepo, cleaner, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "Second stuck", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}

	if rec.PenaltySeconds != DefaultPenaltySeconds {
		t.Errorf("expected flat penalty %d (no doubling), got %d", DefaultPenaltySeconds, rec.PenaltySeconds)
	}
}

func TestEmergencyRescue_AccumulatesSleepTimerWhenAlreadyLocked(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	rescueRepo := &stubRescueRepo{}
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-1": {ID: "char-1", Name: "StuckHero"},
		},
	}
	cleaner := &stubActionCleaner{
		stuckCharacters: map[string]bool{"char-1": true},
	}
	timerSvc := timer.NewService(nil)

	// Pre-lock with 200 seconds of sleep
	if err := timerSvc.SetLock(ctx, timer.CategorySleep, "char-1", 200*time.Second); err != nil {
		t.Fatalf("failed to pre-lock timer: %v", err)
	}

	svc := NewService(rescueRepo, charRepo, cleaner, timerSvc)

	_, err := svc.EmergencyRescue(ctx, "char-1", "Rescue during active sleep", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}

	rem, err := timerSvc.GetRemainingLock(ctx, timer.CategorySleep, "char-1")
	if err != nil {
		t.Fatalf("GetRemainingLock failed: %v", err)
	}
	// Expected ~800 seconds (200s + 600s)
	if rem < 790*time.Second || rem > 805*time.Second {
		t.Errorf("expected accumulated lock (~800s), got %v", rem)
	}
}

func TestEmergencyRescue_IdleCharacterReturnsEarlyWithZeroPenalty(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	rescueRepo := &stubRescueRepo{}
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-idle": {ID: "char-idle", Name: "TownHero"},
		},
	}
	cleaner := &stubActionCleaner{
		stuckCharacters: map[string]bool{"char-idle": false}, // not stuck
	}
	timerSvc := timer.NewService(nil)

	svc := NewService(rescueRepo, charRepo, cleaner, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-idle", "Accidental click in town", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}

	if rec.PenaltySeconds != 0 {
		t.Errorf("expected 0 penalty seconds for idle character, got %d", rec.PenaltySeconds)
	}
	if rec.CharacterID != "char-idle" {
		t.Errorf("expected char-idle, got %s", rec.CharacterID)
	}

	// Must NOT record a penalty record into the repository
	if len(rescueRepo.records) != 0 {
		t.Errorf("expected 0 saved rescue records, got %d", len(rescueRepo.records))
	}

	// Timer lock must NOT be set for idle character
	locked, err := timerSvc.IsLocked(ctx, timer.CategorySleep, "char-idle")
	if err != nil || locked {
		t.Errorf("expected timer not locked for idle character, got locked=%v, err=%v", locked, err)
	}

	// Character is not under penalty
	underPenalty, remaining, err := svc.IsUnderPenalty(ctx, "char-idle", now)
	if err != nil || underPenalty || remaining != 0 {
		t.Errorf("expected not under penalty, got underPenalty=%v, remaining=%v", underPenalty, remaining)
	}

	// Character can perform actions immediately
	if err := svc.CheckActionAllowed(ctx, "char-idle", now); err != nil {
		t.Errorf("expected action allowed for safe character, got %v", err)
	}
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
	svc := NewService(rescueRepo, charRepo, cleaner, timerSvc)

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

func TestEmergencyRescueInvokesActionCleaner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	rescueRepo := &stubRescueRepo{}
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-42": {ID: "char-42", Name: "StuckHero"},
		},
	}
	cleaner := &stubActionCleaner{}
	timerSvc := timer.NewService(nil)
	svc := NewService(rescueRepo, charRepo, cleaner, timerSvc)

	_, err := svc.EmergencyRescue(ctx, "char-42", "Stuck in infinite task", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}

	if len(cleaner.clearedCharacters) != 1 || cleaner.clearedCharacters[0] != "char-42" {
		t.Fatalf("expected cleaner invoked with char-42, got %v", cleaner.clearedCharacters)
	}
}

func TestEmergencyRescueActionCleanerFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	cleanerErr := errors.New("valkey cleanup failure")
	rescueRepo := &stubRescueRepo{}
	charRepo := &stubCharRepo{
		characters: map[string]corecharacter.Character{
			"char-cleaner-fail": {ID: "char-cleaner-fail", Name: "FailHero"},
		},
	}
	cleaner := &stubActionCleaner{err: cleanerErr}
	timerSvc := timer.NewService(nil)
	svc := NewService(rescueRepo, charRepo, cleaner, timerSvc)

	rec, err := svc.EmergencyRescue(ctx, "char-cleaner-fail", "Screen stuck", now)
	if !errors.Is(err, cleanerErr) {
		t.Fatalf("expected cleanerErr %v, got %v", cleanerErr, err)
	}
	if rec.CharacterID != "" || rec.PenaltySeconds != 0 {
		t.Fatalf("expected empty rescue record, got %+v", rec)
	}

	// Verify no rescue record was saved
	if len(rescueRepo.records) != 0 {
		t.Fatalf("expected 0 rescue records saved after cleaner failure, got %d", len(rescueRepo.records))
	}

	// Verify no timer lock was set
	locked, err := timerSvc.IsLocked(ctx, timer.CategorySleep, "char-cleaner-fail")
	if err != nil || locked {
		t.Fatalf("expected timer not locked, got locked=%v, err=%v", locked, err)
	}
}
