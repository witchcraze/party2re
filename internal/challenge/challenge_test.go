package challenge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, challenge.ErrCharacterNotFound
	}
	return c, nil
}

type mockChallengeRepo struct {
	sessions map[string]challenge.ChallengeSession
	records  map[string]challenge.CharacterChallengeRecord
}

func newMockChallengeRepo() *mockChallengeRepo {
	return &mockChallengeRepo{
		sessions: make(map[string]challenge.ChallengeSession),
		records:  make(map[string]challenge.CharacterChallengeRecord),
	}
}

func (m *mockChallengeRepo) SaveSession(ctx context.Context, s challenge.ChallengeSession) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockChallengeRepo) FindSessionByID(ctx context.Context, id string) (*challenge.ChallengeSession, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, challenge.ErrSessionNotFound
	}
	return &s, nil
}

func (m *mockChallengeRepo) FindActiveSessionByCharacter(ctx context.Context, characterID string) (*challenge.ChallengeSession, error) {
	for _, s := range m.sessions {
		if s.CharacterID == characterID && s.Status == challenge.StatusActive {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *mockChallengeRepo) UpdateSession(ctx context.Context, s challenge.ChallengeSession) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockChallengeRepo) SaveRecord(ctx context.Context, r challenge.CharacterChallengeRecord) error {
	key := r.CharacterID + ":" + r.TierID
	m.records[key] = r
	return nil
}

func (m *mockChallengeRepo) FindRecord(ctx context.Context, characterID string, tierID string) (*challenge.CharacterChallengeRecord, error) {
	key := characterID + ":" + tierID
	r, ok := m.records[key]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockChallengeRepo) FindRecordsByCharacter(ctx context.Context, characterID string) ([]challenge.CharacterChallengeRecord, error) {
	var list []challenge.CharacterChallengeRecord
	for _, r := range m.records {
		if r.CharacterID == characterID {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockChallengeRepo) GetLeaderboard(ctx context.Context, tierID string, limit int) ([]challenge.LeaderboardEntry, error) {
	var list []challenge.LeaderboardEntry
	for _, r := range m.records {
		if r.TierID == tierID && r.HighestRound > 0 {
			list = append(list, challenge.LeaderboardEntry{
				CharacterID:   r.CharacterID,
				CharacterName: "Test Hero",
				Level:         20,
				JobID:         "warrior",
				HighestRound:  r.HighestRound,
				BestClearedAt: r.BestClearedAt,
			})
		}
	}
	return list, nil
}

func (m *mockChallengeRepo) FinalizeSession(ctx context.Context, s challenge.ChallengeSession, expReward int, goldReward int, items []string, newStreak int) error {
	m.sessions[s.ID] = s
	key := s.CharacterID + ":" + s.TierID
	rec := m.records[key]
	rec.CharacterID = s.CharacterID
	rec.TierID = s.TierID
	rec.TotalAttempts++
	rec.TotalVictories += newStreak
	if newStreak > rec.HighestRound {
		rec.HighestRound = newStreak
		rec.BestClearedAt = time.Now().UTC()
	}
	m.records[key] = rec
	return nil
}

func TestStartSession_ValidationAndCreation(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"low_char": {
				ID:         "low_char",
				Level:      1,
				Experience: 10,
				Stats:      corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 20, Defense: 10},
			},
			"valid_char": {
				ID:         "valid_char",
				Level:      15,
				Experience: 2500,
				Stats:      corecharacter.Stats{HP: 150, MaxHP: 150, Attack: 40, Defense: 20},
			},
		},
	}
	repo := newMockChallengeRepo()
	service, err := challenge.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Level too low for tier
	_, err = service.StartSession(ctx, "low_char", "novice")
	if !errors.Is(err, challenge.ErrLevelTooLow) {
		t.Errorf("expected ErrLevelTooLow, got %v", err)
	}

	// 2. Success Start
	session, err := service.StartSession(ctx, "valid_char", "novice")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	if session.CurrentRound != 1 || session.CharacterCurrentHP != 150 || session.Status != challenge.StatusActive {
		t.Errorf("unexpected session: %#v", session)
	}

	// 3. Active session already exists
	_, err = service.StartSession(ctx, "valid_char", "novice")
	if !errors.Is(err, challenge.ErrActiveSessionExists) {
		t.Errorf("expected ErrActiveSessionExists, got %v", err)
	}
}

