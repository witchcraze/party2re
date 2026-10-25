package challenge_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func createPartyTestChar(id string, level int, hp int, atk int, def int) corecharacter.Character {
	return corecharacter.Character{
		ID:         id,
		Level:      level,
		Experience: 0,
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

	c1 := createPartyTestChar("lead", 15, 200, 50, 30)
	c1.Name = "挑戦勇者"
	c1.JobID = "hero"
	charRepo.chars[c1.ID] = c1

	c2 := createPartyTestChar("c2", 15, 180, 45, 25)
	c2.Name = "挑戦戦士"
	c2.JobID = "warrior"
	charRepo.chars[c2.ID] = c2

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatalf("failed to create challenge service: %v", err)
	}
	ctx := context.Background()

	// 1. Success starting party session on stage "1" (allows up to 2 participants, hp < 400)
	sess, err := svc.StartPartySession(ctx, "lead", []string{"lead", "c2"}, "1", "栄光の騎士団", "#00FF00")
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
	if sess.PartyColor != "#00FF00" {
		t.Errorf("expected party color #00FF00, got %s", sess.PartyColor)
	}

	// 2. Stage "0" allows max 1 participant -> ErrTooManyPartyMembers
	_, err = svc.StartPartySession(ctx, "lead", []string{"lead", "c2"}, "0", "TooMany", "#00FF00")
	if !errors.Is(err, challenge.ErrTooManyPartyMembers) {
		t.Errorf("expected ErrTooManyPartyMembers for stage 0 with 2 members, got %v", err)
	}
}

func TestPartyAdvanceRoundAndHallOfFame(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}

	c1 := createPartyTestChar("lead", 30, 350, 500, 300)
	c1.Name = "勇者アリス"
	c1.JobID = "paladin"
	c1.Stats.MP = 50
	c1.Stats.MaxMP = 50
	charRepo.chars[c1.ID] = c1

	c2 := createPartyTestChar("c2", 30, 350, 450, 280)
	c2.Name = "魔法使いボブ"
	c2.JobID = "wizard"
	c2.Stats.MP = 100
	c2.Stats.MaxMP = 100
	charRepo.chars[c2.ID] = c2

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	sess, err := svc.StartPartySession(ctx, "lead", []string{"lead", "c2"}, "1", "ドリームチーム", "#FF0000")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}

	// Advance round 1
	roundRes, updatedSess, err := svc.AdvanceRound(ctx, "lead", sess.ID, sess.Revision)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if !roundRes.Won {
		t.Fatalf("expected round 1 to be won by strong party")
	}
	if updatedSess.CurrentRound != 2 {
		t.Errorf("expected session round to advance to 2, got %d", updatedSess.CurrentRound)
	}

	// Check surviving member HP carryover
	for _, m := range updatedSess.Members {
		if m.CharacterCurrentHP <= 0 {
			t.Errorf("expected surviving member %s to have positive HP", m.CharacterID)
		}
	}

	// Check Hall of Fame record
	hof, err := svc.GetHallOfFame(ctx, "1")
	if err != nil {
		t.Fatalf("GetHallOfFame failed: %v", err)
	}
	if hof == nil {
		t.Fatal("expected Hall of Fame entry to be created for new highest round")
	}
	if hof.HighestRound != 1 {
		t.Errorf("expected Hall of Fame highest round 1, got %d", hof.HighestRound)
	}
	if hof.PartyName != "ドリームチーム" {
		t.Errorf("expected Hall of Fame party name ドリームチーム, got %s", hof.PartyName)
	}
	if len(hof.Members) != 2 {
		t.Fatalf("expected 2 members in Hall of Fame, got %d", len(hof.Members))
	}
	if hof.Members[0].CharacterName != "勇者アリス" || hof.Members[0].JobID != "paladin" {
		t.Errorf("unexpected member 0 in Hall of Fame: %+v", hof.Members[0])
	}
	if hof.Members[1].CharacterName != "魔法使いボブ" || hof.Members[1].JobID != "wizard" {
		t.Errorf("unexpected member 1 in Hall of Fame: %+v", hof.Members[1])
	}
}

