package rescue

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type stubRescueRepo struct {
	records []RescueRecord
}

func (r *stubRescueRepo) Save(_ context.Context, record RescueRecord) error {
	r.records = append([]RescueRecord{record}, r.records...)
	return nil
}

func (r *stubRescueRepo) FindRecentByCharacterID(_ context.Context, characterID string, since time.Time) ([]RescueRecord, error) {
	var results []RescueRecord
	for _, rec := range r.records {
		if rec.CharacterID == characterID && rec.CreatedAt.After(since) {
			results = append(results, rec)
		}
	}
	return results, nil
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
}

func (c *stubActionCleaner) ClearActiveActions(_ context.Context, characterID string) error {
	c.clearedCharacters = append(c.clearedCharacters, characterID)
	return nil
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

	svc := NewService(rescueRepo, charRepo, cleaner)

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

func TestEmergencyRescueConsecutivePenaltyMultiplier(t *testing.T) {
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

	svc := NewService(rescueRepo, charRepo, nil)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "Second stuck", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}

	if rec.PenaltySeconds != DefaultPenaltySeconds*2 {
		t.Errorf("expected 2x penalty %d, got %d", DefaultPenaltySeconds*2, rec.PenaltySeconds)
	}
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
	svc := NewService(rescueRepo, charRepo, cleaner)

	_, err := svc.EmergencyRescue(ctx, "char-42", "Stuck in infinite task", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}

	if len(cleaner.clearedCharacters) != 1 || cleaner.clearedCharacters[0] != "char-42" {
		t.Fatalf("expected cleaner invoked with char-42, got %v", cleaner.clearedCharacters)
	}
}
