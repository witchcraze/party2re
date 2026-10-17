package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestStartOutcomeUnknownHTTP(t *testing.T) {
	causeSave := errors.New("secret save response lost")
	causeCleanup := errors.New("secret cleanup failure")
	player := coreplayer.Player{ID: "p1"}
	for _, tc := range []struct {
		name, path, body string
		sentinel         error
	}{
		{"dungeon solo", "/characters/c1/dungeons/start", `{"dungeon_id":"dungeon-01"}`, dungeon.ErrStartOutcomeUnknown},
		{"dungeon party", "/characters/c1/dungeons/party-start", `{"dungeon_id":"dungeon-01"}`, dungeon.ErrStartOutcomeUnknown},
		{"challenge solo", "/characters/c1/challenges/start", `{"tier_id":"1"}`, challenge.ErrStartOutcomeUnknown},
		{"challenge party", "/characters/c1/challenges/party-start", `{"tier_id":"1"}`, challenge.ErrStartOutcomeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			joined := errors.Join(tc.sentinel, causeSave, causeCleanup)
			ds := &stubDungeonService{
				startExpeditionFn: func(context.Context, string, string) (*dungeon.ActiveExpedition, error) { return nil, joined },
				startPartyExpeditionFn: func(context.Context, string, []string, string, string) (*dungeon.ActiveExpedition, error) {
					return nil, joined
				},
			}
			cs := &stubChallengeService{
				startSessionFn: func(context.Context, string, string) (*challenge.ChallengeSession, error) { return nil, joined },
				startPartySessionFn: func(context.Context, string, []string, string, string, string) (*challenge.ChallengeSession, error) {
					return nil, joined
				},
			}
			h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}, &stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) {
				return corecharacter.Character{ID: "c1", PlayerID: player.ID}, nil
			}}, &stubAdventureService{}, &stubShopService{}, apihttp.WithDungeon(ds), apihttp.WithChallenge(cs))
			req := jsonRequest(t, http.MethodPost, tc.path, tc.body)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			body := rec.Body.String()
			if rec.Code != http.StatusInternalServerError || !strings.Contains(body, tc.sentinel.Error()) || strings.Contains(body, "secret") {
				t.Fatalf("status=%d body=%s", rec.Code, body)
			}
		})
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
