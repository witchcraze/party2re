package challenge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockCharRepo struct {
	chars       map[string]corecharacter.Character
	findByIDErr error
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if m.findByIDErr != nil {
		return corecharacter.Character{}, m.findByIDErr
	}
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, challenge.ErrCharacterNotFound
	}
	return c, nil
}

type mockChallengeRepo struct {
	sessions             map[string]challenge.ChallengeSession
	records              map[string]challenge.CharacterChallengeRecord
	hof                  map[string]challenge.HallOfFameEntry
	saveSessionErr       error
	findSessionByIDErr   error
	findActiveErr        error
	updateSessionErr     error
	saveRecordErr        error
	findRecordErr        error
	findRecordsByCharErr error
	getLeaderboardErr    error
	finalizeSessionErr   error
	saveHofErr           error
	getHofErr            error
	listHofErr           error
}

func newMockChallengeRepo() *mockChallengeRepo {
	return &mockChallengeRepo{
		sessions: make(map[string]challenge.ChallengeSession),
		records:  make(map[string]challenge.CharacterChallengeRecord),
		hof:      make(map[string]challenge.HallOfFameEntry),
	}
}

func (m *mockChallengeRepo) SaveSession(ctx context.Context, s challenge.ChallengeSession) error {
	if m.saveSessionErr != nil {
		return m.saveSessionErr
	}
	m.sessions[s.ID] = s
	return nil
}

func (m *mockChallengeRepo) FindSessionByID(ctx context.Context, id string) (*challenge.ChallengeSession, error) {
	if m.findSessionByIDErr != nil {
		return nil, m.findSessionByIDErr
	}
	s, ok := m.sessions[id]
	if !ok {
		return nil, challenge.ErrSessionNotFound
	}
	return &s, nil
}

