package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestHandleListCharacterAdventures_Success(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "user1"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}, nil
		},
	}
	as := &stubAdventureService{
		listHistoryFn: func(_ context.Context, characterID string, limit, offset int) (adventure.PaginatedAdventures, error) {
			if characterID != "c1" || limit != 10 || offset != 5 {
				t.Fatalf("unexpected listHistory args: char=%s, limit=%d, offset=%d", characterID, limit, offset)
			}
			return adventure.PaginatedAdventures{
				CharacterID: "c1",
				Total:       1,
				Limit:       10,
				Offset:      5,
				Adventures: []adventure.AdventureHistoryEntry{
					{
						ID:          "adv-1",
						CharacterID: "c1",
						StageID:     "stage-01",
						StageName:   "平原",
						MonsterID:   "mon-01",
						MonsterName: "スライム",
						Outcome:     corebattle.OutcomeWin,
						BattleTurns: 3,
						Resolved:    true,
						Claimed:     true,
					},
				},
			}, nil
		},
	}

	h := newTestHandler(t, ps, cs, as, &stubShopService{})
	req := httptest.NewRequest(http.MethodGet, "/characters/c1/adventures?limit=10&offset=5", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	w := httptest.NewRecorder()

	h.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var res adventure.PaginatedAdventures
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if res.CharacterID != "c1" || res.Total != 1 || len(res.Adventures) != 1 {
		t.Fatalf("unexpected response body: %+v", res)
	}
	if res.Adventures[0].StageName != "平原" || res.Adventures[0].MonsterName != "スライム" {
		t.Fatalf("unexpected enriched adventure: %+v", res.Adventures[0])
	}
}

func TestHandleListCharacterAdventures_Forbidden_DifferentPlayer(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "user1"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			// Character belongs to p2, but authenticated player is p1
			return corecharacter.Character{ID: "c1", PlayerID: "p2", Name: "Hero"}, nil
		},
	}
	as := &stubAdventureService{}

	h := newTestHandler(t, ps, cs, as, &stubShopService{})
	req := httptest.NewRequest(http.MethodGet, "/characters/c1/adventures", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	w := httptest.NewRecorder()

	h.Router().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleGetAdventureChronicle_Success(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "user1"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}, nil
		},
	}
	as := &stubAdventureService{
		getChronicleFn: func(_ context.Context, characterID string) (adventure.AdventureChronicle, error) {
			return adventure.AdventureChronicle{
				CharacterID:     "c1",
				TotalAdventures: 50,
				TotalVictories:  50,
				WinRate:         1.0,
				Stages: []adventure.StageClearStat{
					{
						StageID:       "stage-01",
						StageName:     "平原",
						ClearCount:    50,
						TotalAttempts: 50,
					},
				},
				Milestones: []adventure.Milestone{
					{
						Key:       "try_mode",
						Name:      "トライモード (Try Mode)",
						Threshold: 50,
						Unlocked:  true,
					},
				},
			}, nil
		},
	}

	h := newTestHandler(t, ps, cs, as, &stubShopService{})
	req := httptest.NewRequest(http.MethodGet, "/characters/c1/adventure-chronicle", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	w := httptest.NewRecorder()

	h.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var chronicle adventure.AdventureChronicle
	if err := json.NewDecoder(w.Body).Decode(&chronicle); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if chronicle.CharacterID != "c1" || chronicle.TotalVictories != 50 || chronicle.WinRate != 1.0 {
		t.Fatalf("unexpected chronicle response: %+v", chronicle)
	}
	if len(chronicle.Milestones) == 0 || !chronicle.Milestones[0].Unlocked {
		t.Fatalf("expected unlocked milestone: %+v", chronicle.Milestones)
	}
}

func TestHandleGetAdventureChronicle_Unauthenticated(t *testing.T) {
	ps := &stubPlayerService{
		authenticateFn: func(_ context.Context, token string) (coreplayer.Player, error) {
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	req := httptest.NewRequest(http.MethodGet, "/characters/c1/adventure-chronicle", nil)
	w := httptest.NewRecorder()

	h.Router().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