func TestExecuteRound_VictoryProgressionAndMilestone(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": {
				ID:         "hero",
				Level:      31,
				Experience: 10000,
				Stats:      corecharacter.Stats{HP: 500, MaxHP: 500, Attack: 150, Defense: 100},
			},
		},
	}
	repo := newMockChallengeRepo()
	service, err := challenge.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	session, err := service.StartSession(ctx, "hero", "novice")
	if err != nil {
		t.Fatal(err)
	}

	// Execute 5 rounds
	for round := 1; round <= 5; round++ {
		res, err := service.ExecuteRound(ctx, session.ID)
		if err != nil {
			t.Fatalf("ExecuteRound %d failed: %v", round, err)
		}
		if !res.Won {
			t.Fatalf("round %d expected victory", round)
		}
		if round == 5 {
			if res.AwardedItem == "" {
				t.Errorf("expected milestone item at round 5")
			}
		}
	}

	active, err := service.GetActiveSession(ctx, "hero")
	if err != nil || active == nil {
		t.Fatalf("expected active session, got %v", err)
	}
	if active.CurrentRound != 6 || active.AccumulatedExp <= 0 || len(active.AccumulatedItems) != 1 {
		t.Errorf("unexpected session state after 5 rounds: %#v", active)
	}

	// Cashout
	cashout, err := service.Cashout(ctx, session.ID)
	if err != nil {
		t.Fatalf("Cashout failed: %v", err)
	}
	if cashout.RoundsCleared != 5 || cashout.AwardedExp != active.AccumulatedExp || len(cashout.AwardedItems) != 1 {
		t.Errorf("unexpected cashout result: %#v", cashout)
	}

	// Record verification
	rec, err := service.GetRecord(ctx, "hero", "novice")
	if err != nil || rec == nil {
		t.Fatalf("GetRecord failed: %v", err)
	}
	if rec.HighestRound != 5 || rec.TotalVictories != 5 || rec.TotalAttempts != 1 {
		t.Errorf("unexpected record: %#v", rec)
	}
}

func TestExecuteRound_DefeatTerminatesSession(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"weak_hero": {
				ID:         "weak_hero",
				Level:      7,
				Experience: 500,
				Stats:      corecharacter.Stats{HP: 10, MaxHP: 10, Attack: 5, Defense: 0},
			},
		},
	}
	repo := newMockChallengeRepo()
	service, err := challenge.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	session, err := service.StartSession(ctx, "weak_hero", "novice")
	if err != nil {
		t.Fatal(err)
	}

	res, err := service.ExecuteRound(ctx, session.ID)
	if err != nil {
		t.Fatalf("ExecuteRound failed: %v", err)
	}
	if res.Won || !res.SessionEnded || res.SessionStatus != challenge.StatusDefeated {
		t.Errorf("unexpected defeat result: %#v", res)
	}

	// Active session should be nil/non-active
	active, _ := service.GetActiveSession(ctx, "weak_hero")
	if active != nil {
		t.Errorf("expected no active session, got %#v", active)
	}
}

func TestChallenge_OwnershipVerification(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"owner_char": {
				ID:         "owner_char",
				Level:      20,
				Experience: 5000,
				Stats:      corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 80, Defense: 50},
			},
			"attacker_char": {
				ID:         "attacker_char",
				Level:      20,
				Experience: 5000,
				Stats:      corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 80, Defense: 50},
			},
		},
	}
	repo := newMockChallengeRepo()
	service, err := challenge.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	session, err := service.StartSession(ctx, "owner_char", "novice")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// 1. AdvanceRound from attacker_char should return ErrForbidden
	_, _, err = service.AdvanceRound(ctx, "attacker_char", session.ID)
	if !errors.Is(err, challenge.ErrForbidden) {
		t.Errorf("expected ErrForbidden for AdvanceRound by non-owner, got %v", err)
	}

	// 2. RetireSession from attacker_char should return ErrForbidden
	_, err = service.RetireSession(ctx, "attacker_char", session.ID)
	if !errors.Is(err, challenge.ErrForbidden) {
		t.Errorf("expected ErrForbidden for RetireSession by non-owner, got %v", err)
	}

	// 3. GetSession from attacker_char should return ErrForbidden
	_, err = service.GetSession(ctx, "attacker_char", session.ID)
	if !errors.Is(err, challenge.ErrForbidden) {
		t.Errorf("expected ErrForbidden for GetSession by non-owner, got %v", err)
	}

	// 4. Owner should succeed
	roundRes, updatedSession, err := service.AdvanceRound(ctx, "owner_char", session.ID)
	if err != nil {
		t.Fatalf("AdvanceRound by owner failed: %v", err)
	}
	if !roundRes.Won || updatedSession.CurrentRound != 2 {
		t.Errorf("unexpected round result: %#v", roundRes)
	}

	retired, err := service.RetireSession(ctx, "owner_char", session.ID)
	if err != nil {
		t.Fatalf("RetireSession by owner failed: %v", err)
	}
	if retired.Status != challenge.StatusClaimed {
		t.Errorf("expected status claimed, got %v", retired.Status)
	}
}
