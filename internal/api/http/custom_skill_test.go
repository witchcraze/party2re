package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/custom_skill"
)

type stubCustomSkillService struct{}

func (stubCustomSkillService) GetCustomSkill(context.Context, string) (*custom_skill.CustomSkill, error) {
	return &custom_skill.CustomSkill{}, nil
}

func (stubCustomSkillService) SetCustomSkill(_ context.Context, characterID, name, comment string, gems [3]string) (*custom_skill.CustomSkill, error) {
	return &custom_skill.CustomSkill{CharacterID: characterID, Name: name, Comment: comment, Gems: gems}, nil
}

func TestCustomSkillEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}
	h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)},
		&stubCharacterService{getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		}}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithCustomSkill(stubCustomSkillService{}))
	router := h.Router()

	t.Run("GET custom skill", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/custom-skills", nil)
		req.Header.Set("Authorization", bearerToken("session"))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST custom skill", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/custom-skills",
			`{"name":"炎の舞","comment":"いくぞ","gems":["gem_atk_1","",""]}`)
		req.Header.Set("Authorization", bearerToken("session"))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
