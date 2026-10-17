package challenge_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/party"
)

type mockPartyRepo struct {
	mu            sync.Mutex
	lockErr       error
	lockedParties map[string]bool
	parties       map[string]party.Party
	members       map[string][]party.Member
}

func newMockPartyRepo() *mockPartyRepo {
	return &mockPartyRepo{
		lockedParties: make(map[string]bool),
		parties:       make(map[string]party.Party),
		members:       make(map[string][]party.Member),
	}
}

func (m *mockPartyRepo) WithPartyLock(ctx context.Context, partyID string, fn func(ctx context.Context) error) error {
	m.mu.Lock()
	if m.lockErr != nil {
		err := m.lockErr
		m.mu.Unlock()
		return err
	}
	if m.lockedParties == nil {
		m.lockedParties = make(map[string]bool)
	}
	if m.lockedParties[partyID] {
		m.mu.Unlock()
		return party.ErrPartyNotRecruiting
	}
	m.lockedParties[partyID] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.lockedParties, partyID)
		m.mu.Unlock()
	}()

	return fn(ctx)
}

func (m *mockPartyRepo) GetParty(ctx context.Context, id string) (party.Party, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.parties[id]
	if !ok {
		return party.Party{}, party.ErrNotFound
	}
	return p, nil
}

func (m *mockPartyRepo) GetMembers(ctx context.Context, partyID string) ([]party.Member, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mems, ok := m.members[partyID]
	if !ok {
		return nil, party.ErrNotFound
	}
	return mems, nil
}

func (m *mockPartyRepo) UpdateParty(ctx context.Context, p party.Party) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.parties[p.ID] = p
	return nil
}

func (m *mockPartyRepo) UpdateMemberReady(ctx context.Context, partyID string, characterID string, ready bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mems, ok := m.members[partyID]
	if !ok {
		return party.ErrNotFound
	}
	for i := range mems {
		if mems[i].CharacterID == characterID {
			mems[i].ReadyState = ready
			return nil
		}
	}
	return nil
}

func (m *mockPartyRepo) addParty(p party.Party, members []party.Member) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.parties[p.ID] = p
	m.members[p.ID] = members
}

func createPartyTestChar(id string, level int, hp int, atk int, def int) corecharacter.Character {
	return corecharacter.Character{
		ID:         id,
		Level:      level,
		Experience: 5000,
		Stats: corecharacter.Stats{
			HP:      hp,
			MaxHP:   hp,
			Attack:  atk,
			Defense: def,
			Agility: 50,
		},
	}
}

