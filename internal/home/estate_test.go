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

func TestBuildHouse_Transactional(t *testing.T) {
	ctx := context.Background()
	fixedTime := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("successful house construction via runner", func(t *testing.T) {
		chars := map[string]corecharacter.Character{
			"tx-char-1": {ID: "tx-char-1", PlayerID: "p1", Name: "Builder", Money: 10000},
		}
		repo := newMockHomeRepo(chars)
		gp := &mockGuildPoints{}
		tmr := &mockTimer{}
		runner := &mockTransactionRunner{chars: chars}

		svc, err := NewService(
			repo,
			&mockCharReader{chars: chars},
			WithNowFunc(func() time.Time { return fixedTime }),
			WithGuildPoints(gp),
			WithTimer(tmr),
			WithTransactionRunner(runner),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		res, err := svc.BuildHouse(ctx, "tx-char-1", "town1", "001")
		if err != nil {
			t.Fatalf("BuildHouse failed: %v", err)
		}
		if res.OwnerName != "Builder" {
			t.Errorf("expected OwnerName Builder, got %s", res.OwnerName)
		}
		// town1 price is 500
		if chars["tx-char-1"].Money != 9500 {
			t.Errorf("expected 9500 gold remaining, got %d", chars["tx-char-1"].Money)
		}
		if runner.calls != 1 {
			t.Errorf("expected 1 runner call, got %d", runner.calls)
		}
		if runner.lastReq.Cost.Gold != 500 {
			t.Errorf("expected 500 gold cost, got %d", runner.lastReq.Cost.Gold)
		}
		// guild points added
		if gp.points["tx-char-1"] != 50 {
			t.Errorf("expected 50 guild points, got %d", gp.points["tx-char-1"])
		}
		// timer set
		if tmr.locks[timer.CategoryHouse+":tx-char-1"] != 5*24*time.Hour {
			t.Errorf("expected 5d lock in timer, got %v", tmr.locks[timer.CategoryHouse+":tx-char-1"])
		}
	})

	t.Run("insufficient funds aborts before save", func(t *testing.T) {
		chars := map[string]corecharacter.Character{
			"tx-poor": {ID: "tx-poor", PlayerID: "p1", Name: "Poor", Money: 400},
		}
		repo := newMockHomeRepo(chars)
		runner := &mockTransactionRunner{chars: chars}

		svc, err := NewService(
			repo,
			&mockCharReader{chars: chars},
			WithNowFunc(func() time.Time { return fixedTime }),
			WithTransactionRunner(runner),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		_, err = svc.BuildHouse(ctx, "tx-poor", "town1", "001")
		if !errors.Is(err, ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
		if chars["tx-poor"].Money != 400 {
			t.Errorf("expected money to remain 400, got %d", chars["tx-poor"].Money)
		}
	})

	t.Run("town max houses aborts transaction without gold loss", func(t *testing.T) {
		chars := map[string]corecharacter.Character{
			"tx-char-2": {ID: "tx-char-2", PlayerID: "p2", Name: "Mage", Money: 10000},
		}
		repo := newMockHomeRepo(chars)
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

		runner := &mockTransactionRunner{chars: chars}

		svc, err := NewService(
			repo,
			&mockCharReader{chars: chars},
			WithNowFunc(func() time.Time { return fixedTime }),
			WithTransactionRunner(runner),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		_, err = svc.BuildHouse(ctx, "tx-char-2", "town1", "002")
		if !errors.Is(err, ErrTownMaxHousesReached) {
			t.Errorf("expected ErrTownMaxHousesReached, got %v", err)
		}
		if chars["tx-char-2"].Money != 10000 {
			t.Errorf("expected money to remain 10000, got %d", chars["tx-char-2"].Money)
		}
		if _, exists := repo.homes["tx-char-2"]; exists {
			t.Error("expected house not to be saved in repository")
		}
	})
}