func TestSoloAdvanceRound_NoFictionalHPRecovery(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}

	// Level 10 character with MaxHP 350, Attack 400, Defense 120 so monster damage goes through
	c1 := createPartyTestChar("solo1", 10, 350, 400, 120)
	c1.Stats.Agility = 150
	c1.Name = "ソロ戦士"
	c1.JobID = "warrior"
	charRepo.chars[c1.ID] = c1

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	sess, err := svc.StartSession(ctx, c1.ID, "0")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	roundRes, updatedSess, err := svc.AdvanceRound(ctx, c1.ID, sess.ID, sess.Revision)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if !roundRes.Won {
		t.Fatalf("expected solo player to win round 1")
	}

	// Legacy parity check: RecoveredHP must be 0
	if roundRes.RecoveredHP != 0 {
		t.Errorf("expected roundRes.RecoveredHP to be 0 (no recovery), got %d", roundRes.RecoveredHP)
	}

	// Verify surviving HP carried over directly from the last combat log
	if len(roundRes.BattleResult.Logs) == 0 {
		t.Fatalf("expected battle logs")
	}
	finalRemainingHP := roundRes.BattleResult.Logs[len(roundRes.BattleResult.Logs)-1].RemainingHP[c1.ID]
	if updatedSess.CharacterCurrentHP != finalRemainingHP {
		t.Errorf("expected session HP to exactly match combat final remaining HP %d, got %d",
			finalRemainingHP, updatedSess.CharacterCurrentHP)
	}
}

func TestPartyAdvanceRound_NoFictionalHPRecovery(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}

	c1 := createPartyTestChar("p_lead", 15, 350, 450, 5)
	c1.Name = "勇者"
	c1.JobID = "hero"
	charRepo.chars[c1.ID] = c1

	c2 := createPartyTestChar("p_mem", 15, 300, 420, 5)
	c2.Name = "戦士"
	c2.JobID = "warrior"
	charRepo.chars[c2.ID] = c2

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	sess, err := svc.StartPartySession(ctx, c1.ID, []string{c1.ID, c2.ID}, "1", "テスト隊", "#00AA00")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}

	roundRes, updatedSess, err := svc.AdvanceRound(ctx, c1.ID, sess.ID, sess.Revision)
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

	setupService := func() (*challenge.Service, *mockChallengeRepo, *mockCharRepo, *mockActiveStore) {
		repo := newMockChallengeRepo()
		charRepo := &mockCharRepo{
			chars: map[string]corecharacter.Character{
				"lead": createPartyTestChar("lead", 15, 200, 50, 30),
				"c2":   createPartyTestChar("c2", 15, 180, 45, 25),
			},
		}
		activeStore := newMockActiveStore(nil)
		svc, err := challenge.NewService(
			repo,
			charRepo,
			&corebattle.Engine{},
			challenge.WithActiveSessionStore(activeStore),
			challenge.WithAdmission(approvedRoster{}),
		)
		if err != nil {
			t.Fatalf("failed to create challenge service: %v", err)
		}
		return svc, repo, charRepo, activeStore
	}

	t.Run("empty leader character ID returns ErrCharacterNotFound", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, err := svc.StartPartySession(ctx, "", []string{"c2"}, "0", "Party", "#FFFFFF")
		if !errors.Is(err, challenge.ErrCharacterNotFound) {
			t.Errorf("expected ErrCharacterNotFound, got %v", err)
		}
	})

	t.Run("activeStore GetActiveSession error propagates", func(t *testing.T) {
		svc, _, _, activeStore := setupService()
		activeStore.getActiveSessionErr = errors.New("valkey cluster unavailable")

		_, err := svc.StartPartySession(ctx, "lead", []string{"lead"}, "0", "Party", "#FFFFFF")
		if err == nil || !strings.Contains(err.Error(), "valkey cluster unavailable") {
			t.Errorf("expected valkey cluster unavailable error, got %v", err)
		}
	})

	t.Run("charRepo FindByID error propagates", func(t *testing.T) {
		svc, _, charRepo, _ := setupService()
		charRepo.findByIDErr = errors.New("mariadb connection lost")

		_, err := svc.StartPartySession(ctx, "lead", []string{"lead"}, "0", "Party", "#FFFFFF")
		if err == nil || !strings.Contains(err.Error(), "mariadb connection lost") {
			t.Errorf("expected charRepo error, got %v", err)
		}
	})

	t.Run("activeStore SaveActiveSession error propagates", func(t *testing.T) {
		svc, _, _, activeStore := setupService()
		activeStore.publishRunErr = errors.New("valkey write timeout")

		_, err := svc.StartPartySession(ctx, "lead", []string{"lead"}, "0", "Party", "#FFFFFF")
		if err == nil || !strings.Contains(err.Error(), "valkey write timeout") {
			t.Errorf("expected activeStore save error, got %v", err)
		}
	})

	t.Run("repo SaveSession error propagates with context", func(t *testing.T) {
		svc, repo, _, _ := setupService()
		repo.saveSessionErr = errors.New("duplicate key in challenge_sessions")

		_, err := svc.StartPartySession(ctx, "lead", []string{"lead"}, "0", "Party", "#FFFFFF")
		if err == nil || !strings.Contains(err.Error(), "saving challenge session: duplicate key in challenge_sessions") {
			t.Errorf("expected wrapped repo save session error, got %v", err)
		}
	})
}

