package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/pvp"
)

type stubChallengeService struct {
	listTiersFn           func() []challenge.ChallengeTier
	getTierFn             func(tierID string) (*challenge.ChallengeTier, error)
	startSessionFn        func(ctx context.Context, characterID string, tierID string) (*challenge.ChallengeSession, error)
	advanceRoundFn        func(ctx context.Context, characterID string, sessionID string) (*challenge.RoundResult, *challenge.ChallengeSession, error)
	retireSessionFn       func(ctx context.Context, characterID string, sessionID string) (*challenge.ChallengeSession, error)
	getCharacterRecordsFn func(ctx context.Context, characterID string) ([]challenge.CharacterChallengeRecord, error)
}

func (s *stubChallengeService) ListTiers() []challenge.ChallengeTier {
	if s.listTiersFn != nil {
		return s.listTiersFn()
	}
	return []challenge.ChallengeTier{{ID: "novice", Name: "Novice"}}
}
func (s *stubChallengeService) GetTier(tierID string) (*challenge.ChallengeTier, error) {
	if s.getTierFn != nil {
		return s.getTierFn(tierID)
	}
	return &challenge.ChallengeTier{ID: tierID}, nil
}
func (s *stubChallengeService) StartSession(ctx context.Context, characterID string, tierID string) (*challenge.ChallengeSession, error) {
	if s.startSessionFn != nil {
		return s.startSessionFn(ctx, characterID, tierID)
	}
	return &challenge.ChallengeSession{ID: "sess-1", CharacterID: characterID, TierID: tierID, Status: challenge.StatusActive}, nil
}
func (s *stubChallengeService) AdvanceRound(ctx context.Context, characterID string, sessionID string) (*challenge.RoundResult, *challenge.ChallengeSession, error) {
	if s.advanceRoundFn != nil {
		return s.advanceRoundFn(ctx, characterID, sessionID)
	}
	return &challenge.RoundResult{Round: 1, Won: true}, &challenge.ChallengeSession{ID: sessionID, CharacterID: characterID, CurrentRound: 2, Status: challenge.StatusActive}, nil
}
func (s *stubChallengeService) RetireSession(ctx context.Context, characterID string, sessionID string) (*challenge.ChallengeSession, error) {
	if s.retireSessionFn != nil {
		return s.retireSessionFn(ctx, characterID, sessionID)
	}
	return &challenge.ChallengeSession{ID: sessionID, CharacterID: characterID, Status: challenge.StatusClaimed}, nil
}
func (s *stubChallengeService) GetCharacterRecords(ctx context.Context, characterID string) ([]challenge.CharacterChallengeRecord, error) {
	if s.getCharacterRecordsFn != nil {
		return s.getCharacterRecordsFn(ctx, characterID)
	}
	return []challenge.CharacterChallengeRecord{{CharacterID: characterID, TierID: "novice", HighestRound: 5}}, nil
}

type stubBossService struct {
	listBossesFn         func(ctx context.Context, characterID string) ([]boss.BossEncounterStatus, error)
	challengeBossFn      func(ctx context.Context, characterID, bossID string) (boss.ChallengeResult, error)
	getCharacterRecordFn func(ctx context.Context, characterID string) (boss.CharacterBossRecord, error)
}

func (s *stubBossService) ListBosses(ctx context.Context, characterID string) ([]boss.BossEncounterStatus, error) {
	if s.listBossesFn != nil {
		return s.listBossesFn(ctx, characterID)
	}
	return []boss.BossEncounterStatus{{Boss: boss.Boss{ID: "boss-1", Name: "Dragon"}, IsUnlocked: true}}, nil
}
func (s *stubBossService) ChallengeBoss(ctx context.Context, characterID, bossID string) (boss.ChallengeResult, error) {
	if s.challengeBossFn != nil {
		return s.challengeBossFn(ctx, characterID, bossID)
	}
	return boss.ChallengeResult{Outcome: "WIN"}, nil
}
func (s *stubBossService) GetCharacterRecord(ctx context.Context, characterID string) (boss.CharacterBossRecord, error) {
	if s.getCharacterRecordFn != nil {
		return s.getCharacterRecordFn(ctx, characterID)
	}
	return boss.CharacterBossRecord{CharacterID: characterID}, nil
}

type stubDungeonService struct {
	listDungeonsFn        func(ctx context.Context, characterID string) ([]dungeon.DungeonOverview, error)
	startExpeditionFn     func(ctx context.Context, characterID string, dungeonID string) (*dungeon.ActiveExpedition, error)
	moveFn                func(ctx context.Context, characterID string, dir dungeon.Direction) (dungeon.ExpeditionStepResult, error)
	escapeFn              func(ctx context.Context, characterID string) (dungeon.ExpeditionStepResult, error)
	getActiveExpeditionFn func(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error)
}

