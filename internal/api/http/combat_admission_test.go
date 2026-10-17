package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/party"
)

func TestPartyCombatAdmissionHTTP(t *testing.T) {
	for _, domain := range []string{"dungeons", "challenges"} {
		for _, tc := range []struct {
			name   string
			err    error
			status int
		}{
			{"saved roster", nil, 200},
			{"fake party", party.ErrNotFound, 400},
			{"unrelated member", party.ErrCharacterNotInParty, 400},
			{"nonleader", party.ErrNotPartyLeader, 403},
			{"not ready", party.ErrPartyNotReady, 422},
			{"busy dungeon member", dungeon.ErrActiveExpeditionExists, 422},
			{"busy challenge member", challenge.ErrActiveSessionExists, 422},
			{"sleeping member", party.ErrMemberBusy, 422},
		} {
			t.Run(domain+"/"+tc.name, func(t *testing.T) {
				called := false
				verify := func(actor string, ids []string) {
					called = true
					if actor != "c1" || len(ids) != 0 {
						t.Fatalf("HTTP invented or changed the roster: %s %v", actor, ids)
					}
				}
				ds := &stubDungeonService{startPartyExpeditionFn: func(ctx context.Context, actor string, ids []string, target, partyID string) (*dungeon.ActiveExpedition, error) {
					verify(actor, ids)
					return &dungeon.ActiveExpedition{}, tc.err
				}}
				cs := &stubChallengeService{startPartySessionFn: func(ctx context.Context, actor string, ids []string, target, name, color string) (*challenge.ChallengeSession, error) {
					verify(actor, ids)
					return &challenge.ChallengeSession{}, tc.err
				}}
				player := coreplayer.Player{ID: "p1"}
				h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}, &stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) {
					return corecharacter.Character{ID: "c1", PlayerID: player.ID}, nil
				}}, &stubAdventureService{}, &stubShopService{}, apihttp.WithDungeon(ds), apihttp.WithChallenge(cs))
				body := `{"dungeon_id":"dungeon-01"}`
				if domain == "challenges" {
					body = `{"tier_id":"1"}`
				}
				req := jsonRequest(t, http.MethodPost, "/characters/c1/"+domain+"/party-start", body)
				req.Header.Set("Authorization", "Bearer valid-token")
				rec := httptest.NewRecorder()
				h.Router().ServeHTTP(rec, req)
				if !called || rec.Code != tc.status {
					t.Fatalf("called=%v status=%d body=%s", called, rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestOrdinaryPartyStartRejectsCombatEnrollment(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}, &stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) {
		return corecharacter.Character{ID: "c1", PlayerID: player.ID}, nil
	}}, &stubAdventureService{}, &stubShopService{}, apihttp.WithParty(&stubPartyService{err: party.ErrMemberBusy}))
	req := jsonRequest(t, http.MethodPost, "/parties/lobby/start", `{"character_id":"c1"}`)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