func TestStartPartyChallengeSession(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	partyRepo := newMockPartyRepo()

	c1 := createPartyTestChar("lead", 15, 200, 50, 30)
	c1.Name = "挑戦勇者"
	c1.JobID = "hero"
	charRepo.chars[c1.ID] = c1

	c2 := createPartyTestChar("c2", 15, 180, 45, 25)
	c2.Name = "挑戦戦士"
	c2.JobID = "warrior"
	charRepo.chars[c2.ID] = c2

	partyRepo.addParty(party.Party{
		ID:                "p1",
		LeaderCharacterID: "lead",
		Name:              "栄光の騎士団",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p1", CharacterID: "lead", CharacterName: "挑戦勇者", ReadyState: true, IsLeader: true},
		{PartyID: "p1", CharacterID: "c2", CharacterName: "挑戦戦士", ReadyState: true},
	})

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithPartyRepository(partyRepo))
	if err != nil {
		t.Fatalf("failed to create challenge service: %v", err)
	}
	ctx := context.Background()

	// 1. Success starting party session on stage "1" (allows up to 2 participants, hp < 400)
	sess, err := svc.StartPartySession(ctx, "lead", "p1", "1")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if len(sess.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(sess.Members))
	}
	if sess.PartyName != "栄光の騎士団" {
		t.Errorf("expected party name 栄光の騎士団, got %s", sess.PartyName)
	}

	// Verify party status updated to in_progress
	pUpdated, _ := partyRepo.GetParty(ctx, "p1")
	if pUpdated.Status != party.StatusInProgress {
		t.Errorf("expected party status in_progress, got %s", pUpdated.Status)
	}

	// 2. Multi-member active session indexing and privacy
	// Non-leader member can fetch the active session
	memSess, err := svc.GetActiveSession(ctx, "c2")
	if err != nil || memSess == nil {
		t.Fatalf("expected member c2 to find active session, err: %v", err)
	}
	if memSess.ID != sess.ID {
		t.Errorf("expected session ID %s, got %s", sess.ID, memSess.ID)
	}

	// Outside character querying gets nil (privacy check)
	outsiderSess, err := svc.GetActiveSession(ctx, "outsider-char")
	if err != nil {
		t.Fatalf("unexpected error for outsider: %v", err)
	}
	if outsiderSess != nil {
		t.Errorf("expected nil session for outsider, got %v", outsiderSess)
	}

	// 3. Busy member rejection
	partyRepo.addParty(party.Party{
		ID:                "p-busy",
		LeaderCharacterID: "lead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-busy", CharacterID: "lead", ReadyState: true, IsLeader: true},
	})
	_, err = svc.StartPartySession(ctx, "lead", "p-busy", "1")
	if !errors.Is(err, challenge.ErrActiveSessionExists) {
		t.Errorf("expected ErrActiveSessionExists, got %v", err)
	}

	// 4. Stage "0" allows max 1 participant -> ErrTooManyPartyMembers
	partyRepo.addParty(party.Party{
		ID:                "p-too-many",
		LeaderCharacterID: "lead2",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-too-many", CharacterID: "lead2", ReadyState: true, IsLeader: true},
		{PartyID: "p-too-many", CharacterID: "c2-2", ReadyState: true},
	})
	charRepo.chars["lead2"] = createPartyTestChar("lead2", 15, 200, 50, 30)
	charRepo.chars["c2-2"] = createPartyTestChar("c2-2", 15, 180, 45, 25)
	_, err = svc.StartPartySession(ctx, "lead2", "p-too-many", "0")
	if !errors.Is(err, challenge.ErrTooManyPartyMembers) {
		t.Errorf("expected ErrTooManyPartyMembers for stage 0 with 2 members, got %v", err)
	}

	// 5. Non-existent party
	_, err = svc.StartPartySession(ctx, "lead", "p-missing", "1")
	if !errors.Is(err, challenge.ErrPartyNotFound) {
		t.Errorf("expected ErrPartyNotFound, got %v", err)
	}

	// 6. Non-leader actor
	partyRepo.addParty(party.Party{
		ID:                "p-lead-test",
		LeaderCharacterID: "lead-new",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-lead-test", CharacterID: "lead-new", ReadyState: true, IsLeader: true},
		{PartyID: "p-lead-test", CharacterID: "other-actor", ReadyState: true},
	})
	charRepo.chars["lead-new"] = createPartyTestChar("lead-new", 15, 200, 50, 30)
	charRepo.chars["other-actor"] = createPartyTestChar("other-actor", 15, 180, 45, 25)
	_, err = svc.StartPartySession(ctx, "other-actor", "p-lead-test", "1")
	if !errors.Is(err, challenge.ErrNotPartyLeader) {
		t.Errorf("expected ErrNotPartyLeader, got %v", err)
	}

	// 7. Party not recruiting
	partyRepo.addParty(party.Party{
		ID:                "p-not-recruiting",
		LeaderCharacterID: "lead-nr",
		Status:            party.StatusInProgress,
	}, []party.Member{
		{PartyID: "p-not-recruiting", CharacterID: "lead-nr", ReadyState: true, IsLeader: true},
	})
	charRepo.chars["lead-nr"] = createPartyTestChar("lead-nr", 15, 200, 50, 30)
	_, err = svc.StartPartySession(ctx, "lead-nr", "p-not-recruiting", "1")
	if !errors.Is(err, challenge.ErrPartyNotRecruiting) {
		t.Errorf("expected ErrPartyNotRecruiting, got %v", err)
	}

	// 8. Member not ready
	partyRepo.addParty(party.Party{
		ID:                "p-unready",
		LeaderCharacterID: "lead-unready",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-unready", CharacterID: "lead-unready", ReadyState: true, IsLeader: true},
		{PartyID: "p-unready", CharacterID: "c-unready-mem", ReadyState: false},
	})
	charRepo.chars["lead-unready"] = createPartyTestChar("lead-unready", 15, 200, 50, 30)
	charRepo.chars["c-unready-mem"] = createPartyTestChar("c-unready-mem", 15, 180, 45, 25)
	_, err = svc.StartPartySession(ctx, "lead-unready", "p-unready", "1")
	if !errors.Is(err, challenge.ErrPartyNotReady) {
		t.Errorf("expected ErrPartyNotReady, got %v", err)
	}

	// 9. Duplicate party members
	partyRepo.addParty(party.Party{
		ID:                "p-dup",
		LeaderCharacterID: "lead-dup",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-dup", CharacterID: "lead-dup", ReadyState: true, IsLeader: true},
		{PartyID: "p-dup", CharacterID: "lead-dup", ReadyState: true},
	})
	charRepo.chars["lead-dup"] = createPartyTestChar("lead-dup", 15, 200, 50, 30)
	_, err = svc.StartPartySession(ctx, "lead-dup", "p-dup", "1")
	if !errors.Is(err, challenge.ErrDuplicatePartyMember) {
		t.Errorf("expected ErrDuplicatePartyMember, got %v", err)
	}

	// 10. Unconscious character
	partyRepo.addParty(party.Party{
		ID:                "p-dead",
		LeaderCharacterID: "lead-dead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-dead", CharacterID: "lead-dead", ReadyState: true, IsLeader: true},
	})
	deadChar := createPartyTestChar("lead-dead", 15, 0, 50, 30)
	charRepo.chars["lead-dead"] = deadChar
	_, err = svc.StartPartySession(ctx, "lead-dead", "p-dead", "1")
	if !errors.Is(err, challenge.ErrCharacterUnconscious) {
		t.Errorf("expected ErrCharacterUnconscious, got %v", err)
	}
}

func TestPartyAdvanceRoundAndHallOfFame(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	partyRepo := newMockPartyRepo()

	c1 := createPartyTestChar("lead", 30, 350, 100, 80)
	c1.Name = "勇者アリス"
	c1.JobID = "paladin"
	c1.Stats.MP = 50
	c1.Stats.MaxMP = 50
	charRepo.chars[c1.ID] = c1

	c2 := createPartyTestChar("c2", 30, 350, 90, 70)
	c2.Name = "魔法使いボブ"
	c2.JobID = "wizard"
	c2.Stats.MP = 100
	c2.Stats.MaxMP = 100
	charRepo.chars[c2.ID] = c2

	partyRepo.addParty(party.Party{
		ID:                "dream-team",
		LeaderCharacterID: "lead",
		Name:              "ドリームチーム",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "dream-team", CharacterID: "lead", CharacterName: "勇者アリス", ReadyState: true, IsLeader: true},
		{PartyID: "dream-team", CharacterID: "c2", CharacterName: "魔法使いボブ", ReadyState: true},
	})

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithPartyRepository(partyRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	sess, err := svc.StartPartySession(ctx, "lead", "dream-team", "1")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}

	// Advance round 1
	res, updatedSess, err := svc.AdvanceRound(ctx, "lead", sess.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if !res.Won {
		t.Fatalf("expected round 1 victory with party")
	}
	if updatedSess.CurrentRound != 2 {
		t.Errorf("expected round 2, got %d", updatedSess.CurrentRound)
	}

	// Verify all members are present in updated session
	if len(updatedSess.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(updatedSess.Members))
	}
	if updatedSess.Members[0].CharacterName != "勇者アリス" || updatedSess.Members[1].CharacterName != "魔法使いボブ" {
		t.Errorf("unexpected members in updated session: %+v", updatedSess.Members)
	}
}

