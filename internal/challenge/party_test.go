package challenge_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/party"
)

type mockPartyRepo struct {
	parties map[string]party.Party
	members map[string][]party.Member
}

func newMockPartyRepo() *mockPartyRepo {
	return &mockPartyRepo{
		parties: make(map[string]party.Party),
		members: make(map[string][]party.Member),
	}
}

func (m *mockPartyRepo) GetParty(ctx context.Context, id string) (party.Party, error) {
	p, ok := m.parties[id]
	if !ok {
		return party.Party{}, party.ErrNotFound
	}
	return p, nil
}

func (m *mockPartyRepo) GetMembers(ctx context.Context, partyID string) ([]party.Member, error) {
	mems, ok := m.members[partyID]
	if !ok {
		return nil, party.ErrNotFound
	}
	return mems, nil
}

func (m *mockPartyRepo) UpdateParty(ctx context.Context, p party.Party) error {
	m.parties[p.ID] = p
	return nil
}

func (m *mockPartyRepo) addParty(p party.Party, members []party.Member) {
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
