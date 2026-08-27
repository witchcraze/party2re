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
	r.records = append(r.records, record)
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
	cleaner := &stubActionCleaner{}

	svc := NewService(rescueRepo, charRepo, cleaner)

	rec, err := svc.EmergencyRescue(ctx, "char-1", "Second rescue", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}

	expectedPenalty := DefaultPenaltySeconds * 2
	if rec.PenaltySeconds != expectedPenalty {
		t.Errorf("expected penalty %d, got %d", expectedPenalty, rec.PenaltySeconds)
	}
}

func TestEmergencyRescueRejectsInvalidCharacter(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	svc := NewService(&stubRescueRepo{}, &stubCharRepo{characters: make(map[string]corecharacter.Character)}, &stubActionCleaner{})
	_, err := svc.EmergencyRescue(ctx, "nonexistent", "help", now)
	if !errors.Is(err, corecharacter.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