func TestPartyBattleResolverNoExtraRecovery(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	partyRepo := newMockPartyRepo()

	c1 := createPartyTestChar("p_lead", 15, 300, 150, 10)
	c1.Name = "勇者"
	c1.JobID = "hero"
	charRepo.chars[c1.ID] = c1

	c2 := createPartyTestChar("p_mem", 15, 300, 135, 5)
	c2.Name = "戦士"
	c2.JobID = "warrior"
	charRepo.chars[c2.ID] = c2

	partyRepo.addParty(party.Party{
		ID:                "test-squad",
		LeaderCharacterID: c1.ID,
		Name:              "テスト隊",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "test-squad", CharacterID: c1.ID, CharacterName: "勇者", ReadyState: true, IsLeader: true},
		{PartyID: "test-squad", CharacterID: c2.ID, CharacterName: "戦士", ReadyState: true},
	})

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithPartyRepository(partyRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	sess, err := svc.StartPartySession(ctx, c1.ID, "test-squad", "1")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}

	roundRes, updatedSess, err := svc.AdvanceRound(ctx, c1.ID, sess.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if !roundRes.Won {
		t.Fatalf("expected party to win round 1")
	}

	// Legacy parity check: RecoveredHP must be 0
	if roundRes.RecoveredHP != 0 {
		t.Errorf("expected roundRes.RecoveredHP to be 0 (no recovery), got %d", roundRes.RecoveredHP)
	}

	// Verify member HPs match battle remaining HPs without +20% recovery
	finalRemainingHPs := roundRes.BattleResult.Logs[len(roundRes.BattleResult.Logs)-1].RemainingHP
	for _, m := range updatedSess.Members {
		remHP := finalRemainingHPs[m.CharacterID]
		if m.CharacterCurrentHP != remHP {
			t.Errorf("expected member %s HP to match remaining HP %d, got %d",
				m.CharacterID, remHP, m.CharacterCurrentHP)
		}
	}
}

func TestStartPartySession_PersistenceAndValidationErrors(t *testing.T) {
	ctx := context.Background()

	setupService := func() (*challenge.Service, *mockChallengeRepo, *mockCharRepo, *mockActiveStore, *mockPartyRepo) {
		repo := newMockChallengeRepo()
		charRepo := &mockCharRepo{
			chars: map[string]corecharacter.Character{
				"lead": createPartyTestChar("lead", 15, 200, 50, 30),
				"c2":   createPartyTestChar("c2", 15, 180, 45, 25),
			},
		}
		activeStore := newMockActiveStore(nil)
		partyRepo := newMockPartyRepo()
		partyRepo.addParty(party.Party{
			ID:                "p-test",
			LeaderCharacterID: "lead",
			Name:              "Party",
			Status:            party.StatusRecruiting,
		}, []party.Member{
			{PartyID: "p-test", CharacterID: "lead", ReadyState: true, IsLeader: true},
		})
		svc, err := challenge.NewService(
			repo,
			charRepo,
			&corebattle.Engine{},
			challenge.WithActiveSessionStore(activeStore),
			challenge.WithPartyRepository(partyRepo),
		)
		if err != nil {
			t.Fatalf("failed to create challenge service: %v", err)
		}
		return svc, repo, charRepo, activeStore, partyRepo
	}

	t.Run("empty leader character ID returns ErrCharacterNotFound", func(t *testing.T) {
		svc, _, _, _, _ := setupService()
		_, err := svc.StartPartySession(ctx, "", "p-test", "0")
		if !errors.Is(err, challenge.ErrCharacterNotFound) {
			t.Errorf("expected ErrCharacterNotFound, got %v", err)
		}
	})

	t.Run("activeStore GetActiveSession error propagates", func(t *testing.T) {
		svc, _, _, activeStore, _ := setupService()
		activeStore.getActiveSessionErr = errors.New("valkey cluster unavailable")

		_, err := svc.StartPartySession(ctx, "lead", "p-test", "0")
		if err == nil || !strings.Contains(err.Error(), "valkey cluster unavailable") {
			t.Errorf("expected valkey cluster unavailable error, got %v", err)
		}
	})

	t.Run("charRepo FindByID error propagates", func(t *testing.T) {
		svc, _, charRepo, _, _ := setupService()
		charRepo.findByIDErr = errors.New("mariadb connection lost")

		_, err := svc.StartPartySession(ctx, "lead", "p-test", "0")
		if err == nil || !strings.Contains(err.Error(), "mariadb connection lost") {
			t.Errorf("expected charRepo error, got %v", err)
		}
	})

	t.Run("activeStore SaveActiveSession error propagates", func(t *testing.T) {
		svc, _, _, activeStore, _ := setupService()
		activeStore.saveActiveSessionErr = errors.New("valkey write timeout")

		_, err := svc.StartPartySession(ctx, "lead", "p-test", "0")
		if err == nil || !strings.Contains(err.Error(), "valkey write timeout") {
			t.Errorf("expected activeStore save error, got %v", err)
		}
	})

	t.Run("repo SaveSession error propagates with context", func(t *testing.T) {
		svc, repo, _, _, _ := setupService()
		repo.saveSessionErr = errors.New("duplicate key in challenge_sessions")

		_, err := svc.StartPartySession(ctx, "lead", "p-test", "0")
		if err == nil || !strings.Contains(err.Error(), "saving challenge session: duplicate key in challenge_sessions") {
			t.Errorf("expected wrapped repo save session error, got %v", err)
		}
	})
}