func (m *mockChallengeRepo) FindActiveSessionByCharacter(ctx context.Context, characterID string) (*challenge.ChallengeSession, error) {
	if m.findActiveErr != nil {
		return nil, m.findActiveErr
	}
	for _, s := range m.sessions {
		if s.CharacterID == characterID && s.Status == challenge.StatusActive {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *mockChallengeRepo) UpdateSession(ctx context.Context, s challenge.ChallengeSession) error {
	if m.updateSessionErr != nil {
		return m.updateSessionErr
	}
	m.sessions[s.ID] = s
	return nil
}

func (m *mockChallengeRepo) SaveRecord(ctx context.Context, r challenge.CharacterChallengeRecord) error {
	if m.saveRecordErr != nil {
		return m.saveRecordErr
	}
	key := r.CharacterID + ":" + r.TierID
	m.records[key] = r
	return nil
}

func (m *mockChallengeRepo) FindRecord(ctx context.Context, characterID string, tierID string) (*challenge.CharacterChallengeRecord, error) {
	if m.findRecordErr != nil {
		return nil, m.findRecordErr
	}
	key := characterID + ":" + tierID
	r, ok := m.records[key]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockChallengeRepo) FindRecordsByCharacter(ctx context.Context, characterID string) ([]challenge.CharacterChallengeRecord, error) {
	if m.findRecordsByCharErr != nil {
		return nil, m.findRecordsByCharErr
	}
	var list []challenge.CharacterChallengeRecord
	for _, r := range m.records {
		if r.CharacterID == characterID {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockChallengeRepo) GetLeaderboard(ctx context.Context, tierID string, limit int) ([]challenge.LeaderboardEntry, error) {
	if m.getLeaderboardErr != nil {
		return nil, m.getLeaderboardErr
	}
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
	if m.finalizeSessionErr != nil {
		return m.finalizeSessionErr
	}
	m.sessions[s.ID] = s
	key := s.CharacterID + ":" + s.TierID
	rec := m.records[key]
	rec.CharacterID = s.CharacterID
	rec.TierID = s.TierID
	rec.TotalAttempts++
	rec.TotalVictories += newStreak
	if newStreak > rec.HighestRound {
		rec.HighestRound = newStreak
		rec.BestClearedAt = s.UpdatedAt
	}
	m.records[key] = rec
	return nil
}

func (m *mockChallengeRepo) SaveHallOfFame(ctx context.Context, entry challenge.HallOfFameEntry) error {
	if m.saveHofErr != nil {
		return m.saveHofErr
	}
	m.hof[entry.TierID] = entry
	return nil
}

func (m *mockChallengeRepo) GetHallOfFame(ctx context.Context, tierID string) (*challenge.HallOfFameEntry, error) {
	if m.getHofErr != nil {
		return nil, m.getHofErr
	}
	entry, ok := m.hof[tierID]
	if !ok {
		return nil, nil
	}
	return &entry, nil
}

func (m *mockChallengeRepo) ListHallOfFame(ctx context.Context) ([]challenge.HallOfFameEntry, error) {
	if m.listHofErr != nil {
		return nil, m.listHofErr
	}
	var list []challenge.HallOfFameEntry
	for _, e := range m.hof {
		list = append(list, e)
	}
	return list, nil
}

type mockActiveStore struct {
	underlying             challenge.ActiveSessionStore
	getActiveSessionErr    error
	saveActiveSessionErr   error
	deleteActiveSessionErr error
	advanceRoundErr        error
}

func newMockActiveStore(underlying challenge.ActiveSessionStore) *mockActiveStore {
	if underlying == nil {
		underlying = challenge.NewMemorySessionRepository()
	}
	return &mockActiveStore{underlying: underlying}
}

func (m *mockActiveStore) GetActiveSession(ctx context.Context, characterID string) (*challenge.ChallengeSession, error) {
	if m.getActiveSessionErr != nil {
		return nil, m.getActiveSessionErr
	}
	return m.underlying.GetActiveSession(ctx, characterID)
}

func (m *mockActiveStore) SaveActiveSession(ctx context.Context, session challenge.ChallengeSession) error {
	if m.saveActiveSessionErr != nil {
		return m.saveActiveSessionErr
	}
	return m.underlying.SaveActiveSession(ctx, session)
}

func (m *mockActiveStore) DeleteActiveSession(ctx context.Context, characterID string) error {
	if m.deleteActiveSessionErr != nil {
		return m.deleteActiveSessionErr
	}
	return m.underlying.DeleteActiveSession(ctx, characterID)
}

func (m *mockActiveStore) AdvanceRound(ctx context.Context, characterID string, params challenge.AdvanceRoundParams) (challenge.AdvanceRoundOutcome, error) {
	if m.advanceRoundErr != nil {
		return challenge.AdvanceRoundOutcome{}, m.advanceRoundErr
	}
	return m.underlying.AdvanceRound(ctx, characterID, params)
}

func TestStartSession_ValidationAndCreation(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"high_hp_char": {
				ID:         "high_hp_char",
				Level:      1,
				Experience: 10,
				Stats:      corecharacter.Stats{HP: 500, MaxHP: 500, Attack: 20, Defense: 10},
			},
			"valid_char": {
				ID:         "valid_char",
				Level:      15,
				Experience: 2500,
				Stats:      corecharacter.Stats{HP: 200, MaxHP: 200, Attack: 40, Defense: 20},
			},
		},
	}
	repo := newMockChallengeRepo()
	service, err := challenge.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	// 1. NeedJoin not met (stage 0 requires MaxHP < 400)
	_, err = service.StartSession(ctx, "high_hp_char", "0")
	if !errors.Is(err, challenge.ErrNeedJoinNotMet) {
		t.Errorf("expected ErrNeedJoinNotMet, got %v", err)
	}

	// 2. Success Start
	session, err := service.StartSession(ctx, "valid_char", "0")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	if session.CurrentRound != 1 || session.CharacterCurrentHP != 200 || session.Status != challenge.StatusActive {
		t.Errorf("unexpected session: %#v", session)
	}

	// 3. Active session already exists
	_, err = service.StartSession(ctx, "valid_char", "0")
	if !errors.Is(err, challenge.ErrActiveSessionExists) {
		t.Errorf("expected ErrActiveSessionExists, got %v", err)
	}
}

func TestExecuteRound_VictoryProgression(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": {
				ID:         "hero",
				Level:      31,
				Experience: 10000,
				Stats:      corecharacter.Stats{HP: 350, MaxHP: 350, Attack: 150, Defense: 100},
			},
		},
	}
	repo := newMockChallengeRepo()
	service, err := challenge.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	session, err := service.StartSession(ctx, "hero", "0")
	if err != nil {
		t.Fatal(err)
	}

	// Execute 3 rounds
	for round := 1; round <= 3; round++ {
		res, err := service.ExecuteRound(ctx, session.ID)
		if err != nil {
			t.Fatalf("ExecuteRound %d failed: %v", round, err)
		}
		if !res.Won {
			t.Fatalf("round %d expected victory", round)
		}
	}

	active, err := service.GetActiveSession(ctx, "hero")
	if err != nil || active == nil {
		t.Fatalf("expected active session, got %v", err)
	}
	if active.CurrentRound != 4 || active.AccumulatedExp <= 0 {
		t.Errorf("unexpected session state after 3 rounds: %#v", active)
	}
}