func (s *stubDungeonService) ListDungeons(ctx context.Context, characterID string) ([]dungeon.DungeonOverview, error) {
	if s.listDungeonsFn != nil {
		return s.listDungeonsFn(ctx, characterID)
	}
	return []dungeon.DungeonOverview{{Dungeon: dungeon.Dungeon{ID: "d1", Name: "Cave"}, IsUnlocked: true}}, nil
}
func (s *stubDungeonService) StartExpedition(ctx context.Context, characterID string, dungeonID string) (*dungeon.ActiveExpedition, error) {
	if s.startExpeditionFn != nil {
		return s.startExpeditionFn(ctx, characterID, dungeonID)
	}
	return &dungeon.ActiveExpedition{ID: "exp-1", CharacterID: characterID, DungeonID: dungeonID, Status: dungeon.StatusExploring}, nil
}
func (s *stubDungeonService) Move(ctx context.Context, characterID string, dir dungeon.Direction) (dungeon.ExpeditionStepResult, error) {
	if s.moveFn != nil {
		return s.moveFn(ctx, characterID, dir)
	}
	return dungeon.ExpeditionStepResult{EventType: dungeon.EventMove, Expedition: dungeon.ActiveExpedition{Status: dungeon.StatusExploring}}, nil
}
func (s *stubDungeonService) Escape(ctx context.Context, characterID string) (dungeon.ExpeditionStepResult, error) {
	if s.escapeFn != nil {
		return s.escapeFn(ctx, characterID)
	}
	return dungeon.ExpeditionStepResult{EventType: dungeon.EventEscape, Expedition: dungeon.ActiveExpedition{Status: dungeon.StatusEscaped}}, nil
}
func (s *stubDungeonService) GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error) {
	if s.getActiveExpeditionFn != nil {
		return s.getActiveExpeditionFn(ctx, characterID)
	}
	return nil, dungeon.ErrNoActiveExpedition
}

type stubPvPService struct {
	getRatingFn     func(ctx context.Context, characterID string) (pvp.ArenaRating, error)
	findOpponentsFn func(ctx context.Context, characterID string, limit int) ([]pvp.OpponentCandidate, error)
	challengeFn     func(ctx context.Context, attackerID, defenderID string) (pvp.ChallengeResult, error)
}

func (s *stubPvPService) GetRating(ctx context.Context, characterID string) (pvp.ArenaRating, error) {
	if s.getRatingFn != nil {
		return s.getRatingFn(ctx, characterID)
	}
	return pvp.ArenaRating{CharacterID: characterID, Rating: 1500, UpdatedAt: time.Now()}, nil
}
func (s *stubPvPService) FindOpponents(ctx context.Context, characterID string, limit int) ([]pvp.OpponentCandidate, error) {
	if s.findOpponentsFn != nil {
		return s.findOpponentsFn(ctx, characterID, limit)
	}
	return []pvp.OpponentCandidate{{CharacterID: "c2", Name: "Rival", Rating: 1510}}, nil
}
func (s *stubPvPService) Challenge(ctx context.Context, attackerID, defenderID string) (pvp.ChallengeResult, error) {
	if s.challengeFn != nil {
		return s.challengeFn(ctx, attackerID, defenderID)
	}
	return pvp.ChallengeResult{
		Match: pvp.MatchRecord{
			Outcome:             pvp.OutcomeWin,
			AttackerRatingAfter: 1516,
			DefenderRatingAfter: 1494,
		},
	}, nil
}

func TestCombatEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithChallenge(&stubChallengeService{}),
		apihttp.WithBoss(&stubBossService{}),
		apihttp.WithDungeon(&stubDungeonService{}),
		apihttp.WithPvP(&stubPvPService{}),
	)
	router := h.Router()

	// Challenges
	t.Run("GET /challenges/tiers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/challenges/tiers", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id}/challenges/records", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/challenges/records", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/challenges/start", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/challenges/start", `{"tier_id":"novice"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/challenges/advance", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/challenges/advance", `{"session_id":"sess-1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/challenges/advance - forbidden for non-owned session", func(t *testing.T) {
		hForbidden := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithChallenge(&stubChallengeService{
				advanceRoundFn: func(ctx context.Context, characterID string, sessionID string) (*challenge.RoundResult, *challenge.ChallengeSession, error) {
					return nil, nil, challenge.ErrForbidden
				},
			}),
		)
		req := jsonRequest(t, http.MethodPost, "/characters/c1/challenges/advance", `{"session_id":"other-sess"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		hForbidden.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/challenges/retire", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/challenges/retire", `{"session_id":"sess-1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/challenges/retire - forbidden for non-owned session", func(t *testing.T) {
		hForbidden := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithChallenge(&stubChallengeService{
				retireSessionFn: func(ctx context.Context, characterID string, sessionID string) (*challenge.ChallengeSession, error) {
					return nil, challenge.ErrForbidden
				},
			}),
		)
		req := jsonRequest(t, http.MethodPost, "/characters/c1/challenges/retire", `{"session_id":"other-sess"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		hForbidden.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec.Code)
		}
	})

	// Bosses
	t.Run("GET /characters/{id}/bosses", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/bosses", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/bosses/fight", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/bosses/fight", `{"boss_id":"boss-1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	// Dungeons
	t.Run("GET /characters/{id}/dungeons", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/dungeons", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/dungeons/start", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/dungeons/start", `{"dungeon_id":"d1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/dungeons/move", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/dungeons/move", `{"direction":"north"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/dungeons/escape", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/dungeons/escape", "")
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	// PvP
	t.Run("GET /characters/{id}/pvp", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/pvp", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id}/pvp/opponents", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/pvp/opponents?limit=5", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/fight", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/pvp/fight", `{"defender_id":"c2"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})
}