func TestPartySession_RollbackOnStartFailure(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	partyRepo.addParty(party.Party{
		ID:                "p-fail",
		LeaderCharacterID: "lead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-fail", CharacterID: "lead", ReadyState: true, IsLeader: true},
	})

	repo := newMockChallengeRepo()
	repo.saveSessionErr = errors.New("db error")

	svc, _ := challenge.NewService(
		repo,
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"lead": createPartyTestChar("lead", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
	)

	_, err := svc.StartPartySession(ctx, "lead", "p-fail", "0")
	if err == nil {
		t.Fatal("expected start to fail")
	}

	p, err := partyRepo.GetParty(ctx, "p-fail")
	if err != nil || p.Status != party.StatusRecruiting {
		t.Errorf("expected party status recruiting after rollback, got %s", p.Status)
	}
}

func TestPartySession_ActivityChecker(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	partyRepo.addParty(party.Party{
		ID:                "p-act",
		LeaderCharacterID: "lead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-act", CharacterID: "lead", ReadyState: true, IsLeader: true},
		{PartyID: "p-act", CharacterID: "m2", ReadyState: true},
	})

	checker := challenge.ActivityCheckerFunc(func(ctx context.Context, charID string) (bool, error) {
		if charID == "m2" {
			return true, nil
		}
		return false, nil
	})

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"lead": createPartyTestChar("lead", 10, 100, 20, 20),
			"m2":   createPartyTestChar("m2", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
		challenge.WithActivityChecker(checker),
	)

	// Party start fails because m2 has active activity
	_, err := svc.StartPartySession(ctx, "lead", "p-act", "2")
	if !errors.Is(err, challenge.ErrActiveSessionExists) {
		t.Errorf("expected ErrActiveSessionExists when member has active activity, got %v", err)
	}

	// Solo start fails for m2
	_, err = svc.StartSession(ctx, "m2", "0")
	if !errors.Is(err, challenge.ErrActiveSessionExists) {
		t.Errorf("expected ErrActiveSessionExists when solo character has active activity, got %v", err)
	}
}

type mockPartyLossResolver struct{}

func (mockPartyLossResolver) Resolve(req corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{
		Outcome:  corebattle.OutcomeDefeat,
		WinnerID: "monster",
	}, nil
}

func (mockPartyLossResolver) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeDefeat,
		WinnerSide:  "enemies",
		RemainingHP: map[string]int{"lead": 0, "m2": 0},
	}, nil
}

func TestPartySession_DefeatResetsPartyState(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	partyRepo.addParty(party.Party{
		ID:                "p-defeat",
		LeaderCharacterID: "lead",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "p-defeat", CharacterID: "lead", ReadyState: true, IsLeader: true},
		{PartyID: "p-defeat", CharacterID: "m2", ReadyState: true},
	})

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"lead": createPartyTestChar("lead", 10, 100, 20, 20),
			"m2":   createPartyTestChar("m2", 10, 100, 20, 20),
		}},
		mockPartyLossResolver{},
		challenge.WithPartyRepository(partyRepo),
	)

	session, err := svc.StartPartySession(ctx, "lead", "p-defeat", "2")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}

	// Advance round -> defeat
	res, _, err := svc.AdvanceRound(ctx, "lead", session.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if res.Won {
		t.Fatal("expected loss/defeat")
	}

	// Party status should be recruiting and members unready
	p, err := partyRepo.GetParty(ctx, "p-defeat")
	if err != nil || p.Status != party.StatusRecruiting {
		t.Errorf("expected party status recruiting after defeat, got %s", p.Status)
	}
	members, err := partyRepo.GetMembers(ctx, "p-defeat")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	for _, m := range members {
		if m.ReadyState {
			t.Errorf("expected member %s ready state to be false after defeat", m.CharacterID)
		}
	}
}

func TestStartPartySession_Concurrent(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	partyRepo.addParty(party.Party{
		ID:                "party-challenge-concurrent",
		LeaderCharacterID: "leader-1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-challenge-concurrent", CharacterID: "leader-1", ReadyState: true, IsLeader: true},
		{PartyID: "party-challenge-concurrent", CharacterID: "member-2", ReadyState: true},
	})

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"leader-1": createPartyTestChar("leader-1", 10, 100, 20, 20),
			"member-2": createPartyTestChar("member-2", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
	)

	const concurrency = 5
	errs := make(chan error, concurrency)
	successes := make(chan *challenge.ChallengeSession, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sess, err := svc.StartPartySession(ctx, "leader-1", "party-challenge-concurrent", "2")
			if err != nil {
				errs <- err
			} else {
				successes <- sess
			}
		}()
	}

	wg.Wait()
	close(errs)
	close(successes)

	var successCount int
	for range successes {
		successCount++
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 start to succeed, got %d", successCount)
	}

	for err := range errs {
		if !errors.Is(err, challenge.ErrPartyNotRecruiting) {
			t.Errorf("expected concurrent failure ErrPartyNotRecruiting, got %v", err)
		}
	}
}