func TestExecuteRound_DefeatTerminatesSession(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"weak_hero": {
				ID:         "weak_hero",
				Level:      1,
				Experience: 0,
				Stats:      corecharacter.Stats{HP: 10, MaxHP: 10, Attack: 5, Defense: 0},
			},
		},
	}
	repo := newMockChallengeRepo()
	service, err := challenge.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	session, err := service.StartSession(ctx, "weak_hero", "0")
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
				Stats:      corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 180, Defense: 100},
			},
			"attacker_char": {
				ID:         "attacker_char",
				Level:      20,
				Experience: 5000,
				Stats:      corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 180, Defense: 100},
			},
		},
	}
	repo := newMockChallengeRepo()
	service, err := challenge.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	session, err := service.StartSession(ctx, "owner_char", "0")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// 1. AdvanceRound from attacker_char should return ErrForbidden
	_, _, err = service.AdvanceRound(ctx, "attacker_char", session.ID)
	if !errors.Is(err, challenge.ErrForbidden) {
		t.Errorf("expected ErrForbidden for AdvanceRound by non-owner, got %v", err)
	}

	// 2. GetSession from attacker_char should return ErrForbidden
	_, err = service.GetSession(ctx, "attacker_char", session.ID)
	if !errors.Is(err, challenge.ErrForbidden) {
		t.Errorf("expected ErrForbidden for GetSession by non-owner, got %v", err)
	}

	// 3. Owner should succeed
	roundRes, updatedSession, err := service.AdvanceRound(ctx, "owner_char", session.ID)
	if err != nil {
		t.Fatalf("AdvanceRound by owner failed: %v", err)
	}
	if !roundRes.Won || updatedSession.CurrentRound != 2 {
		t.Errorf("unexpected round result: %#v", roundRes)
	}
}

func TestMockChallengeRepository_SaveRecord(t *testing.T) {
	ctx := context.Background()
	repo := newMockChallengeRepo()

	r := challenge.CharacterChallengeRecord{
		CharacterID:    "char-1",
		TierID:         "0",
		HighestRound:   4,
		TotalAttempts:  1,
		TotalVictories: 4,
	}
	if err := repo.SaveRecord(ctx, r); err != nil {
		t.Fatalf("SaveRecord failed: %v", err)
	}

	found, err := repo.FindRecord(ctx, "char-1", "0")
	if err != nil || found == nil {
		t.Fatalf("FindRecord failed: %v", err)
	}
	if found.HighestRound != 4 {
		t.Errorf("expected HighestRound 4, got %d", found.HighestRound)
	}
}
