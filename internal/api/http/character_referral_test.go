package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestHandleCreateCharacterReferral(t *testing.T) {
	calls := 0
	players := &stubPlayerService{authenticateFn: func(context.Context, string) (coreplayer.Player, error) {
		return coreplayer.Player{ID: "joining-player"}, nil
	}}
	characters := &stubCharacterService{createWithOptionsFn: func(ctx context.Context, playerID, name string, options character.CreationOptions) (corecharacter.Character, error) {
		calls++
		if playerID != "joining-player" || name != "Hero" || options.ReferrerCharacterID != "other-player-character" || options.JobID != "job-01" || options.Gender != "m" {
			t.Fatalf("creation lost authenticated actor or referral: player=%q name=%q options=%+v", playerID, name, options)
		}
		return corecharacter.Character{ID: "joining", PlayerID: playerID, Name: name}, nil
	}}
	handler := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}).Router()
	request := jsonRequest(t, http.MethodPost, "/characters", `{"name":"Hero","job_id":"job-01","gender":"m","referrer_character_id":"other-player-character"}`)
	request.Header.Set("Authorization", "Bearer valid-session")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	request = jsonRequest(t, http.MethodPost, "/characters", `{"name":"Hero","referrer_character_id":"other-player-character"}`)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || calls != 1 {
		t.Fatalf("unauthenticated referral reached creation: status=%d calls=%d", response.Code, calls)
	}
}

func TestCharacterNameConflictHTTP(t *testing.T) {
	players := &stubPlayerService{authenticateFn: func(context.Context, string) (coreplayer.Player, error) {
		return coreplayer.Player{ID: "player-1"}, nil
	}}
	characters := &stubCharacterService{
		createWithOptionsFn: func(context.Context, string, string, character.CreationOptions) (corecharacter.Character, error) {
			return corecharacter.Character{}, character.ErrNameAlreadyTaken
		},
		changeNameFn: func(context.Context, string, string) (corecharacter.Character, error) {
			return corecharacter.Character{}, character.ErrNameAlreadyTaken
		},
		getFn: func(context.Context, string) (corecharacter.Character, error) {
			return corecharacter.Character{ID: "char-1", PlayerID: "player-1"}, nil
		},
	}
	handler := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}).Router()
	for _, path := range []string{"/characters", "/characters/char-1/name"} {
		request := jsonRequest(t, http.MethodPost, path, `{"name":"Taken"}`)
		request.Header.Set("Authorization", "Bearer valid-session")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusConflict {
			t.Fatalf("%s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