type failSaveSessionRepo struct {
	challenge.Repository
}

func (f failSaveSessionRepo) SaveSession(ctx context.Context, session challenge.ChallengeSession) error {
	return errors.New("database connection failed")
}

func TestStartPartySession_SaveSessionFailureRollback(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	partyRepo.addParty(party.Party{
		ID:                "party-fail-save",
		LeaderCharacterID: "leader-1",
		Status:            party.StatusRecruiting,
	}, []party.Member{
		{PartyID: "party-fail-save", CharacterID: "leader-1", ReadyState: true, IsLeader: true},
		{PartyID: "party-fail-save", CharacterID: "member-2", ReadyState: true},
	})

	activeStore := challenge.NewMemorySessionRepository()
	baseRepo := newMockChallengeRepo()
	repo := failSaveSessionRepo{Repository: baseRepo}

	svc, _ := challenge.NewService(
		repo,
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"leader-1": createPartyTestChar("leader-1", 10, 100, 20, 20),
			"member-2": createPartyTestChar("member-2", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
		challenge.WithActiveSessionStore(activeStore),
	)

	_, err := svc.StartPartySession(ctx, "leader-1", "party-fail-save", "2")
	if err == nil {
		t.Fatal("expected StartPartySession to fail when SaveSession fails")
	}

	// Active store should have NO active session for leader or member (cleaned up)
	leadSess, err := activeStore.GetActiveSession(ctx, "leader-1")
	if err != nil {
		t.Fatalf("GetActiveSession for leader returned error: %v", err)
	}
	if leadSess != nil {
		t.Errorf("expected active session to be purged from activeStore for leader, got %+v", leadSess)
	}

	memSess, err := activeStore.GetActiveSession(ctx, "member-2")
	if err != nil {
		t.Fatalf("GetActiveSession for member returned error: %v", err)
	}
	if memSess != nil {
		t.Errorf("expected active session to be purged from activeStore for member, got %+v", memSess)
	}

	// Party status should be rolled back to recruiting
	p, err := partyRepo.GetParty(ctx, "party-fail-save")
	if err != nil {
		t.Fatalf("GetParty returned error: %v", err)
	}
	if p.Status != party.StatusRecruiting {
		t.Errorf("expected party status to remain StatusRecruiting, got %v", p.Status)
	}
}

func TestPartySession_Defeat_ResetsReadyStates_PreventsStaleRestart(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	pID := "party-defeat-test"
	partyRepo.addParty(party.Party{
		ID:                pID,
		LeaderCharacterID: "leader-1",
		Status:            party.StatusRecruiting,
		Name:              "Defeat Party",
	}, []party.Member{
		{PartyID: pID, CharacterID: "leader-1", ReadyState: true},
		{PartyID: pID, CharacterID: "member-2", ReadyState: true},
	})

	activeStore := challenge.NewMemorySessionRepository()
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		"leader-1": createPartyTestChar("leader-1", 10, 10, 1, 1), // weak stats to guarantee defeat
		"member-2": createPartyTestChar("member-2", 10, 10, 1, 1),
	}}

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		charRepo,
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
		challenge.WithActiveSessionStore(activeStore),
	)

	// 1. Start party session
	sess, err := svc.StartPartySession(ctx, "leader-1", pID, "2")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}

	p, err := partyRepo.GetParty(ctx, pID)
	if err != nil || p.Status != party.StatusInProgress {
		t.Fatalf("expected party to be in progress, got status %v", p.Status)
	}

	// 2. Advance round until defeat
	for i := 0; i < 10; i++ {
		res, curSess, err := svc.AdvanceRound(ctx, "leader-1", sess.ID)
		if err != nil {
			t.Fatalf("AdvanceRound failed: %v", err)
		}
		if res.SessionEnded || curSess.Status == challenge.StatusDefeated {
			break
		}
	}

	// 3. Verify party status reset to recruiting and member ready states reset to false
	p, err = partyRepo.GetParty(ctx, pID)
	if err != nil || p.Status != party.StatusRecruiting {
		t.Fatalf("expected party to be recruiting after defeat settlement, got status %v", p.Status)
	}

	members, err := partyRepo.GetMembers(ctx, pID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	for _, m := range members {
		if m.ReadyState {
			t.Errorf("expected member %s ready state to be reset to false, was true", m.CharacterID)
		}
	}

	// 4. Attempting to restart party session immediately must fail with ErrPartyNotReady
	_, err = svc.StartPartySession(ctx, "leader-1", pID, "2")
	if !errors.Is(err, challenge.ErrPartyNotReady) {
		t.Fatalf("expected StartPartySession to fail with ErrPartyNotReady, got %v", err)
	}

	// 5. Active session store must be purged
	leadSess, _ := activeStore.GetActiveSession(ctx, "leader-1")
	if leadSess != nil {
		t.Errorf("expected leader active session to be purged, got %+v", leadSess)
	}
	memSess, _ := activeStore.GetActiveSession(ctx, "member-2")
	if memSess != nil {
		t.Errorf("expected member active session to be purged, got %+v", memSess)
	}
}