// approvedRoster models a lobby that has already admitted these gameplay fixtures.
// Authorization regressions use the real Party service in composition tests.
type approvedRoster struct{}

func (approvedRoster) AdmitParty(ctx context.Context, actor, partyID string, ids []string, start func(context.Context, string, []string) error) error {
	if len(ids) == 0 {
		ids = []string{actor}
	}
	return start(ctx, partyID, ids)
}
func (approvedRoster) AdmitSolo(ctx context.Context, actor string, start func(context.Context) error) error {
	return start(ctx)
}

func TestPartyRequiresAdmission(t *testing.T) {
	svc, err := challenge.NewService(newMockChallengeRepo(), &mockCharRepo{}, &corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartPartySession(context.Background(), "leader", []string{"leader", "member"}, "1", "Party", ""); !errors.Is(err, challenge.ErrPartyUnavailable) {
		t.Fatalf("party start without consent port: %v", err)
	}
}

func TestChallengeAdmissionRules_SoloAndParty(t *testing.T) {
	ctx := context.Background()

	type testCase struct {
		name    string
		tierID  string
		modChar func(*corecharacter.Character)
		wantErr error
	}

	cases := []testCase{
		{
			name:   "normal tier accepts level 99 without OldJobID",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Level = 99
				c.Experience = 98010
				c.OldJobID = ""
			},
			wantErr: nil,
		},
		{
			name:   "normal tier accepts level 99 with OldJobID",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Level = 99
				c.Experience = 0
				c.OldJobID = "warrior"
			},
			wantErr: nil,
		},
		{
			name:   "normal tier rejects level 100 without OldJobID",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Level = 100
				c.Experience = 0
				c.OldJobID = ""
			},
			wantErr: challenge.ErrLevelTooHigh,
		},
		{
			name:   "normal tier rejects level 100 even with OldJobID",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Level = 100
				c.Experience = 0
				c.OldJobID = "warrior"
			},
			wantErr: challenge.ErrLevelTooHigh,
		},
		{
			name:   "over-level tier 8 accepts level 100 even with empty OldJobID",
			tierID: "8",
			modChar: func(c *corecharacter.Character) {
				c.Level = 100
				c.Experience = 100000
				c.OldJobID = ""
			},
			wantErr: nil,
		},
		{
			name:   "over-level tier 8 accepts level 150 with OldJobID",
			tierID: "8",
			modChar: func(c *corecharacter.Character) {
				c.Level = 150
				c.Experience = 0
				c.OldJobID = "warrior"
			},
			wantErr: nil,
		},
		{
			name:   "over-level tier 8 rejects level 99 even with OldJobID",
			tierID: "8",
			modChar: func(c *corecharacter.Character) {
				c.Level = 99
				c.Experience = 0
				c.OldJobID = "warrior"
			},
			wantErr: challenge.ErrNeedOverLvNotMet,
		},
		{
			name:   "over-level tier 8 rejects level 1 with OldJobID",
			tierID: "8",
			modChar: func(c *corecharacter.Character) {
				c.Level = 1
				c.Experience = 0
				c.OldJobID = "warrior"
			},
			wantErr: challenge.ErrNeedOverLvNotMet,
		},
		{
			name:   "pending level-up rejected at exact threshold (lv 10, exp 1000)",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Level = 10
				c.Experience = 1000
			},
			wantErr: challenge.ErrPendingLevelUp,
		},
		{
			name:   "experience just below threshold accepted (lv 10, exp 999)",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Level = 10
				c.Experience = 999
			},
			wantErr: nil,
		},
		{
			name:   "unconscious character (HP 0) rejected instead of healing to MaxHP",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Stats.HP = 0
				c.Stats.MaxHP = 200
			},
			wantErr: challenge.ErrCharacterUnconscious,
		},
		{
			name:   "exhausted character (Tired 100) rejected",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Tired = 100
			},
			wantErr: challenge.ErrCharacterExhausted,
		},
		{
			name:   "tired 99 character accepted",
			tierID: "2",
			modChar: func(c *corecharacter.Character) {
				c.Tired = 99
			},
			wantErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run("solo/"+tc.name, func(t *testing.T) {
			repo := newMockChallengeRepo()
			c1 := createPartyTestChar("solo", 10, 200, 50, 30)
			tc.modChar(&c1)
			charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{"solo": c1}}
			svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{})
			if err != nil {
				t.Fatal(err)
			}

			sess, err := svc.StartSession(ctx, "solo", tc.tierID)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v (sess=%+v)", tc.wantErr, err, sess)
				}
				if active, _ := svc.GetActiveSession(ctx, "solo"); active != nil {
					t.Fatalf("rejected start left active session: %+v", active)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sess == nil || sess.CharacterCurrentHP != c1.Stats.HP {
				t.Fatalf("unexpected session: %+v", sess)
			}
		})

		t.Run("party-leader/"+tc.name, func(t *testing.T) {
			repo := newMockChallengeRepo()
			c1 := createPartyTestChar("lead", 10, 200, 50, 30)
			tc.modChar(&c1)
			charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{"lead": c1}}
			svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithAdmission(approvedRoster{}))
			if err != nil {
				t.Fatal(err)
			}

			sess, err := svc.StartPartySession(ctx, "lead", []string{"lead"}, tc.tierID, "Party", "#FFFFFF")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v (sess=%+v)", tc.wantErr, err, sess)
				}
				if active, _ := svc.GetActiveSession(ctx, "lead"); active != nil {
					t.Fatalf("rejected start left active session: %+v", active)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sess == nil || sess.CharacterCurrentHP != c1.Stats.HP {
				t.Fatalf("unexpected session: %+v", sess)
			}
		})

		if tc.tierID == "2" {
			t.Run("party-member/"+tc.name, func(t *testing.T) {
				repo := newMockChallengeRepo()
				lead := createPartyTestChar("lead", 10, 200, 50, 30)
				mem := createPartyTestChar("mem", 10, 180, 45, 25)
				tc.modChar(&mem)
				charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{"lead": lead, "mem": mem}}
				svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{}, challenge.WithAdmission(approvedRoster{}))
				if err != nil {
					t.Fatal(err)
				}

				sess, err := svc.StartPartySession(ctx, "lead", []string{"lead", "mem"}, tc.tierID, "Party", "#FFFFFF")
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("expected error %v, got %v (sess=%+v)", tc.wantErr, err, sess)
					}
					for _, id := range []string{"lead", "mem"} {
						if active, _ := svc.GetActiveSession(ctx, id); active != nil {
							t.Fatalf("rejected party start left active session for %s: %+v", id, active)
						}
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if sess == nil || len(sess.Members) != 2 || sess.Members[1].CharacterCurrentHP != mem.Stats.HP {
					t.Fatalf("unexpected session: %+v", sess)
				}
			})
		}
	}
}
