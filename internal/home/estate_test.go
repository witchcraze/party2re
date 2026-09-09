package home

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
)

type mockGuildPoints struct {
	points map[string]int
}

func (m *mockGuildPoints) AddGuildPoints(ctx context.Context, characterID string, points int) error {
	if m.points == nil {
		m.points = make(map[string]int)
	}
	m.points[characterID] += points
	return nil
}

type mockTimer struct {
	locks map[string]time.Duration
}

func (m *mockTimer) SetLock(ctx context.Context, category, targetID string, duration time.Duration) error {
	if m.locks == nil {
		m.locks = make(map[string]time.Duration)
	}
	m.locks[category+":"+targetID] = duration
	return nil
}

func (m *mockTimer) IsLocked(ctx context.Context, category, targetID string) (bool, error) {
	_, ok := m.locks[category+":"+targetID]
	return ok, nil
}

func (m *mockTimer) GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error) {
	d, ok := m.locks[category+":"+targetID]
	if !ok {
		return 0, nil
	}
	return d, nil
}

func (m *mockTimer) ReleaseLock(ctx context.Context, category, targetID string) error {
	delete(m.locks, category+":"+targetID)
	return nil
}

func (m *mockTimer) ResetDailyQuota(ctx context.Context, action, targetID string) error {
	return nil
}

func TestEstateService(t *testing.T) {
	ctx := context.Background()
	chars := map[string]corecharacter.Character{
		"char-1": {ID: "char-1", PlayerID: "p1", Name: "Hero", Money: 10000},
		"char-2": {ID: "char-2", PlayerID: "p2", Name: "Mage", Money: 100},
	}
	charReader := &mockCharReader{chars: chars}
	charUpdater := &mockCharUpdater{chars: chars}
	repo := newMockHomeRepo(chars)
	guildPts := &mockGuildPoints{points: make(map[string]int)}
	tmr := &mockTimer{locks: make(map[string]time.Duration)}

	fixedTime := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithGuildPoints(guildPts),
		WithTimer(tmr),
		WithNowFunc(func() time.Time { return fixedTime }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Run("build house successfully in town1", func(t *testing.T) {
		res, err := svc.BuildHouse(ctx, "char-1", "town1", "001")
		if err != nil {
			t.Fatalf("BuildHouse failed: %v", err)
		}
		if res.TownID != "town1" || res.HouseStyle != "001" {
			t.Errorf("unexpected build result: %+v", res)
		}
		// town1 costs 500G
		if chars["char-1"].Money != 9500 {
			t.Errorf("expected 9500 money, got %d", chars["char-1"].Money)
		}
		// guild points = cycle_days (5) * 10 = 50
		if guildPts.points["char-1"] != 50 {
			t.Errorf("expected 50 guild points, got %d", guildPts.points["char-1"])
		}
		// valkey timer set for 5 days
		if tmr.locks[timer.CategoryHouse+":char-1"] != 5*24*time.Hour {
			t.Errorf("expected 5d lock in timer, got %v", tmr.locks[timer.CategoryHouse+":char-1"])
		}
	})

	t.Run("cannot build second house while owning active house", func(t *testing.T) {
		_, err := svc.BuildHouse(ctx, "char-1", "town2", "005")
		if !errors.Is(err, ErrAlreadyOwnsHouse) {
			t.Errorf("expected ErrAlreadyOwnsHouse, got %v", err)
		}
	})

	t.Run("check house by ID and by name", func(t *testing.T) {
		resID, err := svc.CheckHouse(ctx, "char-1")
		if err != nil {
			t.Fatalf("CheckHouse by ID failed: %v", err)
		}
		if resID.OwnerName != "Hero" {
			t.Errorf("expected Hero, got %s", resID.OwnerName)
		}

		resName, err := svc.CheckHouse(ctx, "Hero")
		if err != nil {
			t.Fatalf("CheckHouse by Name failed: %v", err)
		}
		if resName.CharacterID != "char-1" {
			t.Errorf("expected char-1, got %s", resName.CharacterID)
		}
	})

	t.Run("town max houses limit", func(t *testing.T) {
		// Populate town1 with 10 houses
		for i := 10; i < 20; i++ {
			cid := string(rune('a' + i))
			exp := fixedTime.Add(24 * time.Hour)
			repo.homes[cid] = CharacterHome{
				CharacterID: cid,
				TownID:      "town1",
				HouseStyle:  "002",
				ExpiresAt:   &exp,
			}
		}
		// char-2 has money
		chars["char-2"] = corecharacter.Character{ID: "char-2", PlayerID: "p2", Name: "Mage", Money: 10000}
		_, err := svc.BuildHouse(ctx, "char-2", "town1", "002")
		if !errors.Is(err, ErrTownMaxHousesReached) {
			t.Errorf("expected ErrTownMaxHousesReached, got %v", err)
		}
	})
}