func TestConcurrent_PartySession_Vs_SoloSession(t *testing.T) {
	ctx := context.Background()

	for iter := 0; iter < 10; iter++ {
		partyRepo := newMockPartyRepo()
		pID := fmt.Sprintf("party-race-%d", iter)
		partyRepo.addParty(party.Party{
			ID:                pID,
			LeaderCharacterID: "leader-1",
			Status:            party.StatusRecruiting,
			Name:              "Race Party",
		}, []party.Member{
			{PartyID: pID, CharacterID: "leader-1", ReadyState: true},
			{PartyID: pID, CharacterID: "member-2", ReadyState: true},
		})

		activeStore := challenge.NewMemorySessionRepository()
		svc, _ := challenge.NewService(
			newMockChallengeRepo(),
			&mockCharRepo{chars: map[string]corecharacter.Character{
				"leader-1": createPartyTestChar("leader-1", 10, 100, 20, 20),
				"member-2": createPartyTestChar("member-2", 10, 100, 20, 20),
			}},
			&corebattle.Engine{},
			challenge.WithPartyRepository(partyRepo),
			challenge.WithActiveSessionStore(activeStore),
		)

		var wg sync.WaitGroup
		var partyErr, soloErr error
		var partySess, soloSess *challenge.ChallengeSession

		wg.Add(2)
		go func() {
			defer wg.Done()
			partySess, partyErr = svc.StartPartySession(ctx, "leader-1", pID, "2")
		}()
		go func() {
			defer wg.Done()
			soloSess, soloErr = svc.StartSession(ctx, "member-2", "2")
		}()
		wg.Wait()

		partySuccess := partyErr == nil && partySess != nil
		soloSuccess := soloErr == nil && soloSess != nil

		if partySuccess && soloSuccess {
			t.Fatalf("iteration %d: both party session and solo session succeeded concurrently! dual activity violation", iter)
		}
		if !partySuccess && !soloSuccess {
			t.Fatalf("iteration %d: neither party session nor solo session succeeded (partyErr=%v, soloErr=%v)", iter, partyErr, soloErr)
		}

		if partySuccess {
			if !errors.Is(soloErr, challenge.ErrActiveSessionExists) && !errors.Is(soloErr, challenge.ErrPartyNotRecruiting) {
				t.Errorf("iteration %d: expected solo start to fail with ErrActiveSessionExists or ErrPartyNotRecruiting, got %v", iter, soloErr)
			}
		} else {
			if !errors.Is(partyErr, challenge.ErrActiveSessionExists) && !errors.Is(partyErr, challenge.ErrPartyNotRecruiting) {
				t.Errorf("iteration %d: expected party start to fail with ErrActiveSessionExists or ErrPartyNotRecruiting, got %v", iter, partyErr)
			}
		}
	}
}

func TestStartSession_LockedCharacter_ReturnsErrActiveSessionExists(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"solo-char": createPartyTestChar("solo-char", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
	)

	// Hold character lock (simulating another domain starting an adventure for this character)
	err := partyRepo.WithPartyLock(ctx, "char:solo-char", func(lockedCtx context.Context) error {
		_, err := svc.StartSession(ctx, "solo-char", "2")
		if !errors.Is(err, challenge.ErrActiveSessionExists) {
			t.Fatalf("expected ErrActiveSessionExists when character lock is held, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error holding lock: %v", err)
	}
}

func TestResetPartyState_Challenge_LockContention_RetriesAndSucceeds(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	pID := "party-challenge-contend-retry"
	partyRepo.addParty(party.Party{
		ID:                pID,
		LeaderCharacterID: "leader-1",
		Status:            party.StatusInProgress,
		Name:              "Contended Challenge Party",
	}, []party.Member{
		{PartyID: pID, CharacterID: "leader-1", ReadyState: true},
		{PartyID: pID, CharacterID: "member-2", ReadyState: true},
	})

	activeStore := challenge.NewMemorySessionRepository()
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		"leader-1": createPartyTestChar("leader-1", 10, 10, 1, 1),
		"member-2": createPartyTestChar("member-2", 10, 10, 1, 1),
	}}

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		charRepo,
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
		challenge.WithActiveSessionStore(activeStore),
	)

	// Set active session in progress
	sessID := "sess-contend"
	_ = activeStore.SaveActiveSession(ctx, challenge.ChallengeSession{
		ID:           sessID,
		CharacterID:  "leader-1",
		PartyID:      pID,
		TierID:       "2",
		CurrentRound: 1,
		Status:       challenge.StatusActive,
		Members: []challenge.ChallengeMember{
			{CharacterID: "leader-1", CharacterCurrentHP: 0},
			{CharacterID: "member-2", CharacterCurrentHP: 0},
		},
	})

	// Simulate lock held momentarily by another process (50ms)
	var lockHeld sync.WaitGroup
	lockHeld.Add(1)
	go func() {
		_ = partyRepo.WithPartyLock(ctx, pID, func(lockedCtx context.Context) error {
			lockHeld.Done()
			time.Sleep(50 * time.Millisecond)
			return nil
		})
	}()
	lockHeld.Wait()

	// Advance round triggers defeat and resetPartyState
	_, _, err := svc.AdvanceRound(ctx, "leader-1", sessID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}

	p, err := partyRepo.GetParty(ctx, pID)
	if err != nil || p.Status != party.StatusRecruiting {
		t.Fatalf("expected party to be recruiting, got status %v (err: %v)", p.Status, err)
	}
	mems, err := partyRepo.GetMembers(ctx, pID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	for _, m := range mems {
		if m.ReadyState {
			t.Errorf("expected member %s ready state to be false, was true", m.CharacterID)
		}
	}
}

func TestResetPartyState_Challenge_PersistentLockContention_MaintainsExclusionAndSurfacesError(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	pID := "party-challenge-persist-contend"
	partyRepo.addParty(party.Party{
		ID:                pID,
		LeaderCharacterID: "leader-1",
		Status:            party.StatusInProgress,
		Name:              "Persistent Contended Challenge Party",
	}, []party.Member{
		{PartyID: pID, CharacterID: "leader-1", ReadyState: true},
		{PartyID: pID, CharacterID: "member-2", ReadyState: true},
	})

	activeStore := challenge.NewMemorySessionRepository()
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		"leader-1": createPartyTestChar("leader-1", 10, 10, 1, 1),
		"member-2": createPartyTestChar("member-2", 10, 10, 1, 1),
	}}

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		charRepo,
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
		challenge.WithActiveSessionStore(activeStore),
	)

	sessID := "sess-persist"
	_ = activeStore.SaveActiveSession(ctx, challenge.ChallengeSession{
		ID:           sessID,
		CharacterID:  "leader-1",
		PartyID:      pID,
		TierID:       "2",
		CurrentRound: 1,
		Status:       challenge.StatusActive,
		Members: []challenge.ChallengeMember{
			{CharacterID: "leader-1", CharacterCurrentHP: 0},
			{CharacterID: "member-2", CharacterCurrentHP: 0},
		},
	})

	// Lock is held permanently for 2 seconds (exceeding retry attempts)
	var lockHeld sync.WaitGroup
	var lockDone sync.WaitGroup
	lockHeld.Add(1)
	lockDone.Add(1)
	go func() {
		defer lockDone.Done()
		_ = partyRepo.WithPartyLock(ctx, pID, func(lockedCtx context.Context) error {
			lockHeld.Done()
			time.Sleep(2 * time.Second)
			return nil
		})
	}()
	lockHeld.Wait()

	// AdvanceRound triggers defeat; must NOT bypass lock and must return error
	_, _, err := svc.AdvanceRound(ctx, "leader-1", sessID)
	if err == nil {
		t.Fatalf("expected AdvanceRound to fail due to lock contention, got nil")
	}
	if !errors.Is(err, party.ErrPartyNotRecruiting) {
		t.Fatalf("expected ErrPartyNotRecruiting, got %v", err)
	}

	// While lock was held, party status and ready states MUST NOT have been bypassed/mutated
	p, err := partyRepo.GetParty(ctx, pID)
	if err != nil || p.Status != party.StatusInProgress {
		t.Fatalf("expected party to remain in_progress under lock, got status %v (err: %v)", p.Status, err)
	}
	mems, err := partyRepo.GetMembers(ctx, pID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	for _, m := range mems {
		if !m.ReadyState {
			t.Errorf("expected member %s ready state to remain true under lock, was false", m.CharacterID)
		}
	}
	lockDone.Wait()

	// Retrying AdvanceRound after lock release safely recovers the settled session's party state without double-granting rewards
	retryRes, _, retryErr := svc.AdvanceRound(ctx, "leader-1", sessID)
	if retryErr != nil {
		t.Fatalf("expected retry AdvanceRound to recover party state, got: %v", retryErr)
	}
	if !retryRes.SessionEnded || retryRes.SessionStatus != challenge.StatusDefeated {
		t.Fatalf("expected retry AdvanceRound result to be defeated session")
	}

	// Party status is now reset to recruiting and members are unready
	pAfter, err := partyRepo.GetParty(ctx, pID)
	if err != nil || pAfter.Status != party.StatusRecruiting {
		t.Fatalf("expected party status to be recruiting after recovery, got %v (err: %v)", pAfter.Status, err)
	}
	memsAfter, err := partyRepo.GetMembers(ctx, pID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	for _, m := range memsAfter {
		if m.ReadyState {
			t.Errorf("expected member %s ready state to be false after recovery, was true", m.CharacterID)
		}
	}

	// Active session is now purged; subsequent AdvanceRound returns ErrSessionNotFound
	_, _, postPurgeErr := svc.AdvanceRound(ctx, "leader-1", sessID)
	if !errors.Is(postPurgeErr, challenge.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound after buffer purge, got: %v", postPurgeErr)
	}
}

func TestResetPartyState_Challenge_ValkeyFailure_SurfacesErrorWithoutLockBypass(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	pID := "party-challenge-valkey-down"
	partyRepo.addParty(party.Party{
		ID:                pID,
		LeaderCharacterID: "leader-1",
		Status:            party.StatusInProgress,
		Name:              "Challenge Valkey Down Party",
	}, []party.Member{
		{PartyID: pID, CharacterID: "leader-1", ReadyState: true},
		{PartyID: pID, CharacterID: "member-2", ReadyState: true},
	})

	activeStore := challenge.NewMemorySessionRepository()
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
		"leader-1": createPartyTestChar("leader-1", 10, 10, 1, 1),
		"member-2": createPartyTestChar("member-2", 10, 10, 1, 1),
	}}

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		charRepo,
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
		challenge.WithActiveSessionStore(activeStore),
	)

	sessID := "sess-valkey-fail"
	_ = activeStore.SaveActiveSession(ctx, challenge.ChallengeSession{
		ID:           sessID,
		CharacterID:  "leader-1",
		PartyID:      pID,
		TierID:       "2",
		CurrentRound: 1,
		Status:       challenge.StatusActive,
		Members: []challenge.ChallengeMember{
			{CharacterID: "leader-1", CharacterCurrentHP: 0},
			{CharacterID: "member-2", CharacterCurrentHP: 0},
		},
	})

	// Simulate Valkey failure on party lock
	valkeyErr := errors.New("valkey: connection refused")
	partyRepo.lockErr = valkeyErr

	// AdvanceRound fails and surfaces the error, without bypassing the lock
	_, _, err := svc.AdvanceRound(ctx, "leader-1", sessID)
	if err == nil {
		t.Fatalf("expected AdvanceRound to fail due to valkey failure, got nil")
	}
	if !errors.Is(err, valkeyErr) {
		t.Fatalf("expected valkey failure error to propagate, got: %v", err)
	}

	// State in DB was NOT bypassed/mutated outside of lock
	p, err := partyRepo.GetParty(ctx, pID)
	if err != nil || p.Status != party.StatusInProgress {
		t.Fatalf("expected party status to remain StatusInProgress, got %v (err: %v)", p.Status, err)
	}
	mems, err := partyRepo.GetMembers(ctx, pID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	for _, m := range mems {
		if !m.ReadyState {
			t.Errorf("expected member %s ready state to remain true, was false", m.CharacterID)
		}
	}

	// Valkey recovers
	partyRepo.lockErr = nil

	// Retrying AdvanceRound recovers party state safely and purges active session
	retryRes, _, retryErr := svc.AdvanceRound(ctx, "leader-1", sessID)
	if retryErr != nil {
		t.Fatalf("expected retry AdvanceRound after valkey recovery to succeed, got: %v", retryErr)
	}
	if !retryRes.SessionEnded || retryRes.SessionStatus != challenge.StatusDefeated {
		t.Fatalf("expected retry AdvanceRound to end with defeat")
	}

	pRecovered, err := partyRepo.GetParty(ctx, pID)
	if err != nil || pRecovered.Status != party.StatusRecruiting {
		t.Fatalf("expected party status to be recruiting, got %v (err: %v)", pRecovered.Status, err)
	}
	for _, m := range mems {
		mRecovered, _ := partyRepo.GetMembers(ctx, pID)
		for _, mr := range mRecovered {
			if mr.CharacterID == m.CharacterID && mr.ReadyState {
				t.Errorf("expected member %s ready state to be false after recovery", mr.CharacterID)
			}
		}
	}
}

func TestSettledPartyRun_Challenge_SoloStartRace_CannotOverwriteOrDeleteNewRun(t *testing.T) {
	ctx := context.Background()
	partyRepo := newMockPartyRepo()
	pID := "party-challenge-solo-race"
	partyRepo.addParty(party.Party{
		ID:                pID,
		LeaderCharacterID: "leader-1",
		Status:            party.StatusInProgress,
		Name:              "Settled Challenge Race Party",
	}, []party.Member{
		{PartyID: pID, CharacterID: "leader-1", ReadyState: true},
		{PartyID: pID, CharacterID: "member-2", ReadyState: true},
	})

	activeStore := challenge.NewMemorySessionRepository()
	sess1 := challenge.ChallengeSession{
		ID:           "sess-party-1",
		CharacterID:  "leader-1",
		PartyID:      pID,
		TierID:       "0",
		CurrentRound: 1,
		Status:       challenge.StatusDefeated,
		Members: []challenge.ChallengeMember{
			{CharacterID: "leader-1", CharacterCurrentHP: 0},
			{CharacterID: "member-2", CharacterCurrentHP: 0},
		},
	}
	_ = activeStore.SaveActiveSession(ctx, sess1)

	svc, _ := challenge.NewService(
		newMockChallengeRepo(),
		&mockCharRepo{chars: map[string]corecharacter.Character{
			"leader-1": createPartyTestChar("leader-1", 10, 100, 20, 20),
			"member-2": createPartyTestChar("member-2", 10, 100, 20, 20),
		}},
		&corebattle.Engine{},
		challenge.WithPartyRepository(partyRepo),
		challenge.WithActiveSessionStore(activeStore),
	)

	// 1. While settled party session sess1 remains in activeStore, solo start is prohibited for both leader and member
	_, err := svc.StartSession(ctx, "leader-1", "0")
	if !errors.Is(err, challenge.ErrActiveSessionExists) {
		t.Fatalf("expected ErrActiveSessionExists for leader when settled session is unrecovered, got: %v", err)
	}
	_, err = svc.StartSession(ctx, "member-2", "0")
	if !errors.Is(err, challenge.ErrActiveSessionExists) {
		t.Fatalf("expected ErrActiveSessionExists for member when settled session is unrecovered, got: %v", err)
	}

	// 2. AdvanceRound recovers party state and purges sess1
	res, _, err := svc.AdvanceRound(ctx, "leader-1", "sess-party-1")
	if err != nil || !res.SessionEnded {
		t.Fatalf("expected AdvanceRound to recover party state, got err: %v", err)
	}

	// 3. Leader now starts a new solo run sess2 cleanly
	soloSess, err := svc.StartSession(ctx, "leader-1", "0")
	if err != nil || soloSess == nil {
		t.Fatalf("expected solo start to succeed after party recovery, got: %v", err)
	}
	if soloSess.ID == sess1.ID {
		t.Fatalf("expected new solo session to have different ID from sess1")
	}

	// 4. Stale recovery/completion of old run sess1 must NOT delete or overwrite new solo run sess2
	cur, cErr := activeStore.GetActiveSession(ctx, "leader-1")
	if cErr != nil || cur == nil {
		t.Fatalf("expected solo session to remain in activeStore, got nil (err: %v)", cErr)
	}
	if cur.ID != soloSess.ID {
		t.Fatalf("expected activeStore to retain new solo session %s, but got %s", soloSess.ID, cur.ID)
	}
}
